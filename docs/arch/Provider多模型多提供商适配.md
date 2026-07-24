# Provider 多模型多提供商适配

## 目标

Agent 只依赖统一的 `ModelClient`，Provider 差异封装在数据层。默认使用 DeepSeek；OpenAI、Gemini、Grok、Claude 和 OpenRouter 均为可选 Provider。

```text
AgentRuntime
  -> LLMProvider（策略）
  -> ModelClient（统一消息、工具调用、响应）
  -> OpenAI-compatible client
```

Gemini 使用 Google 的 OpenAI-compatible endpoint，不接入 Vertex AI 或 Google 专用 SDK。

## 结构

| 层 | 位置 | 职责 |
| --- | --- | --- |
| 领域接口 | `internal/biz/provider`、`internal/biz/llm` | `LLMProvider` 提供 `GetProviderName`、创建中立的 `ModelClient`；`ProviderError` 保留来源与原始错误 |
| Provider 工厂 | `internal/data/provider/provider.go` | 按 `ai.provider` 选择主 Provider，组装 fallback 链 |
| OpenAI-compatible 策略 | `internal/data/provider/openai_compatible.go` | 所有 Provider 统一解析 API Key、Base URL、模型和超时 |
| Chat Completions 协议适配 | `internal/data/llm/openai.go` | 所有 Provider 均通过 `github.com/sashabaranov/go-openai` 的 `CreateChatCompletion` 调用 |
| CLI 配置 | `internal/service/cli_credentials.go` | 将 CLI 凭证写入当前 Provider 子配置 |

Provider 不再拥有不同的网络调用实现。DeepSeek、Gemini、Grok、Claude 与 OpenRouter 仅提供不同默认模型/端点；所有模型与 tool calling 统一遵循 OpenAI Chat Completions 协议。

## Provider 与默认值

| `ai.provider` | 默认模型 | 默认 Base URL | Token 状态 |
| --- | --- | --- | --- |
| `deepseek` | `deepseek-v4-flash` | `https://api.deepseek.com` | 默认接入，需配置 |
| `openai` | `gpt-4o-mini` | CLI 输入或自定义兼容地址 | 可选 |
| `gemini` | `gemini-2.5-flash` | `https://generativelanguage.googleapis.com/v1beta/openai/` | 可选 |
| `grok` | `grok-3-mini` | `https://api.x.ai/v1` | 可选 |
| `claude` | `claude-sonnet-4-5` | `https://api.anthropic.com/v1` | 可选 |
| `openrouter` | `google/gemini-2.5-flash` | `https://openrouter.ai/api/v1` | 未接入 Token，不启用 |

`openrouter` 只是预留 Provider 类型和端点，不作为默认 fallback，也不提供 Token。只有明确配置 `ai.openrouter.api_key` 且将其写入 `fallback_providers` 时才会参与调用。

## 调用与降级

```text
primary provider
  -> 最多 max_retries + 1 次请求
  -> fallback_providers[0]
  -> fallback_providers[n]
  -> 返回聚合错误
```

Fallback 在实际 `GenerateContent` 请求失败时触发，而非只在客户端创建阶段。未配置密钥的 fallback 会在启动时记录 warning 并跳过；主 Provider 缺少密钥则启动/任务初始化报错，避免静默使用未预期模型。

该结构与常见的 `ClaudeProvider{client, fallbackClient}` 设计保持相同职责，但 fallback 统一置于门面层：Claude/Grok 不私自持有 OpenRouter client、不改写调用中的 model，也不需要 OpenRouter Token。这避免在多个 Provider 中复制重试、流式 goroutine 和模型映射逻辑。

当前重试对所有调用错误一视同仁；后续如需减少无效请求，可按 HTTP 状态码区分认证、配额等不可重试错误。

## 配置

默认 DeepSeek：

```yaml
ai:
  provider: deepseek
  fallback_providers: []
  max_retries: 1
  retry_delay_milliseconds: 300
  deepseek:
    api_key: ${DEEPSEEK_API_KEY}
    base_url: https://api.deepseek.com
    model: deepseek-v4-flash
```

Gemini 作为备用：

```yaml
ai:
  provider: deepseek
  fallback_providers: [gemini]
  deepseek:
    api_key: ${DEEPSEEK_API_KEY}
  gemini:
    api_key: ${GEMINI_API_KEY}
    base_url: https://generativelanguage.googleapis.com/v1beta/openai/
    model: gemini-2.5-flash
```

生产环境应通过环境注入或密钥管理系统提供 Key；不要提交真实 Token，尤其不要为 OpenRouter 创建或保存 Token，除非后续决定正式启用它。

## 扩展

优先判断新服务是否兼容 OpenAI 协议：兼容时只需在 `conf.proto` 添加配置、在 `newCompatibleProvider` 添加默认值、补齐 CLI 展示与测试；非兼容时再实现独立 `LLMProvider` 策略。这样避免重复消息和工具调用转换代码。
