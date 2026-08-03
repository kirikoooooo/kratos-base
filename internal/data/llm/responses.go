// Package llm provides LLM client implementations using the official OpenAI Go SDK.
//
// This file implements ModelClient with the official OpenAI Responses API SDK.
//
// Responses API 与 Chat Completions API 的核心差异：
//
//  1. 请求结构：Chat Completions 使用 messages 数组，每条消息嵌套 tool_calls；
//     Responses API 使用 input 数组，所有条目（message、function_call、function_call_output）
//     平铺在同一层，不再是嵌套结构。
//
//  2. 对话状态：Chat Completions 每次需传完整历史；Responses API 可通过 previous_response_id
//     自动携带上下文，无需重复发送历史消息。
//
//  3. 响应结构：Chat Completions 返回 choices[0].message.content + tool_calls；
//     Responses API 返回 output 数组，text 在 type="message" 的条目中，
//     工具调用在 type="function_call" 的条目中，一一区分。
//
//  4. 工具定义：Responses API 的工具统一包装在 ToolUnionParam 的 OfFunction 字段中，
//     支持 strict mode、web_search、file_search、code_interpreter 等多种内置工具。
//
// 参考：https://platform.openai.com/docs/guides/conversation-state
package llm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	bizllm "kratos-demo/internal/biz/llm"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared/constant"
)

// ResponsesModelConfig 创建 Responses API ModelClient 的配置。
type ResponsesModelConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// NewResponsesModelClient creates a ModelClient backed by OpenAI Responses API (official SDK).
func NewResponsesModelClient(config ResponsesModelConfig) bizllm.ModelClient {
	model := strings.TrimSpace(config.Model)
	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
	}
	if strings.TrimSpace(config.BaseURL) != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimSpace(config.BaseURL)))
	}
	if config.Timeout > 0 {
		opts = append(opts, option.WithHTTPClient(responsesHTTPClient(config.Timeout)))
	}
	return &responsesModelClient{
		// openai.NewClient 返回的是值类型（struct），不是指针，所以字段类型为 openai.Client 而非 *openai.Client。
		client: openai.NewClient(opts...),
		model:  model,
	}
}

