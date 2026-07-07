package memory

import (
	"time"

	bizconversation "kratos-demo/internal/biz/conversation"
)

// SessionConversation stores the full conversation history for a session.
type SessionConversation struct {
	SessionID string                         `json:"session_id"`
	Agent     string                         `json:"agent,omitempty"`
	Turns     []bizconversation.ConversationTurn `json:"turns,omitempty"`
	UpdatedAt time.Time                      `json:"updated_at"`
}

// CommandPolicy describes a situational command-execution policy.
type CommandPolicy struct {
	Situation string   `json:"situation"`
	Commands  []string `json:"commands,omitempty"`
	Notes     string   `json:"notes,omitempty"`
}

// ToolHint gives the agent guidance about when to use a particular tool.
type ToolHint struct {
	Name        string `json:"name"`
	WhenToUse   string `json:"when_to_use"`
	Constraints string `json:"constraints,omitempty"`
}

// SkillHint describes a skill that the agent can invoke.
type SkillHint struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	WhenToUse   string `json:"when_to_use"`
	Description string `json:"description,omitempty"`
}

// PromptAdjustment is a persistent prompt override or addition.
type PromptAdjustment struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// UserAgentMemory stores user-level preferences, hints, and policies.
type UserAgentMemory struct {
	UserID          string             `json:"user_id"`
	WorkspaceRoot   string             `json:"workspace_root,omitempty"`
	ToolHints       []ToolHint         `json:"tool_hints,omitempty"`
	SkillHints      []SkillHint        `json:"skill_hints,omitempty"`
	CommandPolicies []CommandPolicy    `json:"command_policies,omitempty"`
	PromptNotes     []PromptAdjustment `json:"prompt_notes,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// SessionAgentMemory stores session-level preferences.
type SessionAgentMemory struct {
	SessionID       string             `json:"session_id"`
	Agent           string             `json:"agent,omitempty"`
	ToolHints       []ToolHint         `json:"tool_hints,omitempty"`
	CommandPolicies []CommandPolicy    `json:"command_policies,omitempty"`
	PromptNotes     []PromptAdjustment `json:"prompt_notes,omitempty"`
	ToolsUsed       []string           `json:"tools_used,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// SessionErrorRecord captures an error that occurred during a session.
type SessionErrorRecord struct {
	Time      time.Time `json:"time"`
	SessionID string    `json:"session_id"`
	Agent     string    `json:"agent,omitempty"`
	Stage     string    `json:"stage"`
	Tool      string    `json:"tool,omitempty"`
	Message   string    `json:"message"`
	Detail    string    `json:"detail,omitempty"`
}
