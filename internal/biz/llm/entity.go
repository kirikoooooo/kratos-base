// Package llm 定义 LLM 调用的中性类型，不依赖任何第三方 LLM SDK。
package llm

import (
	"context"
	"strings"

	"github.com/openai/openai-go/responses"
)

// Tool is the official OpenAI Responses API tool union.
type Tool = responses.ToolUnionParam

// ——— ContentPart 联合类型 ———

// ContentPart 表示消息中的一个内容片段（文本、工具调用或工具响应）。
type ContentPart interface {
	isLLMContentPart()
}

// TextContent 纯文本内容。
type TextContent struct {
	Text string
}

func (TextContent) isLLMContentPart() {}

// ToolCallPart 表示模型请求的一次工具调用（出现在 assistant 消息中）。
type ToolCallPart struct {
	ID           string        // 工具调用 ID
	Type         string        // 通常为 "function"
	FunctionCall *FunctionCall // 调用的函数信息
}

func (ToolCallPart) isLLMContentPart() {}

// FunctionCall 函数调用的名称和参数。
type FunctionCall struct {
	Name      string // 函数名
	Arguments string // JSON 格式的参数
}

// ToolCallResponse 表示工具执行的返回结果（出现在 tool 消息中）。
type ToolCallResponse struct {
	ToolCallID string // 对应的工具调用 ID
	Name       string // 工具名
	Content    string // 工具执行结果
}

func (ToolCallResponse) isLLMContentPart() {}

// ——— 消息 ———

// MessageRole 消息角色。
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// MessageContent 表示一条完整的对话消息。
type MessageContent struct {
	Role  MessageRole
	Parts []ContentPart
}

// ——— ModelClient 接口 ———

// ModelClient 抽象 LLM 调用，for model backends。
type ModelClient interface {
	// GenerateContent 生成回复，支持多轮对话和工具调用。
	GenerateContent(ctx context.Context, messages []MessageContent, opts ...CallOption) (*ContentResponse, error)
	// Call 简单的单轮文本补全。
	Call(ctx context.Context, prompt string, opts ...CallOption) (string, error)
}

// ——— CallOption ———

// CallOption 模型调用选项。
type CallOption func(*CallConfig)

// CallConfig 聚合调用选项（导出供适配层使用）。
type CallConfig struct {
	Tools       []Tool
	ToolChoice  string
	Temperature float64
	HasTemp     bool
}

// WithTools 设置可用工具列表。
func WithTools(tools []Tool) CallOption {
	return func(c *CallConfig) {
		c.Tools = tools
	}
}

// WithToolChoice 设置工具选择策略（如 "auto"）。
func WithToolChoice(choice string) CallOption {
	return func(c *CallConfig) {
		c.ToolChoice = choice
	}
}

// WithTemperature 设置采样温度。
func WithTemperature(temp float64) CallOption {
	return func(c *CallConfig) {
		c.Temperature = temp
		c.HasTemp = true
	}
}

// ——— 响应 ———

// ContentResponse 模型生成的响应。
type ContentResponse struct {
	Choices      []*ContentChoice
	Model        string
	InputTokens  int
	OutputTokens int
}

// ContentChoice 单个回复选项。
type ContentChoice struct {
	Content   string         // 文本回复
	ToolCalls []ToolCallPart // 工具调用（assistant 角色）
	FuncCall  *FunctionCall  // 旧版 function call（兼容）
}

// ——— 便捷构造 ———

// TextParts 创建一条纯文本消息。
func TextParts(role MessageRole, text string) MessageContent {
	return MessageContent{
		Role:  role,
		Parts: []ContentPart{TextContent{Text: text}},
	}
}

// GenerateFromSinglePrompt 单轮文本生成（替代 llms.GenerateFromSinglePrompt）。
func GenerateFromSinglePrompt(ctx context.Context, model ModelClient, prompt string, opts ...CallOption) (string, error) {
	msg := []MessageContent{
		TextParts(RoleUser, prompt),
	}
	resp, err := model.GenerateContent(ctx, msg, opts...)
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(resp.Choices[0].Content), nil
}
