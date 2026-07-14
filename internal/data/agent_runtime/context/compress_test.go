package context

import (
	"strings"
	"testing"
)

func TestEstimateConversationContextSize(t *testing.T) {
	turns := []ConversationTurn{
		{Role: ConversationRoleHuman, Content: strings.Repeat("a", 1000)},
		{Role: ConversationRoleTool, Content: strings.Repeat("b", 5000), ToolName: "read_file"},
	}
	if got := estimateConversationContextSize(turns); got != 6000+len("read_file") {
		t.Fatalf("size = %d, want %d", got, 6000+len("read_file"))
	}
}

func TestCompressConversationTurns(t *testing.T) {
	turns := []ConversationTurn{
		{Role: ConversationRoleHuman, Content: "start"},
		{Role: ConversationRoleTool, Content: strings.Repeat("z", 5000), ToolName: "read_file"},
	}
	out, result := CompressConversationTurns(turns, CompressConfig{
		Threshold:          1000,
		ToolOutputMaxChars: 200,
		KeepRecentTurns:    2,
	})
	if !result.Compressed {
		t.Fatal("expected compression")
	}
	if estimateConversationContextSize(out) > 1000 {
		t.Fatalf("prepared size = %d, want <= 1000", estimateConversationContextSize(out))
	}
}

func TestCompressPreservesAIToolRound(t *testing.T) {
	turns := []ConversationTurn{
		{Role: ConversationRoleHuman, Content: "q1"},
		{Role: ConversationRoleAI, ToolCalls: []ConversationToolCall{{ID: "c1", Name: "read_file", Arguments: `{}`}}},
		{Role: ConversationRoleTool, ToolCallID: "c1", ToolName: "read_file", Content: strings.Repeat("x", 120000)},
		{Role: ConversationRoleHuman, Content: "q2"},
		{Role: ConversationRoleAI, ToolCalls: []ConversationToolCall{{ID: "c2", Name: "read_file", Arguments: `{}`}}},
		{Role: ConversationRoleTool, ToolCallID: "c2", ToolName: "read_file", Content: "ok"},
	}
	cfg := CompressConfig{
		Threshold:          50000,
		ToolOutputMaxChars: 1000,
		KeepRecentTurns:    1,
	}
	out, _ := CompressConversationTurns(turns, cfg)
	for i, turn := range out {
		if turn.Role == ConversationRoleTool {
			if i == 0 || out[i-1].Role != ConversationRoleAI {
				t.Fatalf("orphan tool at %d: %+v", i, out)
			}
		}
	}
}
