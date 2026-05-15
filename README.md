# Kratos Demo

这是一个基于 [`Kratos`](go.mod:6) 构建的 Go 示例项目，用来演示以下几类能力的组合：

- HTTP / gRPC 服务装配
- `biz / data / service / server` 分层
- 基于内存仓储的任务管理
- 基于 [`LangChainGo`](go.mod:39) 的多 Agent 运行时
- 基于 [`third_party/actor`](third_party/actor/actor.go:41) 的 actor 消息投递与调度

当前项目已经切换到 [`Go 1.24.0`](go.mod:3)，并验证可以正常构建。

---

## 项目目标

这个项目实现了一个最小可运行的“任务驱动 Agent 服务”：

1. 客户端通过 HTTP 提交任务
2. 服务把任务保存到内存仓储
3. 任务通过 dispatcher 投递给 AgentRuntime
4. AgentRuntime 作为 actor 注册到 actor system 中
5. actor 收到消息后执行对应 agent 逻辑并返回结果
6. 任务状态更新为 `done` 或 `failed`

---

## 目录结构

### 启动入口

- [`cmd/kratos-demo/main.go`](cmd/kratos-demo/main.go) 负责加载配置、初始化依赖并启动应用

### 核心分层

- [`internal/server`](internal/server) HTTP / gRPC 入口与协议适配
- [`internal/service`](internal/service) 面向接口层的服务封装
- [`internal/biz`](internal/biz) 业务模型、接口定义、用例编排
- [`internal/data`](internal/data) 仓储、runtime、dispatcher 等具体实现
- [`third_party/actor`](third_party/actor) 本地 actor 框架实现

### 关键文件

- [`internal/biz/task.go`](internal/biz/task.go) 任务模型、任务状态、仓储接口、调度接口
- [`internal/biz/agent.go`](internal/biz/agent.go) AgentRuntime 抽象与 usecase 代理
- [`internal/data/langchain.go`](internal/data/langchain.go) LangChain Runtime 与 actor 能力实现
- [`internal/data/task_dispatcher.go`](internal/data/task_dispatcher.go) 任务分发与 actor 调用入口
- [`internal/data/task_memory.go`](internal/data/task_memory.go) 内存版任务仓储
- [`internal/service/task.go`](internal/service/task.go) 创建任务、查询任务服务
- [`internal/server/task_http.go`](internal/server/task_http.go) HTTP 路由与处理器

---

## 当前能力

### 1. 任务创建与查询

当前 HTTP 路由注册在 [`registerTaskHTTPServer()`](internal/server/task_http.go:13)：

- `POST /api/v1/tasks`
- `GET /api/v1/tasks/{task_id}`

服务入口：

- [`CreateTask()`](internal/service/task.go:38)
- [`GetTask()`](internal/service/task.go:46)

### 2. 支持的 Agent

任务 agent 定义在 [`internal/biz/task.go`](internal/biz/task.go:28)：

- `router`
- `coder`
- `reviewer`

其中运行时能力由 [`langChainAgentRuntime`](internal/data/langchain.go:24) 提供，支持性判断由 [`Supports()`](internal/data/langchain.go:65) 控制。

### 3. Actor 接入

当前 actor 注册发生在 [`NewTaskDispatcher()`](internal/data/task_dispatcher.go:22)：

- 运行时通过 [`RegisterActor()`](third_party/actor/export.go:18) 注册
- 任务执行时通过 [`SyncRequest()`](internal/data/langchain.go:94) 投递到 actor
- actor 消息处理入口是 [`Process()`](internal/data/langchain.go:42)

`mailbox` 并不直接挂在业务对象上，而是由 actor 框架在注册时创建，并保存在 [`defaultActorRef.mb`](third_party/actor/actor_ref.go:37)。

---

## 运行环境

### Go 版本

- [`go 1.24.0`](go.mod:3)

### 主要依赖

- [`github.com/go-kratos/kratos/v2`](go.mod:6)
- [`github.com/google/wire`](go.mod:7)
- [`google.golang.org/grpc`](go.mod:8)
- [`github.com/tmc/langchaingo`](go.mod:39)

---

## 配置说明

默认配置文件位于 [`configs/config.yaml`](configs/config.yaml)。

当前项目会在启动时从 [`-conf`](cmd/kratos-demo/main.go:21) 参数指定的目录加载配置，默认目录是 `../../configs`。

配置中包含：

