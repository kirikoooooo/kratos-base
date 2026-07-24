package memory

import (
	"strings"
	"testing"
)

func TestFormatAgentMemoryForPrompt(t *testing.T) {
	user := &UserAgentMemory{
		UserID:        "default",
		WorkspaceRoot: "D:/code/kratos-base",
		ToolHints: []ToolHint{{
			Name:      "read_file",
			WhenToUse: "read docs first",
		}},
		SkillHints: []SkillHint{{Name: "systematic-debugging"}},
		CommandPolicies: []CommandPolicy{{
			Situation: "verify go build",
			Commands:  []string{"go build ./..."},
		}},
	}
	session := &SessionAgentMemory{
		SessionID: "task-1",
		Agent:     "default",
		ToolsUsed: []string{"read_file"},
		PromptNotes: []PromptAdjustment{{
			Title:   "path",
			Content: "prefer relative paths",
		}},
	}

	got := formatAgentMemoryForPrompt(user, session)
	for _, want := range []string{
		"?????",
		"read_file",
		"search_skills",
		"go build ./...",
		"?????",
		"????????: read_file",
		"prefer relative paths",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt context missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "systematic-debugging") {
		t.Fatalf("skill catalog leaked into prompt context:\n%s", got)
	}
}
