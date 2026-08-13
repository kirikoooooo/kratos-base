package biz

import "context"

// PromptContextReader reads persistent context for an Agent turn. Mutations
// are deliberately not part of the first remote API; agent-runtime remains
// the owner of ordered conversation updates and context compression.
type PromptContextReader interface {
	PromptContext(context.Context, string, string) string
}
