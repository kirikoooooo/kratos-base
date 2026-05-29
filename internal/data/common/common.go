package common

import (
	"strings"
)

const (
	DefaultMemoryDir    = ".myagent"
	DefaultMemoryUserID = "default"
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