- HTTP 服务地址
- gRPC 服务地址
- 数据层配置
- OpenAI 兼容接口配置

建议在本地使用你自己的 API Key，并自行维护 [`configs/config.yaml`](configs/config.yaml) 中的 AI 配置，不要把真实密钥继续提交到仓库。

---

## 启动方式

### 1. 构建

项目构建命令定义在 [`Makefile`](Makefile) 的 [`build`](Makefile:14) 目标中：

```bash
go build ./...
```

### 2. 直接运行

可以在项目根目录执行：

```bash
go run ./cmd/kratos-demo -conf ./configs
```

### 3. 使用 Makefile

可用目标定义在 [`Makefile`](Makefile) 中：

- [`config`](Makefile:5)：生成配置 protobuf 对应代码
- [`proto`](Makefile:8)：生成 API protobuf / gRPC / HTTP 代码
- [`wire`](Makefile:11)：重新生成 Wire 装配代码
- [`build`](Makefile:14)：构建项目

---

## HTTP API 示例

### 创建任务

请求：

```http
POST /api/v1/tasks
Content-Type: application/json

{
  "agent": "coder",
  "prompt": "实现一个最小可运行版本"
}
```

处理逻辑入口：[`createTaskHandler()`](internal/server/task_http.go:19)

### 查询任务

请求：

```http
GET /api/v1/tasks/{task_id}
```

处理逻辑入口：[`getTaskHandler()`](internal/server/task_http.go:36)

### 返回结果示意

成功后任务可能返回：

- `pending`
- `running`
- `done`
- `failed`

这些状态定义在 [`internal/biz/task.go`](internal/biz/task.go:23)。

---

## Actor 消息流转

以任务执行为例，当前链路如下：

1. [`TaskUsecase.Create()`](internal/biz/task.go:78) 创建任务
2. 调用 [`TaskDispatcher.Dispatch()`](internal/biz/task.go:61)
3. 由 [`taskDispatcher.Dispatch()`](internal/data/task_dispatcher.go:37) 异步触发处理
4. 通过 [`runtime.SyncRequest()`](internal/data/task_dispatcher.go:69) 向 actor 发送消息
5. actor 框架内部把消息写入 mailbox
6. [`langChainAgentRuntime.Process()`](internal/data/langchain.go:42) 消费消息
7. [`langChainAgentRuntime.Execute()`](internal/data/langchain.go:74) 执行对应 agent
8. 执行结果写回仓储

当前 runtime 也直接暴露了三种 actor 能力：

- [`Send()`](internal/data/langchain.go:87)
- [`SyncRequest()`](internal/data/langchain.go:94)
- [`AsyncRequest()`](internal/data/langchain.go:101)

对应的 usecase 代理位于：

- [`Send()`](internal/biz/agent.go:87)
- [`SyncRequest()`](internal/biz/agent.go:94)
- [`AsyncRequest()`](internal/biz/agent.go:101)

---

## 测试

已经补充并验证了 actor 与 agent 集成测试，测试文件位于 [`internal/data/task_dispatcher_test.go`](internal/data/task_dispatcher_test.go)。

主要测试：

- [`TestAgentRuntimeSyncRequestViaActor()`](internal/data/task_dispatcher_test.go:17)
- [`TestTaskDispatcherDispatchViaActor()`](internal/data/task_dispatcher_test.go:46)

运行命令示例：

```bash
go test ./internal/data -run "TestAgentRuntimeSyncRequestViaActor|TestTaskDispatcherDispatchViaActor" -count=1
```

---

## 适合继续扩展的方向

这个项目当前更偏向 demo / 骨架，后续可以继续演进：

1. 把内存仓储替换为数据库实现
2. 增加真正的 gRPC 任务接口
3. 为 router / coder / reviewer 引入更完整的工具链
4. 把任务执行改造成更强的异步队列模型
5. 为 actor runtime 增加监控、超时、重试和指标采集
6. 将 AI 配置改为环境变量或密钥管理方案

---

## 总结

这是一个结合了 [`Kratos`](go.mod:6)、[`Wire`](go.mod:7)、[`LangChainGo`](go.mod:39) 与本地 actor 系统的 Go 项目骨架。

它重点演示了三件事：

- 如何按 Kratos 风格组织分层结构
- 如何把 AgentRuntime 以 actor 方式接入调度链路
- 如何围绕任务创建、执行、查询形成一个最小闭环
