package chat

import (
	"context"

	errconst "kratos-demo/internal/consts/error"
)

var ErrChatClientNotAvailable = errconst.ErrChatClientNotAvailable

// ToolDef 定义一个可供模型调用的工具/函数。
type ToolDef struct {
	Name        string
	Description string
	Parameters  any
}

// ToolCall 表示模型请求的一次工具调用。
type ToolCall struct {
	ID        string
	Type      string
	Name      string
	Arguments string
}

// ChatMessage 表示对话中的一条消息。
type ChatMessage struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
	Name       string
}

// ChatResponse 是模型对对话请求的响应。
type ChatResponse struct {
	Content   string
	ToolCalls []ToolCall
}

// ChatClient 是对话补全后端的领域抽象。
type ChatClient interface {
	Chat(ctx context.Context, messages []ChatMessage, tools []ToolDef) (*ChatResponse, error)
}

// ChatClientProvider 抽象创建 ChatClient 的工厂。
type ChatClientProvider interface {
	CreateChatClient(functionCalling bool) (ChatClient, error)
}
