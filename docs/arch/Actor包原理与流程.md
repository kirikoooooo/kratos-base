# third_party/actor 原理与流程

## 定位

`third_party/actor` 是进程内 Actor 邮箱模型，不是分布式 Actor 框架。每个 Actor 有唯一 `PID`、一个有界 mailbox 和一个顺序消费消息的 goroutine，用于把同一运行时的任务串行化。

当前项目将 `agentRuntime` 注册为 `agent-runtime:9001`；任务调度器通过它执行 Agent 任务。

## 核心对象

| 对象 | 作用 |
| --- | --- |
| `Actor` | 实现 `PID()`、`Process(*Message)`、`OnStop()` |
| `rootSystem` | 进程级注册表；`PID < 1000` 用数组，其他 PID 用 map |
| `defaultMailBox` | 最小容量 32 的有界 channel；满时直接返回 `ErrMailOverflow` |
| `defaultActorRef` | 对外提供 `Send`、`AsyncRequest`、`Request` |
| `Message` | 消息 ID、数据、来源/目标 PID，以及同步响应 channel 或异步回调 |

## 注册与生命周期

```text
NewTaskDispatcher
  -> StopActor(runtime.PID())       # 清理同 PID 的旧运行时
  -> RegisterActor(runtime, 128)
  -> rootSystem.spawn
  -> 创建 mailbox / ActorRef，登记 PID
  -> goroutine actorLoop
       -> 顺序读取 mailbox
       -> Process(message)
       -> OnStop()
```

`StopActor` 向 mailbox 投递 `SysId_Stop`，随后立即从注册表移除；循环读到该系统消息后退出。全局 `Stop` 会关闭根系统并等待全部 Actor goroutine 结束。

## 消息处理

```text
调用方
  -> rootSystem.get(to PID)
  -> ActorRef 将 Message 投递到目标 mailbox
  -> 目标 actorLoop 串行 dispatchMessage
       -> 系统消息处理 / Hotfix 拦截
       -> Actor.Process
       -> Message.Response
```

系统消息包括停止、邮箱积压指标和暂停。普通消息在 `Process` 前会经过可替换的 `hotfixInterface`；其返回 `true` 时消息被拦截，不再调用业务 `Process`。

## 三种调用方式

| API | 调用方行为 | 响应路径 |
| --- | --- | --- |
| `Send` | 仅投递，不等待 | 无响应 |
| `SyncRequest` | 等待最多 3 秒 | `Message.Response` 写入 `respCh` |
| `AsyncRequest` | 立即返回 | `Response` 将回调函数投递回发送方 Actor 的 mailbox |

同步请求禁止向自身发送，否则会阻塞自身 mailbox，返回 `ErrSyncRequestSelf`。超时只结束等待，不会取消目标 Actor 中已经开始的处理。异步回调在发送方 Actor 的 goroutine 中执行，因此发送方必须仍在注册表中。

## 项目任务链路

```text
CLI / Dashboard
  -> TaskDispatcher.Dispatch（后台 goroutine）
  -> agentRuntime.SendTask
  -> actor.SyncRequest(nil, agent-runtime:9001, TaskCommand)
  -> agentRuntime.Process
  -> ReceiveTask -> Execute -> LLM / Tool 调用
  -> Message.Response(TaskResult)
  -> TaskDispatcher 更新任务状态、Trace、Memory
```

因此当前 `agentRuntime` 同一时刻只处理一条进入其 mailbox 的任务。并行任务会在容量为 128 的 mailbox 排队；队列满时投递失败，不会阻塞等待。

## 事务与暂停辅助能力

- `Transaction` 可对指针对象以 JSON 快照保存点；`Abort` 时反序列化回滚，`Commit` 只确认完成。它依赖反射和 JSON，注释明确提示容易死锁，仅适合非环形同步流程。
- `GroupSync` 通过 `SysId_Suspend` 暂停多个 Actor，等待全部 `Done` 或超时后恢复；当前 Agent 主任务链路未使用。
- `Dispatcher` 是独立的消息 ID 到 Handler 路由器，带慢调用统计与黑名单数据结构；当前 Agent Runtime 不依赖它。

## 边界与注意事项

- Actor 隔离的是单 Actor 消息处理顺序，不会自动隔离 `Message.Data` 指向的共享对象。
- Mailbox 是有界非阻塞投递；调用方必须处理 `ErrMailOverflow`。
- `rootSystem` 是进程全局单例；测试会重置它，生产中不可重复调用全局 `Stop` 后继续注册。
- 同步调用固定 3 秒超时，未与上层 `context.Context` 联动；耗时 Agent 任务不应依赖该路径作为取消机制。

