package memory

import (
	"strings"

	"kratos-demo/internal/data/common"
)

func previewPrompt(prompt string) string {
	return common.PreviewPrompt(prompt)
}

func formatAgentMemoryForPrompt(user *UserAgentMemory, session *SessionAgentMemory) string {
	var sections []string

	if user != nil {
		if block := formatUserMemorySection(user); block != "" {
			sections = append(sections, block)
		}
	}
	if session != nil {
		if block := formatSessionMemorySection(session); block != "" {
			sections = append(sections, block)
		}
	}
	if len(sections) == 0 {
		return ""
	}
	return strings.Join(sections, "\n\n")
}

func formatUserMemorySection(user *UserAgentMemory) string {
	var parts []string
	parts = append(parts, "### ??????????")
	if root := strings.TrimSpace(user.WorkspaceRoot); root != "" {
		parts = append(parts, "- ???: "+root)
	}
	parts = append(parts, formatToolHints(user.ToolHints)...)
	parts = append(parts, formatSkillHints(user.SkillHints)...)
	parts = append(parts, formatCommandPolicies(user.CommandPolicies)...)
	parts = append(parts, formatPromptNotes(user.PromptNotes)...)
	return joinNonEmptyLines(parts)
}

func formatSessionMemorySection(session *SessionAgentMemory) string {
	var parts []string
	parts = append(parts, "### ???????????")
	if agent := strings.TrimSpace(session.Agent); agent != "" {
		parts = append(parts, "- ?? agent: "+agent)
	}
	if len(session.ToolsUsed) > 0 {
		parts = append(parts, "- ????????: "+strings.Join(session.ToolsUsed, ", "))
	}
	parts = append(parts, formatToolHints(session.ToolHints)...)
	parts = append(parts, formatCommandPolicies(session.CommandPolicies)...)
	parts = append(parts, formatPromptNotes(session.PromptNotes)...)
	return joinNonEmptyLines(parts)
}

func formatToolHints(hints []ToolHint) []string {
	if len(hints) == 0 {
		return nil
	}
	lines := []string{"- ??????:"}
	for _, hint := range hints {
		name := strings.TrimSpace(hint.Name)
		when := strings.TrimSpace(hint.WhenToUse)
		if name == "" && when == "" {
			continue
		}
		line := "  - " + name
		if when != "" {
			line += ": " + when
		}
		if c := strings.TrimSpace(hint.Constraints); c != "" {
			line += "???: " + c + "?"
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		return nil
	}
	return lines
}

func formatSkillHints(hints []SkillHint) []string {
	if len(hints) == 0 {
		return nil
	}
	lines := []string{"- ?? Skills????????????:"}
	for _, hint := range hints {
		name := strings.TrimSpace(hint.Name)
		if name == "" {
			continue
		}
		line := "  - " + name
		if p := strings.TrimSpace(hint.Path); p != "" {
			line += " @ " + p
		}
		if w := strings.TrimSpace(hint.WhenToUse); w != "" {
			line += ": " + w
		}
		if d := strings.TrimSpace(hint.Description); d != "" {
			line += " - " + d
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		return nil
	}
	return lines
}

func formatCommandPolicies(policies []CommandPolicy) []string {
	if len(policies) == 0 {
		return nil
	}
	lines := []string{"- ??/????:"}
	for _, policy := range policies {
		situation := strings.TrimSpace(policy.Situation)
		if situation == "" {
			continue
		}
		line := "  - ??: " + situation
		if len(policy.Commands) > 0 {
			line += " -> " + strings.Join(policy.Commands, "?")
		}
		if n := strings.TrimSpace(policy.Notes); n != "" {
			line += "?" + n + "?"
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		return nil
	}
	return lines
}

func formatPromptNotes(notes []PromptAdjustment) []string {
	if len(notes) == 0 {
		return nil
	}
	var lines []string
	for _, note := range notes {
		title := strings.TrimSpace(note.Title)
		content := strings.TrimSpace(note.Content)
		if title == "" && content == "" {
			continue
		}
		if title == "" {
			lines = append(lines, "- "+content)
			continue
		}
		lines = append(lines, "- "+title+": "+content)
	}
	return lines
}

func joinNonEmptyLines(lines []string) string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}