// responsesHTTPClient 构造带超时控制的 HTTP 客户端（与 openai.go 中 httpClient 逻辑一致）。
func responsesHTTPClient(timeout time.Duration) *http.Client {
	dialTimeout := 30 * time.Second
	if timeout < dialTimeout {
		dialTimeout = timeout
	}
	tlsTimeout := 30 * time.Second
	if timeout < tlsTimeout {
		tlsTimeout = timeout
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   dialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   tlsTimeout,
			ResponseHeaderTimeout: timeout,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          16,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

type responsesModelClient struct {
	client openai.Client // 值类型，非指针（NewClient 返回 struct）
	model  string
}

// GenerateContent 调用 Responses API 生成回复。
//
// 内部流程：
//  1. 将中性消息（[]MessageContent）转换为 Responses API 的 input 数组
//  2. 构建 ResponseNewParams，设置 model、input、tools、tool_choice、temperature
//  3. 调用 c.client.Responses.New()，即 POST /v1/responses
//  4. 将 *responses.Response 转回中性的 *ContentResponse
func (c *responsesModelClient) GenerateContent(ctx context.Context, messages []bizllm.MessageContent, opts ...bizllm.CallOption) (*bizllm.ContentResponse, error) {
	cc := &bizllm.CallConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(cc)
	}

	// ResponseNewParams 是 Responses API 的请求体。
	// Input 字段是一个 Union 类型：可以是 string（单条 prompt）或 InputItemList（结构化消息数组）。
	// 我们始终使用 InputItemList 模式以支持多轮对话和工具调用。
	req := responses.ResponseNewParams{
		Model: c.model,                    // Responses API 的 model 类型是 shared.ResponsesModel（即 string 别名）
		Input: toResponsesInput(messages), // 将中性消息转为 Responses API 的 input 数组
	}

	// Tools are already official Responses SDK union values.
	if len(cc.Tools) > 0 {
		req.Tools = cc.Tools
		req.ToolChoice = toResponsesToolChoice(cc.ToolChoice)
	}

	// param.NewOpt 将 Go 原生值包装为 SDK 的 Optional 类型。
	// 只有显式设置过的 Optional 字段才会被序列化到 JSON 中（omitempty 语义）。
	if cc.HasTemp {
		req.Temperature = param.NewOpt(cc.Temperature)
	}

	// c.client.Responses.New() → POST /v1/responses
	resp, err := c.client.Responses.New(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai create response: %w", err)
	}
	return fromResponsesResponse(resp), nil
}

// Call 单轮文本补全，等价于 GenerateContent 的简化调用。
func (c *responsesModelClient) Call(ctx context.Context, prompt string, opts ...bizllm.CallOption) (string, error) {
	messages := []bizllm.MessageContent{
		bizllm.TextParts(bizllm.RoleUser, prompt),
	}
	resp, err := c.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(resp.Choices[0].Content), nil
}

// ============================================================================
// 输入转换：中性消息 → Responses API input 数组
// ============================================================================

// toResponsesInput 将中性消息列表转换为 Responses API 的 input 参数。
//
// Responses API 的 input 是一个平铺的数组（ResponseInputParam = []ResponseInputItemUnionParam），
// 每条 item 都独立表达——这与 Chat Completions API 中 messages 嵌套 tool_calls 的结构完全不同。
//
// 转换规则：
//   - system/user 消息 → OfMessage（EasyInputMessageParam），content 为文本
//   - assistant 消息 → 如果有文本则生成 OfMessage，如果有 tool_calls 则每个 call 生成独立的 OfFunctionCall
//   - tool 消息   → 每条 ToolCallResponse 生成独立的 OfFunctionCallOutput
//
// 示例：一轮带工具调用的对话在 Chat Completions 中是 4 条 messages，在 Responses API 中会变成 5 条 input items：
//
//	Chat Completions:                         Responses API:
//	  user: "天气怎么样？"              →       message(user, "天气怎么样？")
//	  assistant: (tool_call id=1)      →       function_call(id=1, get_weather, {city: "北京"})
//	  tool: (id=1, "晴天 25°C")        →       function_call_output(id=1, "晴天 25°C")
//	  assistant: "北京今天晴天..."      →       message(assistant, "北京今天晴天...")
//
// toResponsesInput 返回 ResponseNewParamsInputUnion（Union 类型），而非裸的 ResponseInputParam。
// 这是因为 ResponseNewParams.Input 字段本身是 Union 类型：它既可以是 string，也可以是 ItemList。
// 我们固定使用 OfInputItemList 变体。
func toResponsesInput(messages []bizllm.MessageContent) responses.ResponseNewParamsInputUnion {
	var input responses.ResponseInputParam
	for _, msg := range messages {
		switch msg.Role {
		case bizllm.RoleSystem:
			// System 消息在 Responses API 中直接作为 message(role="system") 放入 input。
			input = append(input, systemMessage(textFromParts(msg.Parts)))
		case bizllm.RoleUser:
			input = append(input, userMessage(textFromParts(msg.Parts)))
		case bizllm.RoleAssistant:
			// Assistant 消息在 Responses API 中需要拆分：
			// 1. 文本内容 → message(role="assistant")
			// 2. 工具调用 → 每个调用是一个独立的 function_call item
			// 这是因为 Responses API 中 function_call 是独立的 input item 类型，而非 message 的子属性。
			text := textFromParts(msg.Parts)
			if text != "" {
				input = append(input, assistantMessage(text))
			}
			for _, tc := range toolCallsFromPartsResponses(msg.Parts) {
				input = append(input, functionCallItem(tc.ID, tc.FunctionCall.Name, tc.FunctionCall.Arguments))
			}
		case bizllm.RoleTool:
			// Tool 消息在 Responses API 中对应 function_call_output item。
			// 每条 BizToolCallResponse 生成一个独立的 function_call_output。
			for _, part := range msg.Parts {
				toolResp, ok := part.(bizllm.ToolCallResponse)
				if !ok {
					continue
				}
				input = append(input, functionCallOutputItem(toolResp.ToolCallID, toolResp.Content))
			}
		}
	}
	// 将构建好的 input 数组包装为 Union 类型的 OfInputItemList 变体。
	return responses.ResponseNewParamsInputUnion{
		OfInputItemList: input,
	}
}

func textFromParts(parts []bizllm.ContentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		text, ok := part.(bizllm.TextContent)
		if !ok {
			continue
		}
		if value := strings.TrimSpace(text.Text); value != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(value)
		}
	}
	return builder.String()
}

// systemMessage 构造 system 角色的 input message。
// 使用 SDK 提供的便捷函数 ResponseInputItemParamOfMessage。
func systemMessage(content string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleSystem)
}

// userMessage 构造 user 角色的 input message。
func userMessage(content string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser)
}

// assistantMessage 构造 assistant 角色的 input message。
func assistantMessage(content string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleAssistant)
}

// functionCallItem 构造 function_call 类型的 input item。
//
// 注意：在 Responses API 中，工具调用作为独立的 item 出现在 input 数组中，
// 类型为 ResponseFunctionToolCallParam（而非嵌套在 assistant message 内部）。
// Type 必须为 "function_call"。
func functionCallItem(id, name, arguments string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemUnionParam{
		OfFunctionCall: &responses.ResponseFunctionToolCallParam{
			CallID:    id,        // 工具调用 ID，与 Chat Completions 中的 tool_call_id 对应
			Name:      name,      // 函数名
			Arguments: arguments, // JSON 格式的函数参数
			Type:      constant.FunctionCall("function_call"),
		},
	}
}

