# Langfuse Trace 观测与评测框架

## 接入方式

本项目使用 Langfuse 官方的 OpenTelemetry（OTLP/HTTP）接入，不使用不存在可用实现的 `langfuse-go` 模块。

```text
Agent / Tool / MCP / 委派事件
        -> 本地 DelegationTraceStore -> Dashboard / SSE
        -> Langfuse OTLP exporter  -> Langfuse Trace
```

本地 Trace 是实时控制与调试来源；Langfuse 是异步持久观测后端。发送或鉴权失败不会影响任务。

## 安装

依赖已经加入项目：

```bash
go get go.opentelemetry.io/otel@v1.44.0 \
  go.opentelemetry.io/otel/sdk@v1.44.0 \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.44.0
```

创建 Langfuse Project，在 Settings 中获取 Public Key 和 Secret Key。密钥只放环境变量：

```bash
export LANGFUSE_PUBLIC_KEY=pk-lf-...
export LANGFUSE_SECRET_KEY=sk-lf-...
```

启用 `configs/config.yaml`：

```yaml
observability:
  langfuse:
    enabled: true
    host: https://cloud.langfuse.com # 自托管时改为其 API host
    public_key_env: LANGFUSE_PUBLIC_KEY
    secret_key_env: LANGFUSE_SECRET_KEY
    environment: development
    release: kratos-demo
    flush_timeout_seconds: 5
```

不要将 Secret Key 写入 YAML、`.myagent/credentials.json`、日志或 SSE。

## 传输握手

启动时读取配置指定的环境变量，并构造官方 OTLP HTTP 请求：

```text
POST {host}/api/public/otel/v1/traces
Authorization: Basic base64(public_key:secret_key)
x-langfuse-ingestion-version: 4
Content-Type: application/x-protobuf
```

Cloud 端点为 `https://cloud.langfuse.com/api/public/otel/v1/traces`。Exporter 批量发送，进程退出时在 `flush_timeout_seconds` 内 flush。

## 事件映射与使用

| 本地动作 | Langfuse 对象 |
| --- | --- |
| 任务开始/结束 | root span `agent.task`，携带 task ID、状态和最终输出 |
| 工具/MCP | child span `tool.<name>`，携带受限长度的输入、输出和错误 |
| 委派/远程执行 | child span `agent.delegate` |
| Agent 进度、计划、上下文压缩 | child span 或 trace metadata |

执行 CLI 或 HTTP 服务后，在 Langfuse 项目的 Traces 中查看 `agent.task`。本地实时调试仍使用 `/debug/a2a/events?task_id=<id>` 和 CLI SSE。

## 评测

以 Trace ID 关联测试集执行与 Score。首批推荐指标：`task_success`、`tool_error_rate`、`answer_quality`、`latency_ms`。每次实验固定模型、Skill/MCP 配置和 release/environment，确保可比较。
