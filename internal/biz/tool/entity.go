package tool

import "context"

// Handler is a tool execution function.
type Handler func(ctx context.Context, input string) (string, error)

// BindingSpec maps a tool name to its handler.
type BindingSpec struct {
	Name    string
	Handler Handler
}

// ToolResult captures the outcome of a tool invocation.
type ToolResult struct {
	Output string
	Error  string
}
