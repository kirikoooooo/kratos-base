// Package chat 提供 LLM 对话客户端的实现（langchaingo 适配器与 OpenAI 原生 SDK）。
// 领域抽象定义在 internal/biz/chat 中。
package chat

import bizchat "kratos-demo/internal/biz/chat"

// Re-export domain types.
type (
	ToolDef     = bizchat.ToolDef
	ToolCall    = bizchat.ToolCall
	ChatMessage = bizchat.ChatMessage
	ChatClient  = bizchat.ChatClient
)

// ChatResponse is re-exported from biz/chat.
type ChatResponse = bizchat.ChatResponse
