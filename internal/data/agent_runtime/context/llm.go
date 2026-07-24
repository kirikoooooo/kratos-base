package context

import (
	"fmt"
	"strings"

	lmm "kratos-demo/internal/biz/llm"
)

func TurnsFromLLM(messages []lmm.MessageContent) []ConversationTurn {
	turns := make([]ConversationTurn, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case lmm.RoleSystem:
			continue
		case lmm.RoleUser:
			content := textFromParts(msg.Parts)
			if content == "" {
				continue
			}
			turns = append(turns, ConversationTurn{
				Role:    ConversationRoleHuman,
				Content: content,
			})
		case lmm.RoleAssistant:
			turn := ConversationTurn{Role: ConversationRoleAI}
			for _, part := range msg.Parts {
				switch typed := part.(type) {
				case lmm.TextContent:
					if text := strings.TrimSpace(typed.Text); text != "" {
						if turn.Content != "" {
							turn.Content += "\n"
						}
						turn.Content += text
					}
				case lmm.ToolCallPart:
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
		case lmm.RoleTool:
			for _, part := range msg.Parts {
				toolResp, ok := part.(lmm.ToolCallResponse)
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

func LLMMessagesFromTurns(turns []ConversationTurn) []lmm.MessageContent {
	messages := make([]lmm.MessageContent, 0, len(turns))
	for _, turn := range turns {
		switch turn.Role {
		case ConversationRoleHuman:
			content := strings.TrimSpace(turn.Content)
			if content == "" {
				continue
			}
			messages = append(messages, lmm.TextParts(lmm.RoleUser, content))
		case ConversationRoleAI:
			parts := make([]lmm.ContentPart, 0, len(turn.ToolCalls)+1)
			if content := strings.TrimSpace(turn.Content); content != "" {
				parts = append(parts, lmm.TextContent{Text: content})
			}
			for idx, call := range turn.ToolCalls {
				parts = append(parts, NormalizeLLMToolCall(lmm.ToolCallPart{
					ID: ensureToolCallID(call.ID, call.Name, idx),
					FunctionCall: &lmm.FunctionCall{
						Name:      call.Name,
						Arguments: call.Arguments,
					},
				}))
			}
			if len(parts) == 0 {
				continue
			}
			messages = append(messages, lmm.MessageContent{
				Role:  lmm.RoleAssistant,
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
			messages = append(messages, lmm.MessageContent{
				Role: lmm.RoleTool,
				Parts: []lmm.ContentPart{lmm.ToolCallResponse{
					ToolCallID: toolCallID,
					Name:       name,
					Content:    content,
				}},
			})
		}
	}
	return messages
}

func textFromParts(parts []lmm.ContentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		text, ok := part.(lmm.TextContent)
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

func NormalizeLLMToolCall(call lmm.ToolCallPart) lmm.ToolCallPart {
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

func BuildLLMMessages(systemText string, turns []ConversationTurn) []lmm.MessageContent {
	messages := []lmm.MessageContent{
		lmm.TextParts(lmm.RoleSystem, strings.TrimSpace(systemText)),
	}
	messages = append(messages, LLMMessagesFromTurns(turns)...)
	return messages
}