## 项目中的 Agent 运行与多 Agent 交互

### 先区分三种边界

项目当前的 `default`、`router`、`coder`、`reviewer` 是同一 `agentRuntime` 中按 Prompt 和工具集区分的逻辑角色，不是默认一角色一进程，也不是默认一角色一 Actor。所有角色委派先构造 A2A Task：本地由 A2A adapter 直接调用同一 Runtime，远端则通过 A2A JSON-RPC 2.0 调用目标进程。

| 交互类型 | 发送方与接收方 | 协议 / 载体 | 是否跨进程 | 当前处理方式 |
| --- | --- | --- | --- | --- |
| 外部任务提交 | HTTP 或 JSON-RPC 客户端 -> `TaskService` | HTTP `POST /api/v1/tasks` 或 JSON-RPC `tasks.create` | 可跨进程 | 创建内存任务后异步交给 `TaskDispatcher`。 |
| 顶层任务执行 | `TaskDispatcher` -> 本地 Runtime | `actor.Message`，同步 `SyncRequest` | 否 | 消息 `Data` 为 `*TaskCommand`，投递至 `agent-runtime:9001` 的 mailbox。 |
| 本地角色委派 | Router/Coder -> 同一 Runtime | A2A `SendMessage` adapter | 否 | 复用当前 context 和 task ID，按目标角色切换 Prompt/工具集后执行。 |
| 远端角色委派 | Router/Coder -> 远端 Runtime | A2A JSON-RPC 2.0 `SendMessage` | 是 | 根据 `runtime.remotes[].agent` 匹配目标，POST 到目标 `/a2a` endpoint。 |
| 结果和进度回传 | Runtime -> 调度器 / Dashboard / CLI | Actor 响应、内存 Trace、SSE | 进度 SSE 可供其他进程订阅 | 顶层结果返回 `TaskResult`；过程事件写入 Trace，再由 SSE 推送。 |

### 顶层任务：HTTP/JSON-RPC 到 Actor Runtime

```text
POST /api/v1/tasks 或 JSON-RPC `tasks.create`
  -> TaskUsecase.Create
       保存 Task{pending}
       -> TaskDispatcher.Dispatch（启动后台 goroutine，立即返回 pending 任务）
  -> TaskDispatcher.handle
       pending -> running，初始化 Memory / Trace / Plan
       -> runtime.SendTask(TaskCommand)
            -> actor.SyncRequest(nil, agent-runtime:9001, Message{Id: 1001, Data: *TaskCommand})
            -> agentRuntime.Process
                 -> ReceiveTask -> Execute(按 agent 选择角色流程)
                 -> Message.Response(RespMessage{Data: *TaskResult, Err: ...})
       -> done 或 failed，持久化 Task 并更新 Trace
```

Actor 消息体并不是网络协议：`Message.Data` 是进程内 Go 指针 `*taskv1.TaskCommand`。本链路中 `Message.Id` 固定为 `1001`，业务并未按该 ID 做分发；`from` 为空 PID，响应经同步 `respCh` 回到 `SendTask` 的等待 goroutine。由于 Actor mailbox 串行消费，多个顶层任务虽然各自有调度 goroutine，仍会在同一个 Runtime mailbox 中排队执行。

### 进程间委派协议

远端委派配置的最小形式如下；未配置匹配项时不会拨号，而是走前述本地直接调用。

```yaml
runtime:
  remotes:
    - agent: coder
      target: http://127.0.0.1:8000/a2a
      timeout: 8
```

