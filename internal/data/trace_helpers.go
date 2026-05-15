package data

import "strings"

func previewPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if len(prompt) <= 96 {
		return prompt
	}
	return prompt[:96] + "..."
}
