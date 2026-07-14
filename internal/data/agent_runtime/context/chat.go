package context

import (
	"fmt"
	"strings"

	bizchat "kratos-demo/internal/biz/chat"
)

// BuildChatMessages 构建包含 system prompt 的完整消息列表。
func BuildChatMessages(systemText string, turns []ConversationTurn) []bizchat.ChatMessage {
	messages := []bizchat.ChatMessage{
		{Role: "system", Content: strings.TrimSpace(systemText)},
	}
	messages = append(messages, ChatMessagesFromTurns(turns)...)
	return messages
}

// ChatMessagesFromTurns 将 ConversationTurn 切片转为 ChatMessage 切片。
func ChatMessagesFromTurns(turns []ConversationTurn) []bizchat.ChatMessage {
	messages := make([]bizchat.ChatMessage, 0, len(turns))
	for _, turn := range turns {
		switch turn.Role {
		case ConversationRoleHuman:
			content := strings.TrimSpace(turn.Content)
			if content == "" {
				continue
			}
			messages = append(messages, bizchat.ChatMessage{
				Role:    "user",
				Content: content,
			})
		case ConversationRoleAI:
			parts := make([]bizchat.ToolCall, 0, len(turn.ToolCalls))
			for idx, call := range turn.ToolCalls {
				tcID := strings.TrimSpace(call.ID)
				if tcID == "" {
					tcID = ensureToolCallID("", call.Name, idx)
				}
				parts = append(parts, bizchat.ToolCall{
					ID:        tcID,
					Type:      "function",
					Name:      call.Name,
					Arguments: call.Arguments,
				})
			}
			content := strings.TrimSpace(turn.Content)
			if content == "" && len(parts) == 0 {
				continue
			}
			messages = append(messages, bizchat.ChatMessage{
				Role:      "assistant",
				Content:   content,
				ToolCalls: parts,
			})
		case ConversationRoleTool:
			name := strings.TrimSpace(turn.ToolName)
			content := strings.TrimSpace(turn.Content)
			if name == "" && content == "" {
				continue
			}
			tcID := strings.TrimSpace(turn.ToolCallID)
			if tcID == "" {
				tcID = ensureToolCallID("", name, 0)
			}
			messages = append(messages, bizchat.ChatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: tcID,
				Name:       name,
			})
		}
	}
	return messages
}

// TurnsFromChatMessages 将 ChatMessage 切片转回 ConversationTurn 切片。
func TurnsFromChatMessages(messages []bizchat.ChatMessage) []ConversationTurn {
	turns := make([]ConversationTurn, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		switch role {
		case "system":
			continue
		case "user":
			content := strings.TrimSpace(msg.Content)
			if content == "" {
				continue
			}
			turns = append(turns, ConversationTurn{
				Role:    ConversationRoleHuman,
				Content: content,
			})
		case "assistant":
			turn := ConversationTurn{
				Role:    ConversationRoleAI,
				Content: strings.TrimSpace(msg.Content),
			}
			for _, tc := range msg.ToolCalls {
				turn.ToolCalls = append(turn.ToolCalls, ConversationToolCall{
					ID:        strings.TrimSpace(tc.ID),
					Name:      strings.TrimSpace(tc.Name),
					Arguments: strings.TrimSpace(tc.Arguments),
				})
			}
			if turn.Content == "" && len(turn.ToolCalls) == 0 {
				continue
			}
			turns = append(turns, turn)
		case "tool":
			turns = append(turns, ConversationTurn{
				Role:       ConversationRoleTool,
				ToolCallID: strings.TrimSpace(msg.ToolCallID),
				ToolName:   strings.TrimSpace(msg.Name),
				Content:    strings.TrimSpace(msg.Content),
			})
		}
	}
	return turns
}

// AppendChatAssistantMessage 构造一条 assistant 消息（含可选的工具调用）。
func AppendChatAssistantMessage(messages []bizchat.ChatMessage, content string, toolCalls []bizchat.ToolCall) []bizchat.ChatMessage {
	parts := make([]bizchat.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		parts = append(parts, bizchat.ToolCall{
			ID:        strings.TrimSpace(tc.ID),
			Type:      "function",
			Name:      strings.TrimSpace(tc.Name),
			Arguments: strings.TrimSpace(tc.Arguments),
		})
	}
	return append(messages, bizchat.ChatMessage{
		Role:      "assistant",
		Content:   strings.TrimSpace(content),
		ToolCalls: parts,
	})
}

// AppendChatToolResult 构造一条 tool 结果消息。
func AppendChatToolResult(messages []bizchat.ChatMessage, toolCallID, toolName, content string) []bizchat.ChatMessage {
	return append(messages, bizchat.ChatMessage{
		Role:       "tool",
		Content:    strings.TrimSpace(content),
		ToolCallID: strings.TrimSpace(toolCallID),
		Name:       strings.TrimSpace(toolName),
	})
}

// AppendChatUserMessage 构造一条 user 消息。
func AppendChatUserMessage(messages []bizchat.ChatMessage, content string) []bizchat.ChatMessage {
	return append(messages, bizchat.ChatMessage{
		Role:    "user",
		Content: strings.TrimSpace(content),
	})
}

// ensureChatToolCallID 保证工具调用 ID 不为空。
func ensureChatToolCallID(id, toolName string, index int) string {
	return ensureToolCallID(id, toolName, index)
}

// FormatToolErrorObservationForChat 格式化工具错误观察（使用 neutral 类型）。
func FormatToolErrorObservationForChat(toolName, output string, err error, attempt, maxAttempts int) string {
	return fmt.Sprintf(
		"status: error\n"+
			"tool: %s\n"+
			"attempt: %d/%d\n"+
			"error: %s\n"+
			"请根据这个错误修正参数、路径或工具选择后重试。",
		strings.TrimSpace(toolName),
		attempt,
		maxAttempts,
		strings.TrimSpace(err.Error()),
	)
}