通信契约采用 A2A JSON-RPC 2.0：`на `POST /a2a` обменивается `SendMessageRequest` и `Task`/выходное `Message`。

| A2A 对象 | 字段 | 语义 |
| --- | --- | --- |
| `SendMessageRequest.message` | `metadata.task_id` | 父任务 / 会话 ID；子任务沿用父 ID，使 Memory 与 Trace 可以归集。 |
|  | `metadata.agent` | 目标逻辑角色，例如 `coder` 或 `reviewer`。 |
|  | `parts[].text` | 委派给子 Agent 的完整任务文本。 |
| `Message` / `Task` 响应 | `parts[].text` | 子任务正文；调用方将它格式化为 tool result，再交回 Router/Coder 的模型上下文。 |
| JSON-RPC error | `error` | 超时、传输或远端执行失败；调用方记录失败事件并把错误返回给发起委派工具。 |

调用端在 `executeRemoteTask` 中为 A2A HTTP 请求创建带超时的 `context`。当前仍未配置 TLS、认证、重试或推送订阅；`timeout` 到期会让调用方返回错误，但不保证远端已停止执行。

远端进程通过 HTTP server 暴露 `POST /a2a` 和 `/.well-known/agent-card.json`。A2A executor 将输入映射为 `TaskCommand`，经远端 Runtime Actor mailbox 执行。

### 角色之间如何交互

`router` 的函数调用工具集暴露 `coder_agent` 与 `reviewer_agent`；`coder` 还可调用 `router_agent` 与 `reviewer_agent`。模型决定是否调用这些工具，工具 Handler 随后调用 `dispatchSubTask`：优先远端配置，否则在本地执行。`reviewer` 当前是纯 LLM 角色，不向其他 Agent 暴露委派工具。

```text
Router LLM
  -> function call: coder_agent(prompt)
  -> dispatchSubTask(coder, prompt)
       -> remote configured ? A2A JSON-RPC SendMessage : local A2A adapter
  -> TaskResult(summary, output)
  -> formatTaskResult
  -> 作为 tool result 注入 Router 的下一轮 LLM 上下文
  -> Router 继续调用 reviewer_agent 或输出最终答复
```

因此角色交互在边界层采用 A2A `Message` / `Task`；adapter 将其映射为内部 `TaskCommand` / `TaskResult`。Actor 仍负责本地 Runtime 的串行任务执行；跨进程委派统一使用 JSON-RPC 2.0。

### 状态、可观测性与交互接口

顶层 `Task` 的状态机为 `pending -> running -> done | failed`。任务记录目前使用带锁的内存仓库，进程重启后不会恢复；`CreateTask` 返回时通常仍为 `pending`，调用方应通过查询接口或事件流观察最终状态。子任务没有独立 `Task` 实体和独立状态机，它们以父 `task_id` 的 Trace 事件记录委派、远端执行成功或失败。

| 接口 | 用途 | 返回 / 事件内容 |
| --- | --- | --- |
| `POST /api/v1/tasks` | 创建顶层任务 | `TaskReply`：`task_id`、角色、Prompt、状态、结果、错误、创建/更新时间。 |
| `GET /api/v1/tasks/{taskID}` | 查询顶层任务 | 同上；用于轮询 `pending/running/done/failed`。 |
| JSON-RPC `tasks.create` / `tasks.get` | HTTP 接口的 RPC 等价物 | `TaskReply`。 |
| `POST /a2a` JSON-RPC | 远端 Agent 执行子任务 | A2A `SendMessageRequest -> Message/Task`。 |
| `POST /debug/a2a/message` | Dashboard 向已启动角色发送会话消息 | 同步返回 `task_id`、`TaskResult` 与会话快照。 |
| `POST /debug/a2a/verify` | 验证 Router 到指定角色的委派链路 | 同步返回验证子任务的结果与 Trace 会话。 |
| `GET /debug/a2a/events?task_id=...` | Dashboard SSE | `trace.event`，含阶段、角色、目标地址、Prompt 摘要、工具输入输出、错误、耗时、模型和 token 信息。 |
| `GET /debug/cli/stream?session_id=...` | CLI 可选 SSE 输出 | CLI 回合开始、进度、完成或失败事件。 |

Trace 会话在内存中最多保留 32 个，事件会驱动计划步骤更新。例如 `delegate_local`、`delegate_remote` 表示委派开始，`remote_execute_done` 或 `remote_execute_failed` 表示远端结果，`task_done` / `task_failed` 表示顶层任务终态。Dashboard 和 CLI 通过订阅该 Trace 读取进度；它们不是 Agent 之间的命令通道。

### 当前限制

- 当前只有一个本地 Runtime Actor，角色并行和并发子任务尚未实现；本地委派会在当前调用栈内同步运行。
- Router 与 Coder 都可能互相委派；没有循环检测或最大委派深度，Prompt 和模型策略需要避免递归调用。
- 顶层 Actor `SyncRequest` 的固定等待上限是 3 秒，而远端 A2A 默认超时来自 `runtime.remotes[].timeout`；二者未统一，也未形成端到端取消协议。
- 任务、Trace 与 Dashboard 状态均以进程内存为主；没有跨进程共享的任务存储、去重、断点恢复或可靠投递。
- 远端 A2A JSON-RPC 通道默认明文 HTTP；部署到非受信任网络前，应补充 mTLS/认证、授权、审计、请求大小限制与幂等任务 ID。
