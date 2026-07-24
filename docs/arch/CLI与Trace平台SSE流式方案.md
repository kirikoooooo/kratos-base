# CLI 与 Trace 平台 SSE 流式方案

## 1. 端点

| 场景 | SSE 地址 | 过滤条件 | 事件 |
| --- | --- | --- | --- |
| CLI 输出 | `/debug/cli/stream?session_id=<id>` | 必填 `session_id` | `cli.turn.started`、`cli.trace`、`cli.turn.completed`、`cli.turn.failed` |
| Trace 增量 | `/debug/a2a/events?task_id=<id>` | 可选 `task_id` | `trace.event` |
| Trace 快照 | `/debug/a2a/stream` | 无 | `state`（既有接口） |

CLI 默认不启动 HTTP 监听。需要订阅时以回环地址启动：

```bash
make cli ARGS='-cli-sse-addr 127.0.0.1:8010'
```

CLI 启动后打印 session ID 和订阅地址。Trace SSE 由主 HTTP 服务提供，默认地址为 `127.0.0.1:8000`。

## 2. 握手与传输

```text
前端/EventSource                 服务端                    Agent / Trace
      | GET /stream?task_id=...     |                            |
      |---------------------------->| 校验 query / 建立订阅        |
      |                             |                            |
      | 200 text/event-stream       |                            |
      |<----------------------------|                            |
      | event: ready                |                            |
      | data: {task_id/session_id}  |                            |
      |<----------------------------|                            |
      |                             |        任务、工具、委派事件 |
      |                             |<---------------------------|
      | event: trace.event          |                            |
      | data: {...}                 |                            |
      |<----------------------------|                            |
      |                             |                            |
      | close / network error       | 取消订阅                    |
      |---------------------------->|                            |
```

- 服务端响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`X-Accel-Buffering: no`。
- 首条 `ready` 事件表示订阅已建立；前端只在收到它后显示“实时连接”。
- 当前不支持 `Last-Event-ID` 重放。断线后前端先请求 `/debug/a2a/state` 恢复快照，再重新建立 SSE 连接。
- 订阅缓冲为 32；慢客户端会丢弃后续事件，不阻塞 Agent 执行。

## 3. 后端实现

1. `DelegationTraceStore.AppendEvent` 发布单条 `TraceStreamEvent`。
2. Dashboard 的 `/debug/a2a/events` 订阅事件并按 `task_id` 过滤，编码为 SSE。
3. CLI 消费 Trace 后，向 `CLIOutputStream` 发布已输出的进度和回合状态。
4. `-cli-sse-addr` 启动独立的本机 HTTP 服务，将 `CLIOutputStream` 转为 SSE。

`/debug/a2a/stream` 保留全量状态刷新；时间线、日志面板和自动化消费者应使用 `/debug/a2a/events`。

## 4. 前端实现

1. 创建 `EventSource` 并监听 `ready`、业务事件和 `error`。
2. `trace.event` 追加到任务时间线；按 `stage` 展示 Agent、工具、错误和结果。
3. `cli.trace` 显示终端进度；`cli.turn.completed` 显示最终回复；`cli.turn.failed` 显示失败原因。
4. `error` 或连接关闭时，使用退避重连；重连前先拉取 state 快照以避免遗漏。

事件 payload 均为 JSON，并至少携带 `task_id` 或 `session_id`、`type`、时间和业务内容。前端不得依赖事件到达次数或全局顺序；应以任务 ID 聚合。

## 5. 安全约束

- CLI SSE 仅允许绑定 `127.0.0.1`；不得暴露到公网。
- 生产环境在 SSE handler 前接入认证和任务可见性校验。
- 不传输 API key、MCP token、`.env` 内容或未脱敏的敏感工具输出。
