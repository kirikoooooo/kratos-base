package common

import (
	"strings"

	"kratos-demo/internal/consts/public"
)

// Re-export from consts/public for backward compatibility.
const (
	DefaultMemoryDir    = public.DefaultMemoryDir
	DefaultMemoryUserID = public.DefaultMemoryUserID
)

func PreviewPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if len(prompt) <= 96 {
		return prompt
	}
	return prompt[:96] + "..."
}

func SafeFileName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(value)
}
