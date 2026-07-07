package memory

import "strings"

// AgentMemoryConfig holds configuration for the agent memory system.
type AgentMemoryConfig struct {
	Dir                      string
	UserID                   string
	ContextCompressThreshold int
	KeepRecentTurns          int
	ToolOutputMaxChars       int
}

// NormalizedUserID returns the configured user ID, defaulting to "default".
func (c AgentMemoryConfig) NormalizedUserID() string {
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		return "default"
	}
	return userID
}
