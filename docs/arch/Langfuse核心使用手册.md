# Langfuse 核心使用手册

## 1. 用途与模型

Langfuse 用于查看一次 Agent 任务中 LLM、工具、MCP 与委派的完整执行链，并基于 Trace 做成本、延迟和质量评测。

```text
一次用户消息 = 一个 Trace (agent.task)
同一 CLI 会话 = Session
LLM 调用 = Generation
工具 / MCP = Tool
子 Agent 委派 = Agent/Span
```

本项目本地 Trace/SSE 继续用于实时调试；Langfuse 是异步持久观测，不应阻塞任务执行。

## 2. 配置与启动

在 Langfuse Project 的 Settings > API Keys 创建 Key。不要把真实 Key 写入 `config.yaml`。

将密钥写入项目根目录的 `.env`（该文件已被 Git 忽略），再在启动终端加载它：

```bash
set -a
source .env
set +a
make cli
```

`configs/config.yaml` 只保存变量名：

```yaml
observability:
  langfuse:
    enabled: true
    public_key_env: LANGFUSE_PUBLIC_KEY
    secret_key_env: LANGFUSE_SECRET_KEY
    host: https://cloud.langfuse.com
    environment: development
    release: kratos-demo
```

启动终端必须出现以下提示，才表示 exporter 已启用：

```text
Langfuse: enabled; OTLP endpoint=https://cloud.langfuse.com/api/public/otel/v1/traces
```

若提示 `missing environment variable(s)`，说明 `make cli` 所在终端没有继承环境变量，不能产生远端 Trace。

## 3. Trace 内容

| Langfuse 对象 | 名称 | 核心字段 |
| --- | --- | --- |
| Root trace | `agent.task` | 用户输入、最终输出、task/session ID、Agent、环境、release、状态 |
| Generation | `generate-response` | 模型名、输入/输出、input/output tokens |
| Tool | `tool.<name>` | 工具输入、输出、错误、耗时 |
| Delegation | `agent.delegate` | 来源 Agent、目标 Agent、执行模式 |

Trace 名和 observation 名是评测、Dashboard 过滤的稳定标识；不要把 task ID、模型名等高基数字段拼入名称，应放入 metadata。

输入/输出记录只保留必要内容并有长度上限；不得写入 API Key、Token、`.env` 内容或敏感文件全文。

## 4. 上报自定义属性（Trace metadata）

使用 `DelegationTraceStore.SetTaskMetadata` 为**已经开始、尚未结束**的任务增加或覆盖属性。它同时更新本地 Trace（Dashboard/SSE 可见）和 Langfuse Trace metadata；无需也不应直接构造 OTLP span。

```go
// trace 是注入到 service/runtime 的 datatrace.DelegationTraceStore。
trace.SetTaskMetadata(taskID, map[string]any{
	"tenant_id":        "acme",      // string
	"experiment":       "router-v2", // 用于实验过滤
	"a2a.remote":       true,         // bool
	"retry_count":      2,            // integer
	"quality_snapshot": map[string]any{"score": 0.92}, // JSON 字符串
})
```

在 Langfuse 中这些字段显示为 `metadata.tenant_id`、`metadata.experiment` 等；OTLP 内部属性名为 `langfuse.trace.metadata.<key>`。相同 key 的后一次调用会覆盖前一次值。推荐在 `StartTask` 之后、`UpdateTask` 结束前调用；任务不存在或已结束时会被安全忽略。

约束：key 仅可含字母、数字、`_`、`-`、`.`；值可用 string、bool、整数、浮点数，map/slice 会 JSON 编码。单个字符串/JSON 值最多 4096 字符。不要将 task ID、完整 prompt、用户原文、密钥或其他敏感/高基数值作为 metadata；任务 ID 已由框架自动上报。

现有框架自动写入 `task_id`、`agent`、`status`、`plan_steps` 和 `context_chars`，业务自定义字段应使用具业务含义的 key，例如 `experiment`、`tenant_id`、`a2a.remote`。

## 5. 验证闭环

1. 在 CLI 提交一条消息，例如“读取 README.md 并总结项目”。
2. 等待任务结束后输入 `/exit`，触发 OTLP batch flush。
3. 查看 CLI 日志：

```bash
rg -i 'langfuse|otlp' "$(ls -t .myagent/log/*.log | head -1)"
```

成功至少应包含：

```text
Langfuse tracing enabled
Langfuse trace started
Langfuse trace ended
Langfuse OTLP export batch succeeded
Langfuse shutdown/flush completed
```

4. 打开 Langfuse Project 的 `Traces`，按 `agent.task` 或 session ID 查找；进入 Trace 后检查 Generation、Tool 与最终输出是否齐全。

## 6. 排障

| 日志/现象 | 原因与处理 |
| --- | --- |
| `credentials are missing` | 在启动 CLI 的同一终端 export `LANGFUSE_PUBLIC_KEY` 和 `LANGFUSE_SECRET_KEY` |
| 没有 `tracing enabled` | `enabled` 为 false，或 `*_env` 被误填为真实 Key 而非环境变量名称 |
| 没有 export batch | 未完成任务或未通过 `/exit` 退出；先结束任务再退出 |
| `export batch failed` | 检查网络、Base URL、Project API Key 是否匹配；不要在日志中复制 Key |
| 平台无 Trace | 确认 Cloud 区域/Project 正确，再按本节日志顺序排查 |

## 7. 查询与评测

使用官方 CLI 查询数据前，复用同一组环境变量：

```bash
npx langfuse-cli api __schema
npx langfuse-cli api observations list --help
```

优先使用 `observations` 与 `scores` API，不使用 legacy v1 接口。首批建议评分：

- `task_success`: 任务是否完成，0/1。
- `tool_error_rate`: 工具失败次数/调用次数。
- `answer_quality`: 人工或 LLM-as-a-Judge，0~1。
- `latency_ms`: 端到端耗时。

评测时固定模型、Prompt/Skill/MCP 配置、release 与 environment，保证不同实验可比较。

## 8. 官方资料

- [OpenTelemetry 集成](https://langfuse.com/integrations/native/opentelemetry)
- [Trace 最佳实践](https://langfuse.com/docs/observability/best-practices)
- [Langfuse CLI](https://langfuse.com/docs/api-and-data-platform/features/cli)
