package tool

import "context"

// ToolBinding maps a tool name to its handler.
type ToolBinding struct {
	Name    string
	Handler func(ctx context.Context, input string) (string, error)
}

// ToolResult captures the outcome of a tool invocation.
type ToolResult struct {
	Output string
	Error  string
}
