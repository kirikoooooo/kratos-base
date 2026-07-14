package context

import (
	"testing"

	"github.com/tmc/langchaingo/llms"
)

func TestConversationTurnsFromLLMRoundTrip(t *testing.T) {
	messages := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, "system"),
		llms.TextParts(llms.ChatMessageTypeHuman, "hello"),
		{
			Role: llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{
				llms.ToolCall{
					ID: "call-1",
					FunctionCall: &llms.FunctionCall{
						Name:      "read_file",
						Arguments: `{"path":"README.md"}`,
					},
				},
			},
		},
		{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{llms.ToolCallResponse{
				ToolCallID: "call-1",
				Name:       "read_file",
				Content:    "ok",
			}},
		},
	}

	turns := TurnsFromLLM(messages)
	if len(turns) != 3 {
		t.Fatalf("turn count = %d, want 3", len(turns))
	}
	restored := LLMMessagesFromTurns(turns)
	if len(restored) != 3 {
		t.Fatalf("restored message count = %d, want 3", len(restored))
	}
	if restored[0].Role != llms.ChatMessageTypeHuman {
		t.Fatalf("first restored role = %s, want human", restored[0].Role)
	}
	if restored[1].Role != llms.ChatMessageTypeAI {
		t.Fatalf("second restored role = %s, want ai", restored[1].Role)
	}
	toolCall, ok := restored[1].Parts[0].(llms.ToolCall)
	if !ok {
		t.Fatalf("expected tool call part, got %T", restored[1].Parts[0])
	}
	if toolCall.Type != "function" {
		t.Fatalf("tool call type = %q, want function", toolCall.Type)
	}
	if restored[2].Role != llms.ChatMessageTypeTool {
		t.Fatalf("third restored role = %s, want tool", restored[2].Role)
	}
}

func TestBuildLLMMessagesUsesConversationTurns(t *testing.T) {
	turns := []ConversationTurn{
		{Role: ConversationRoleHuman, Content: "read readme"},
	}
	messages := BuildLLMMessages("sys", turns)
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if textFromParts(messages[1].Parts) != "read readme" {
		t.Fatalf("human content = %q", textFromParts(messages[1].Parts))
	}
}
