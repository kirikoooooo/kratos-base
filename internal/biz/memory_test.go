package biz

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
			WhenToUse: "先读 README 与相关源码再修改",
		}},
		SkillHints: []SkillHint{{
			Name:      "systematic-debugging",
			Path:      ".agents/skills/debugging/systematic-debugging/SKILL.md",
			WhenToUse: "排查失败任务或工具错误时",
		}},
		CommandPolicies: []CommandPolicy{{
			Situation: "验证 Go 编译",
			Commands:  []string{"go build ./..."},
		}},
	}
	session := &SessionAgentMemory{
		SessionID: "task-1",
		Agent:     "default",
		ToolsUsed: []string{"read_file"},
		PromptNotes: []PromptAdjustment{{
			Title:   "路径",
			Content: "Windows 工作区优先使用相对路径",
		}},
	}

	got := FormatAgentMemoryForPrompt(user, session)
	for _, want := range []string{
		"用户级记忆",
		"read_file",
		"systematic-debugging",
		"go build ./...",
		"会话级记忆",
		"本会话已使用工具: read_file",
		"Windows 工作区",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt context missing %q:\n%s", want, got)
		}
	}
}
