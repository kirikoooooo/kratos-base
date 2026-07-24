package context

import (
	"testing"

	lmm "kratos-demo/internal/biz/llm"
)

func TestConversationTurnsFromLLMRoundTrip(t *testing.T) {
	messages := []lmm.MessageContent{
		lmm.TextParts(lmm.RoleSystem, "system"),
		lmm.TextParts(lmm.RoleUser, "hello"),
		{
			Role: lmm.RoleAssistant,
			Parts: []lmm.ContentPart{
				lmm.ToolCallPart{
					ID: "call-1",
					FunctionCall: &lmm.FunctionCall{
						Name:      "read_file",
						Arguments: `{"path":"README.md"}`,
					},
				},
			},
		},
		{
			Role: lmm.RoleTool,
			Parts: []lmm.ContentPart{lmm.ToolCallResponse{
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
	if restored[0].Role != lmm.RoleUser {
		t.Fatalf("first restored role = %s, want human", restored[0].Role)
	}
	if restored[1].Role != lmm.RoleAssistant {
		t.Fatalf("second restored role = %s, want ai", restored[1].Role)
	}
	toolCall, ok := restored[1].Parts[0].(lmm.ToolCallPart)
	if !ok {
		t.Fatalf("expected tool call part, got %T", restored[1].Parts[0])
	}
	if toolCall.Type != "function" {
		t.Fatalf("tool call type = %q, want function", toolCall.Type)
	}
	if restored[2].Role != lmm.RoleTool {
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
