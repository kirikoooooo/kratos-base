package biz

import (
	"strings"
	"testing"
)

func TestEstimateConversationContextSize(t *testing.T) {
	turns := []ConversationTurn{
		{Role: ConversationRoleHuman, Content: strings.Repeat("a", 1000)},
		{Role: ConversationRoleTool, Content: strings.Repeat("b", 5000), ToolName: "read_file"},
	}
	if got := EstimateConversationContextSize(turns); got != 6000+len("read_file") {
		t.Fatalf("size = %d, want %d", got, 6000+len("read_file"))
	}
}

func TestCompressConversationTurnsWhenBelowThreshold(t *testing.T) {
	turns := []ConversationTurn{{Role: ConversationRoleHuman, Content: "hello"}}
	out, result := CompressConversationTurns(turns, ContextCompressConfig{Threshold: 200000})
	if result.Compressed {
		t.Fatal("expected no compression")
	}
	if len(out) != 1 || out[0].Content != "hello" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestCompressConversationTurnsTruncatesToolOutput(t *testing.T) {
	turns := []ConversationTurn{
		{Role: ConversationRoleHuman, Content: "task"},
		{Role: ConversationRoleTool, Content: strings.Repeat("x", 30000), ToolName: "read_file"},
	}
	cfg := ContextCompressConfig{
		Threshold:          20000,
		ToolOutputMaxChars: 1000,
		KeepRecentTurns:    8,
	}
	out, result := CompressConversationTurns(turns, cfg)
	if !result.Compressed {
		t.Fatal("expected compression")
	}
	if EstimateConversationContextSize(out) > cfg.Threshold {
		t.Fatalf("compressed size still too large: %d", EstimateConversationContextSize(out))
	}
	if !strings.Contains(out[1].Content, "truncated") {
		t.Fatalf("tool output not truncated: %q", out[1].Content)
	}
}

func TestCompressConversationTurnsDisabled(t *testing.T) {
	turns := []ConversationTurn{
		{Role: ConversationRoleHuman, Content: strings.Repeat("a", 300000)},
	}
	out, result := CompressConversationTurns(turns, ContextCompressConfig{Threshold: -1})
	if result.Compressed {
		t.Fatal("expected compression disabled")
	}
	if len(out[0].Content) != 300000 {
		t.Fatalf("content mutated when disabled")
	}
}

func TestPrepareTurnsForLLMIntegration(t *testing.T) {
	uc := NewAgentMemoryUsecase(nil, AgentMemoryConfig{
		ContextCompressThreshold: 1000,
		ToolOutputMaxChars:       200,
		KeepRecentTurns:          2,
	})
	turns := []ConversationTurn{
		{Role: ConversationRoleHuman, Content: "start"},
		{Role: ConversationRoleTool, Content: strings.Repeat("z", 5000)},
	}
	prepared := uc.PrepareTurnsForLLM(nil, "session-1", turns)
	if EstimateConversationContextSize(prepared) > 1000 {
		t.Fatalf("prepared size = %d, want <= 1000", EstimateConversationContextSize(prepared))
	}
}
