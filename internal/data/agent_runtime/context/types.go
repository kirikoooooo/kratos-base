package context

import (
	bizconversation "kratos-demo/internal/biz/conversation"
)

// Re-export domain types from biz/conversation for backward compatibility.
type (
	ConversationRole     = bizconversation.ConversationRole
	ConversationToolCall = bizconversation.ConversationToolCall
	ConversationTurn     = bizconversation.ConversationTurn
	CompressConfig       = bizconversation.CompressConfig
	CompressStats        = bizconversation.CompressStats
	PrepareMeta          = bizconversation.PrepareMeta
	CompressResult       = bizconversation.CompressResult
)

const (
	ConversationRoleHuman = bizconversation.ConversationRoleHuman
	ConversationRoleAI    = bizconversation.ConversationRoleAI
	ConversationRoleTool  = bizconversation.ConversationRoleTool

	DefaultCompressThreshold  = bizconversation.DefaultCompressThreshold
	DefaultKeepRecentTurns    = bizconversation.DefaultKeepRecentTurns
	DefaultToolOutputMaxChars = bizconversation.DefaultToolOutputMaxChars
)

var ConfigFromMemory = bizconversation.ConfigFromMemory
