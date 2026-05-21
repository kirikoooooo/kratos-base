package biz

import (
	"strings"
	"testing"
)

func TestAppendHumanTurnIfNeeded(t *testing.T) {
	turns := []ConversationTurn{{Role: ConversationRoleHuman, Content: "first"}}
	out := AppendHumanTurnIfNeeded(turns, "second")
	if len(out) != 2 {
		t.Fatalf("turns = %d, want 2", len(out))
	}
	dup := AppendHumanTurnIfNeeded(out, "second")
	if len(dup) != 2 {
		t.Fatalf("duplicate append changed length: %d", len(dup))
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
	cfg := ContextCompressConfig{
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
