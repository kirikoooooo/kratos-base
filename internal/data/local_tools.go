package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kratos-demo/internal/biz"
	toolcatalog "kratos-demo/third_party/tools"
)

type localToolRuntime struct {
	root  string
	trace biz.DelegationTraceStore
}

func newLocalToolRuntime(trace biz.DelegationTraceStore) (*localToolRuntime, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get workspace root failed: %w", err)
	}
	return &localToolRuntime{
		root:  root,
		trace: trace,
	}, nil
}

func (r *localToolRuntime) bindings() []toolcatalog.BindingSpec {
	return []toolcatalog.BindingSpec{
		{Name: "read_file", Handler: r.readFile},
		{Name: "write_file", Handler: r.writeFile},
		{Name: "exec_command", Handler: r.execCommand},
	}
}

func (r *localToolRuntime) readFile(ctx context.Context, input string) (string, error) {
	spec, err := parseReadFileInput(input)
	if err != nil {
		return "", err
	}

	absPath, err := r.resolvePath(spec.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("read file %s failed: %w", spec.Path, err)
	}
	if info.IsDir() {
		output, err := r.readDirectory(spec.Path, absPath, spec.Start, spec.End)
		if err != nil {
			return "", err
		}
		r.appendToolEvent(ctx, "tool_read_file", "read_file", spec.Path, output, "", 0)
		return output, nil
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read file %s failed: %w", spec.Path, err)
	}

	content := string(raw)
	lines := strings.Split(content, "\n")
	start, end := normalizeLineRange(spec.Start, spec.End, len(lines))
	selected := lines[start-1 : end]

	var builder strings.Builder
	builder.WriteString("path: ")
	builder.WriteString(spec.Path)
	builder.WriteString("\n")
	builder.WriteString("line_range: ")
	builder.WriteString(strconv.Itoa(start))
	builder.WriteString("-")
	builder.WriteString(strconv.Itoa(end))
	builder.WriteString("\n")
	for i, line := range selected {
		builder.WriteString(strconv.Itoa(start + i))
		builder.WriteString(": ")
		builder.WriteString(line)
		if i < len(selected)-1 {
			builder.WriteString("\n")
		}
	}

	output := builder.String()
	r.appendToolEvent(ctx, "tool_read_file", "read_file", spec.Path, output, "", 0)
	return output, nil
}

func (r *localToolRuntime) readDirectory(path, absPath string, start, end int) (string, error) {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return "", fmt.Errorf("read directory %s failed: %w", path, err)
	}

	var builder strings.Builder
	builder.WriteString("path: ")
	builder.WriteString(path)
	builder.WriteString("\n")
	builder.WriteString("type: directory\n")
	builder.WriteString("entry_count: ")
	builder.WriteString(strconv.Itoa(len(entries)))

	if len(entries) == 0 {
		builder.WriteString("\nentries: (empty)")
		return builder.String(), nil
	}

	rangeStart, rangeEnd := normalizeLineRange(start, end, len(entries))
	builder.WriteString("\nentry_range: ")
	builder.WriteString(strconv.Itoa(rangeStart))
	builder.WriteString("-")
	builder.WriteString(strconv.Itoa(rangeEnd))

	for i := rangeStart - 1; i < rangeEnd; i++ {
		builder.WriteString("\n")
		builder.WriteString(strconv.Itoa(i + 1))
		builder.WriteString(": ")
		builder.WriteString(entries[i].Name())
		if entries[i].IsDir() {
			builder.WriteString("/")
		}
	}

	return builder.String(), nil
}

func (r *localToolRuntime) writeFile(ctx context.Context, input string) (string, error) {
	spec, err := parseWriteFileInput(input)
	if err != nil {
		return "", err
	}

	absPath, err := r.resolvePath(spec.Path)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", fmt.Errorf("create parent directory for %s failed: %w", spec.Path, err)
	}
	if err := os.WriteFile(absPath, []byte(spec.Content), 0o644); err != nil {
		return "", fmt.Errorf("write file %s failed: %w", spec.Path, err)
	}

	output := fmt.Sprintf("path: %s\nbytes_written: %d", spec.Path, len(spec.Content))
	r.appendToolEvent(ctx, "tool_write_file", "write_file", spec.Path, output, "", 0)
	return output, nil
}

