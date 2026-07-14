package agent

import (
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

func TestFormatToolErrorObservationHintsAbsolutePath(t *testing.T) {
	obs := formatToolErrorObservation(llms.ToolCall{
		FunctionCall: &llms.FunctionCall{Name: "write_file"},
	}, "", errAbsolutePath(), 1)
	if !strings.Contains(obs, "hint:") || !strings.Contains(obs, "相对路径") {
		t.Fatalf("missing path hint: %q", obs)
	}
}

func TestFormatToolErrorObservationHintsJSONEscape(t *testing.T) {
	obs := formatToolErrorObservation(llms.ToolCall{
		FunctionCall: &llms.FunctionCall{Name: "write_file"},
	}, "", errJSONEscape(), 2)
	if !strings.Contains(obs, "正斜杠") {
		t.Fatalf("missing json escape hint: %q", obs)
	}
}

func TestBuildFallbackFailureAnalysis(t *testing.T) {
	text := buildFallbackFailureAnalysis(errAbsolutePath(), "tool_correction_exhausted")
	if !strings.Contains(text, "任务未能完成") || !strings.Contains(text, "相对路径") {
		t.Fatalf("unexpected fallback: %q", text)
	}
}

type absolutePathError struct{}

func (absolutePathError) Error() string {
	return "tool write_file failed: absolute paths are not allowed"
}

func errAbsolutePath() error { return absolutePathError{} }

type jsonEscapeError struct{}

func (jsonEscapeError) Error() string {
	return "tool write_file failed: parse write_file input failed: invalid character 'c' in string escape code"
}

func errJSONEscape() error { return jsonEscapeError{} }