// functionCallOutputItem 构造 function_call_output 类型的 input item。
//
// 对应 Chat Completions 中 role="tool" 的消息。
// callID 必须与之前 function_call item 的 call_id 匹配，OpenAI 据此关联调用和结果。
func functionCallOutputItem(callID, output string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemParamOfFunctionCallOutput(callID, output)
}

// ============================================================================
// toResponsesToolChoice 将 tool_choice 字符串转为 Responses API 的 ToolChoice 联合类型。
//
// Responses API 的 tool_choice 比 Chat Completions 更丰富，支持以下模式（通过联合类型表达）：
//   - OfToolChoiceMode：预设模式（"auto"/"required"/"none"）
//   - OfHostedTool：指定使用某个内置工具（如 web_search）
//   - OfFunctionTool：指定调用某个具体函数
//   - OfMcpTool：指定 MCP 工具
//
// 当前实现支持 OfToolChoiceMode 的三种预设，自定义函数选择暂退化为 auto。
func toResponsesToolChoice(choice string) responses.ResponseNewParamsToolChoiceUnion {
	switch choice {
	case "auto", "":
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	case "required":
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsRequired),
		}
	case "none":
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsNone),
		}
	default:
		// 自定义 tool_choice（如通过 {"type":"function","name":"xxx"} 指定函数名）暂未实现，退化为 auto。
		return responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	}
}

// ============================================================================
// 响应转换：Responses API 响应 → 中性 ContentResponse
// ============================================================================

// fromResponsesResponse 将 Responses API 的响应转为中性 ContentResponse。
//
// Responses API 的响应结构与 Chat Completions 不同：
//
// Chat Completions:
//
//	{
//	  "choices": [{
//	    "message": {
//	      "content": "回复文本",
//	      "tool_calls": [{...}]
//	    }
//	  }]
//	}
//
// Responses API:
//
//	{
//	  "output": [
//	    {"type": "message", "content": [{"type": "output_text", "text": "回复文本"}]},
//	    {"type": "function_call", "call_id": "xxx", "name": "get_weather", "arguments": "{...}"}
//	  ],
//	  "usage": {"input_tokens": 100, "output_tokens": 50}
//	}
//
// 转换规则：
//   - output 中 type="message" 的条目 → Choices[0].Content（多个 content 拼接为一段文本）
//   - output 中 type="function_call" 的条目 → Choices[0].ToolCalls
//   - usage.input_tokens / output_tokens → InputTokens / OutputTokens
func fromResponsesResponse(resp *responses.Response) *bizllm.ContentResponse {
	if resp == nil {
		return &bizllm.ContentResponse{}
	}
	cr := &bizllm.ContentResponse{
		Model:        string(resp.Model),
		InputTokens:  int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
	}

	var textBuilder strings.Builder
	var toolCalls []bizllm.ToolCallPart

	// 遍历 output 数组，按类型分别处理。
	// Responses API 的 output 顺序即是模型生成顺序：通常 message → function_call → function_call_output → ...
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			// message 类型条目：包含 assistant 回复文本。
			// Content 是一个数组，每个元素可能是 output_text 或 refusal。
			// 我们将所有 output_text 的 text 字段拼接为最终回复文本。
			for _, content := range item.Content {
				if line := strings.TrimSpace(content.Text); line != "" {
					if textBuilder.Len() > 0 {
						textBuilder.WriteString("\n")
					}
					textBuilder.WriteString(line)
				}
			}
		case "function_call":
			// function_call 类型条目：模型请求调用一个函数。
			// CallID 和 Chat Completions 中的 tool_call.id 是对等的。
			// Name 和 Arguments 与 Chat Completions 中的 function.name / function.arguments 一致。
			toolCalls = append(toolCalls, bizllm.ToolCallPart{ID: item.CallID, Type: "function", FunctionCall: &bizllm.FunctionCall{Name: item.Name, Arguments: item.Arguments}})
		}
		// 其他类型（reasoning、file_search_call、web_search_call 等）暂不处理，
		// 不会出现在纯 function calling 场景中。
	}

	cr.Choices = []*bizllm.ContentChoice{{
		Content:   textBuilder.String(),
		ToolCalls: toolCalls,
	}}
	return cr
}

// toolCallsFromPartsResponses 从 ContentPart 列表中提取 ToolCallPart。
// 与 openai.go 中 toolCallsFromParts 功能相同，但操作的是 bizllm.ToolCallPart 而非 go-openai 类型。
func toolCallsFromPartsResponses(parts []bizllm.ContentPart) []bizllm.ToolCallPart {
	var out []bizllm.ToolCallPart
	for _, part := range parts {
		tc, ok := part.(bizllm.ToolCallPart)
		if !ok {
			continue
		}
		out = append(out, tc)
	}
	return out
}
