# third_party/actor 原理与流程

## 定位

`third_party/actor` 是进程内 Actor 邮箱模型，不是分布式 Actor 框架。每个 Actor 有唯一 `PID`、一个有界 mailbox 和一个顺序消费消息的 goroutine，用于把同一运行时的任务串行化。

当前项目将 `langChainAgentRuntime` 注册为 `langchain-runtime:9001`；任务调度器通过它执行 Agent 任务。

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
  -> langChainAgentRuntime.SendTask
  -> actor.SyncRequest(nil, langchain-runtime:9001, TaskCommand)
  -> langChainAgentRuntime.Process
  -> ReceiveTask -> Execute -> LLM / Tool 调用
  -> Message.Response(TaskResult)
  -> TaskDispatcher 更新任务状态、Trace、Memory
```

因此当前 `langChainAgentRuntime` 同一时刻只处理一条进入其 mailbox 的任务。并行任务会在容量为 128 的 mailbox 排队；队列满时投递失败，不会阻塞等待。

## 事务与暂停辅助能力

- `Transaction` 可对指针对象以 JSON 快照保存点；`Abort` 时反序列化回滚，`Commit` 只确认完成。它依赖反射和 JSON，注释明确提示容易死锁，仅适合非环形同步流程。
- `GroupSync` 通过 `SysId_Suspend` 暂停多个 Actor，等待全部 `Done` 或超时后恢复；当前 Agent 主任务链路未使用。
- `Dispatcher` 是独立的消息 ID 到 Handler 路由器，带慢调用统计与黑名单数据结构；当前 Agent Runtime 不依赖它。

## 边界与注意事项

- Actor 隔离的是单 Actor 消息处理顺序，不会自动隔离 `Message.Data` 指向的共享对象。
- Mailbox 是有界非阻塞投递；调用方必须处理 `ErrMailOverflow`。
- `rootSystem` 是进程全局单例；测试会重置它，生产中不可重复调用全局 `Stop` 后继续注册。
- 同步调用固定 3 秒超时，未与上层 `context.Context` 联动；耗时 Agent 任务不应依赖该路径作为取消机制。
