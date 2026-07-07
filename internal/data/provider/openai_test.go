package provider

import (
	"testing"
)

func TestNormalizeForFunctionCalling(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "blank", input: "", want: openaiDefaultModel},
		{name: "gpt5 fallback", input: "gpt-5.4-mini", want: openaiDefaultModel},
		{name: "compatible model kept", input: "gpt-4o-mini", want: "gpt-4o-mini"},
		{name: "deepseek model kept", input: "deepseek-chat", want: "deepseek-chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeForFunctionCalling(tt.input); got != tt.want {
				t.Fatalf("normalizeForFunctionCalling(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
