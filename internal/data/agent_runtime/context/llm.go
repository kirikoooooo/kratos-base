package context

import (
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

func TurnsFromLLM(messages []llms.MessageContent) []ConversationTurn {
	turns := make([]ConversationTurn, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case llms.ChatMessageTypeSystem:
			continue
		case llms.ChatMessageTypeHuman:
			content := textFromParts(msg.Parts)
			if content == "" {
				continue
			}
			turns = append(turns, ConversationTurn{
				Role:    ConversationRoleHuman,
				Content: content,
			})
		case llms.ChatMessageTypeAI:
			turn := ConversationTurn{Role: ConversationRoleAI}
			for _, part := range msg.Parts {
				switch typed := part.(type) {
				case llms.TextContent:
					if text := strings.TrimSpace(typed.Text); text != "" {
						if turn.Content != "" {
							turn.Content += "\n"
						}
						turn.Content += text
					}
				case llms.ToolCall:
					call := ConversationToolCall{
						ID:   strings.TrimSpace(typed.ID),
						Name: "",
					}
					if typed.FunctionCall != nil {
						call.Name = strings.TrimSpace(typed.FunctionCall.Name)
						call.Arguments = strings.TrimSpace(typed.FunctionCall.Arguments)
					}
					if call.Name != "" {
						turn.ToolCalls = append(turn.ToolCalls, call)
					}
				}
			}
			if turn.Content != "" || len(turn.ToolCalls) > 0 {
				turns = append(turns, turn)
			}
		case llms.ChatMessageTypeTool:
			for _, part := range msg.Parts {
				toolResp, ok := part.(llms.ToolCallResponse)
				if !ok {
					continue
				}
				turns = append(turns, ConversationTurn{
					Role:       ConversationRoleTool,
					ToolCallID: strings.TrimSpace(toolResp.ToolCallID),
					ToolName:   strings.TrimSpace(toolResp.Name),
					Content:    strings.TrimSpace(toolResp.Content),
				})
			}
		}
	}
	return turns
}

func LLMMessagesFromTurns(turns []ConversationTurn) []llms.MessageContent {
	messages := make([]llms.MessageContent, 0, len(turns))
	for _, turn := range turns {
		switch turn.Role {
		case ConversationRoleHuman:
			content := strings.TrimSpace(turn.Content)
			if content == "" {
				continue
			}
			messages = append(messages, llms.TextParts(llms.ChatMessageTypeHuman, content))
		case ConversationRoleAI:
			parts := make([]llms.ContentPart, 0, len(turn.ToolCalls)+1)
			if content := strings.TrimSpace(turn.Content); content != "" {
				parts = append(parts, llms.TextContent{Text: content})
			}
			for idx, call := range turn.ToolCalls {
				parts = append(parts, NormalizeLLMToolCall(llms.ToolCall{
					ID: ensureToolCallID(call.ID, call.Name, idx),
					FunctionCall: &llms.FunctionCall{
						Name:      call.Name,
						Arguments: call.Arguments,
					},
				}))
			}
			if len(parts) == 0 {
				continue
			}
			messages = append(messages, llms.MessageContent{
				Role:  llms.ChatMessageTypeAI,
				Parts: parts,
			})
		case ConversationRoleTool:
			name := strings.TrimSpace(turn.ToolName)
			content := strings.TrimSpace(turn.Content)
			if name == "" && content == "" {
				continue
			}
			toolCallID := strings.TrimSpace(turn.ToolCallID)
			if toolCallID == "" {
				toolCallID = ensureToolCallID("", name, 0)
			}
			messages = append(messages, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{llms.ToolCallResponse{
					ToolCallID: toolCallID,
					Name:       name,
					Content:    content,
				}},
			})
		}
	}
	return messages
}

func textFromParts(parts []llms.ContentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		text, ok := part.(llms.TextContent)
		if !ok {
			continue
		}
		if line := strings.TrimSpace(text.Text); line != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(line)
		}
	}
	return builder.String()
}

func NormalizeLLMToolCall(call llms.ToolCall) llms.ToolCall {
	if strings.TrimSpace(call.Type) == "" {
		call.Type = "function"
	}
	name := ""
	if call.FunctionCall != nil {
		name = call.FunctionCall.Name
	}
	call.ID = ensureToolCallID(call.ID, name, 0)
	return call
}

func ensureToolCallID(id, toolName string, index int) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "tool"
	}
	return fmt.Sprintf("call-%s-%d", toolName, index)
}

func BuildLLMMessages(systemText string, turns []ConversationTurn) []llms.MessageContent {
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, strings.TrimSpace(systemText)),
	}
	messages = append(messages, LLMMessagesFromTurns(turns)...)
	return messages
}
