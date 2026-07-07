package context

import "testing"

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