func (r *localToolRuntime) execCommand(ctx context.Context, input string) (string, error) {
	command, err := parseExecCommandInput(input)
	if err != nil {
		return "", err
	}
	if err := validateExecCommand(command); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "powershell", "-Command", command)
	cmd.Dir = r.root
	outputBytes, runErr := cmd.CombinedOutput()
	output := strings.TrimSpace(string(outputBytes))
	if output == "" {
		output = "(no output)"
	}

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	formatted := strings.Join([]string{
		"command: " + command,
		fmt.Sprintf("exit_code: %d", exitCode),
		"output:",
		output,
	}, "\n")

	if runErr != nil {
		r.appendToolEvent(ctx, "tool_exec_command", "exec_command", command, formatted, runErr.Error(), exitCode)
		return formatted, fmt.Errorf("exec command failed: %w", runErr)
	}

	r.appendToolEvent(ctx, "tool_exec_command", "exec_command", command, formatted, "", exitCode)
	return formatted, nil
}

func (r *localToolRuntime) resolvePath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", errors.New("path is required")
	}
	if filepath.IsAbs(path) {
		return "", errors.New("absolute paths are not allowed")
	}

	cleaned := filepath.Clean(strings.ReplaceAll(path, "/", string(filepath.Separator)))
	absPath := filepath.Join(r.root, cleaned)
	rel, err := filepath.Rel(r.root, absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path %s failed: %w", path, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes workspace root")
	}
	return absPath, nil
}

func (r *localToolRuntime) appendToolEvent(ctx context.Context, stage, toolName, input, output, errMsg string, exitCode int) {
	if r == nil || r.trace == nil {
		return
	}
	taskID := currentTaskID(ctx)
	if taskID == "" {
		return
	}
	event := biz.DelegationEvent{
		Time:          time.Now(),
		TaskID:        taskID,
		Agent:         currentTaskAgent(ctx).String(),
		Stage:         stage,
		PromptPreview: previewPrompt(input),
		Summary:       summarizeToolOutput(output),
		ToolName:      toolName,
		ToolInput:     strings.TrimSpace(input),
		ToolOutput:    strings.TrimSpace(output),
		ExitCode:      exitCode,
		DurationMS:    0,
	}
	if strings.TrimSpace(errMsg) != "" {
		event.Error = errMsg
	}
	r.trace.AppendEvent(event)
}

type readFileInput struct {
	Path  string `json:"path"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func parseReadFileInput(input string) (readFileInput, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return readFileInput{}, errors.New("read_file input is empty")
	}
	if !strings.HasPrefix(input, "{") {
		return readFileInput{Path: input}, nil
	}

	var spec readFileInput
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		return readFileInput{}, fmt.Errorf("parse read_file input failed: %w", err)
	}
	if strings.TrimSpace(spec.Path) == "" {
		return readFileInput{}, errors.New("read_file path is required")
	}
	return spec, nil
}

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func parseWriteFileInput(input string) (writeFileInput, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return writeFileInput{}, errors.New("write_file input is empty")
	}
	var spec writeFileInput
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		return writeFileInput{}, fmt.Errorf("parse write_file input failed: %w", err)
	}
	if strings.TrimSpace(spec.Path) == "" {
		return writeFileInput{}, errors.New("write_file path is required")
	}
	return spec, nil
}

type execCommandInput struct {
	Command string `json:"command"`
}

func parseExecCommandInput(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("exec_command input is empty")
	}
	if !strings.HasPrefix(input, "{") {
		return input, nil
	}

	var spec execCommandInput
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		return "", fmt.Errorf("parse exec_command input failed: %w", err)
	}
	if strings.TrimSpace(spec.Command) == "" {
		return "", errors.New("exec_command command is required")
	}
	return strings.TrimSpace(spec.Command), nil
}

func normalizeLineRange(start, end, total int) (int, int) {
	if total <= 0 {
		return 1, 1
	}
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end > total {
		end = total
	}
	if start > total {
		start = total
	}
	if end < start {
		end = start
	}
	return start, end
}

func validateExecCommand(command string) error {
	command = strings.TrimSpace(strings.ToLower(command))
	if command == "" {
		return errors.New("exec command is empty")
	}
	blocked := []string{
		"rm ", "rmdir ", "del ", "erase ", "format ", "shutdown ", "restart-computer",
		"stop-computer", "remove-item", "git reset", "git checkout --", "git clean",
	}
	for _, item := range blocked {
		if strings.Contains(command, item) {
			return fmt.Errorf("exec command is not allowed: %s", strings.TrimSpace(item))
		}
	}
	return nil
}

func summarizeToolOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return strings.Join(lines, "\n")
}
