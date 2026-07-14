package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	agentfile "kratos-demo/internal/data/agent_runtime/file"
	"kratos-demo/internal/data/common"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	biztool "kratos-demo/internal/biz/tool"
	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
	datasession "kratos-demo/internal/data/session"
	datatrace "kratos-demo/internal/data/trace"
	toolcatalog "kratos-demo/third_party/tools"
)

type Runtime struct {
	root     string
	trace    datatrace.DelegationTraceStore
	sessions datasession.SessionStore
}

func NewRuntime(trace datatrace.DelegationTraceStore, sessions datasession.SessionStore) (*Runtime, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get workspace root failed: %w", err)
	}
	return &Runtime{
		root:     root,
		trace:    trace,
		sessions: sessions,
	}, nil
}

func (r *Runtime) Bindings() []toolcatalog.BindingSpec {
	return []toolcatalog.BindingSpec{
		{Name: "read_file", Handler: r.readFile},
		{Name: "edit_file", Handler: r.editFile},
		{Name: "write_file", Handler: r.writeFile},
		{Name: "delete_file", Handler: r.deleteFile},
		{Name: "exec_command", Handler: r.execCommand},
		{Name: "load_skill", Handler: r.loadSkill},
	}
}

// loadSkill exposes the full, local skill instructions only when an agent asks
// for a discovered skill. The explicit root check keeps skill loading scoped.
func (r *Runtime) loadSkill(ctx context.Context, input string) (string, error) {
	name := strings.TrimSpace(input)
	if strings.HasPrefix(name, "{") {
		var payload struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(name), &payload); err == nil {
			name = strings.TrimSpace(payload.Name)
		}
	}
	if name == "" || strings.Contains(name, "..") || filepath.IsAbs(name) {
		return "", errors.New("skill name must be a relative skill directory name")
	}
	var raw []byte
	var path string
	for _, root := range []string{filepath.Join(r.root, ".agents", "skills"), filepath.Join(r.root, "skills")} {
		candidate := filepath.Join(root, filepath.FromSlash(name), "SKILL.md")
		rel, err := filepath.Rel(root, candidate)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", errors.New("skill path escapes configured skill roots")
		}
		raw, err = os.ReadFile(candidate)
		if err == nil {
			path = candidate
			break
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("load skill %s: %w", name, err)
		}
	}
	if path == "" {
		return "", fmt.Errorf("load skill %s: not found", name)
	}
	rel, _ := filepath.Rel(r.root, path)
	output := "path: " + filepath.ToSlash(rel) + "\n\n" + string(raw)
	r.appendToolEvent(ctx, "tool_load_skill", "load_skill", name, output, "", 0)
	return output, nil
}

func (r *Runtime) readFile(ctx context.Context, input string) (string, error) {
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

func (r *Runtime) readDirectory(path, absPath string, start, end int) (string, error) {
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

func (r *Runtime) editFile(ctx context.Context, input string) (string, error) {
	spec, err := parseEditFileInput(input)
	if err != nil {
		return "", err
	}

	absPath, err := r.resolvePath(spec.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("edit_file %s failed: file does not exist, use write_file to create new files", spec.Path)
		}
		return "", fmt.Errorf("edit_file %s failed: %w", spec.Path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("edit_file %s failed: path is a directory", spec.Path)
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read file %s before edit failed: %w", spec.Path, err)
	}
	original := string(raw)

	updated, detail, err := applyFileEdit(original, spec)
	if err != nil {
		return "", fmt.Errorf("edit_file %s failed: %w", spec.Path, err)
	}
	if updated == original {
		return "", fmt.Errorf("edit_file %s failed: no changes applied", spec.Path)
	}

	if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write file %s after edit failed: %w", spec.Path, err)
	}

	output := fmt.Sprintf("path: %s\noperation: %s\n%s\nbytes_before: %d\nbytes_after: %d",
		spec.Path, spec.Operation, detail, len(original), len(updated))
	r.recordFileChange(ctx, spec.Path, "edit_file", spec.Operation, detail, original, updated)
	r.appendToolEvent(ctx, "tool_edit_file", "edit_file", spec.Path, output, "", 0)
	return output, nil
}

func applyFileEdit(original string, spec editFileInput) (string, string, error) {
	switch spec.Operation {
	case "prepend":
		if strings.TrimSpace(spec.NewString) == "" {
			return "", "", errors.New("prepend requires new_string")
		}
		updated, detail, err := agentfile.ApplyPrepend(original, spec.NewString)
		if err != nil {
			return "", "", err
		}
		return updated, detail, nil
	case "insert_line":
		if spec.Line <= 0 {
			return "", "", errors.New("insert_line requires line >= 1")
		}
		if strings.TrimSpace(spec.NewString) == "" {
			return "", "", errors.New("insert_line requires new_string")
		}
		updated, err := agentfile.ApplyInsertLine(original, spec.Line, spec.NewString)
		if err != nil {
			return "", "", err
		}
		return updated, fmt.Sprintf("inserted content at line %d", spec.Line), nil
	case "insert_after_line":
		if spec.Line <= 0 {
			return "", "", errors.New("insert_after_line requires line >= 1")
		}
		if strings.TrimSpace(spec.NewString) == "" {
			return "", "", errors.New("insert_after_line requires new_string")
		}
		updated, err := agentfile.ApplyInsertAfterLine(original, spec.Line, spec.NewString)
		if err != nil {
			return "", "", err
		}
		return updated, fmt.Sprintf("inserted content after line %d", spec.Line), nil
	case "delete_line":
		if spec.Line <= 0 {
			return "", "", errors.New("delete_line requires line >= 1")
		}
		updated, err := agentfile.ApplyDeleteLine(original, spec.Line)
		if err != nil {
			return "", "", err
		}
		return updated, fmt.Sprintf("deleted line %d", spec.Line), nil
	case "delete_lines":
		start, end := spec.lineSpan()
		updated, err := agentfile.ApplyDeleteLines(original, start, end)
		if err != nil {
			return "", "", err
		}
		return updated, fmt.Sprintf("deleted lines %d-%d", start, end), nil
	case "replace_line":
		if spec.Line <= 0 {
			return "", "", errors.New("replace_line requires line >= 1")
		}
		updated, err := agentfile.ApplyReplaceLine(original, spec.Line, spec.NewString)
		if err != nil {
			return "", "", err
		}
		return updated, fmt.Sprintf("replaced line %d", spec.Line), nil
	case "replace_lines":
		start, end := spec.lineSpan()
		updated, err := agentfile.ApplyReplaceLines(original, start, end, spec.NewString)
		if err != nil {
			return "", "", err
		}
		return updated, fmt.Sprintf("replaced lines %d-%d", start, end), nil
	case "delete_string":
		if strings.TrimSpace(spec.OldString) == "" {
			return "", "", errors.New("delete_string requires old_string")
		}
		updated, count, err := agentfile.ApplyDeleteString(original, spec.OldString, spec.ReplaceAll)
		if err != nil {
			return "", "", err
		}
		return updated, fmt.Sprintf("deleted %d occurrence(s)", count), nil
	case "append":
		if strings.TrimSpace(spec.NewString) == "" {
			return "", "", errors.New("append requires new_string")
		}
		return agentfile.ApplyAppendContent(original, spec.NewString), "appended new content to end of file", nil
	case "search_replace", "":
		if strings.TrimSpace(spec.OldString) == "" {
			return "", "", errors.New("search_replace requires old_string; use append/prepend/insert_line for additions")
		}
		updated, count, err := agentfile.ApplySearchReplace(original, spec.OldString, spec.NewString, spec.ReplaceAll)
		if err != nil {
			return "", "", err
		}
		return updated, fmt.Sprintf("replaced %d occurrence(s)", count), nil
	default:
		return "", "", fmt.Errorf("unsupported operation %q", spec.Operation)
	}
}

func (r *Runtime) writeFile(ctx context.Context, input string) (string, error) {
	spec, err := parseWriteFileInput(input)
	if err != nil {
		return "", err
	}

	absPath, err := r.resolvePath(spec.Path)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(absPath); err == nil {
		return "", fmt.Errorf("file %s already exists; use edit_file to modify (search_replace/append), write_file only creates new files", spec.Path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat file %s failed: %w", spec.Path, err)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", fmt.Errorf("create parent directory for %s failed: %w", spec.Path, err)
	}
	if err := os.WriteFile(absPath, []byte(spec.Content), 0o644); err != nil {
		return "", fmt.Errorf("write file %s failed: %w", spec.Path, err)
	}

	output := fmt.Sprintf("path: %s\nbytes_written: %d", spec.Path, len(spec.Content))
	r.recordFileChange(ctx, spec.Path, "write_file", "create", "created new file", "", spec.Content)
	r.appendToolEvent(ctx, "tool_write_file", "write_file", spec.Path, output, "", 0)
	return output, nil
}

func (r *Runtime) deleteFile(ctx context.Context, input string) (string, error) {
	spec, err := parseDeleteFileInput(input)
	if err != nil {
		return "", err
	}

	absPath, err := r.resolvePath(spec.Path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("delete_file %s failed: file does not exist", spec.Path)
		}
		return "", fmt.Errorf("delete_file %s failed: %w", spec.Path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("delete_file %s failed: path is a directory; delete_file only removes files", spec.Path)
	}

	before, _ := os.ReadFile(absPath)
	if err := requireRiskApproval(ctx, agentctx.RiskAction{
		Tool:    "delete_file",
		Summary: "删除文件 " + spec.Path,
		Detail:  fmt.Sprintf("将永久删除工作区文件 %s（%d 字节）", spec.Path, info.Size()),
	}); err != nil {
		return "", err
	}

	if err := os.Remove(absPath); err != nil {
		return "", fmt.Errorf("delete_file %s failed: %w", spec.Path, err)
	}

	output := fmt.Sprintf("path: %s\noperation: delete_file\ndeleted file (%d bytes)", spec.Path, info.Size())
	r.recordFileChange(ctx, spec.Path, "delete_file", "delete", "deleted file", string(before), "")
	r.appendToolEvent(ctx, "tool_delete_file", "delete_file", spec.Path, output, "", 0)
	return output, nil
}

func (r *Runtime) execCommand(ctx context.Context, input string) (string, error) {
	command, err := parseExecCommandInput(input)
	if err != nil {
		return "", err
	}
	if risky, reason := classifyExecCommandRisk(command); risky {
		if err := requireRiskApproval(ctx, agentctx.RiskAction{
			Tool:    "exec_command",
			Summary: "执行高风险命令",
			Detail:  fmt.Sprintf("%s\n命令: %s", reason, command),
		}); err != nil {
			return "", err
		}
	}

	cmd := shellCommand(ctx, command)
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

func (r *Runtime) resolvePath(raw string) (string, error) {
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

func (r *Runtime) recordFileChange(ctx context.Context, path, toolName, operation, summary, before, after string) {
	if r == nil || r.sessions == nil {
		return
	}
	taskID := agentctx.TaskID(ctx)
	if taskID == "" {
		return
	}
	if sess := r.sessions.Open(taskID); sess != nil {
		sess.RecordFileChange(path, toolName, operation, summary, before, after)
	}
}

func (r *Runtime) appendToolEvent(ctx context.Context, stage, toolName, input, output, errMsg string, exitCode int) {
	if r == nil || r.trace == nil {
		return
	}
	taskID := agentctx.TaskID(ctx)
	if taskID == "" {
		return
	}
	event := datatrace.DelegationEvent{
		Time:          time.Now(),
		TaskID:        taskID,
		Agent:         string(agentctx.Agent(ctx)),
		Stage:         stage,
		PromptPreview: common.PreviewPrompt(input),
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

type editFileInput struct {
	Path       string `json:"path"`
	Operation  string `json:"operation"`
	Line       int    `json:"line"`
	LineEnd    int    `json:"line_end"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
	Content    string `json:"content"`
}

func (s editFileInput) lineSpan() (int, int) {
	start := s.Line
	end := s.LineEnd
	if start <= 0 {
		start = 1
	}
	if end <= 0 {
		end = start
	}
	return start, end
}

func parseEditFileInput(input string) (editFileInput, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return editFileInput{}, errors.New("edit_file input is empty")
	}
	var spec editFileInput
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		return editFileInput{}, fmt.Errorf("parse edit_file input failed: %w", err)
	}
	if strings.TrimSpace(spec.Path) == "" {
		return editFileInput{}, errors.New("edit_file path is required")
	}
	spec.Operation = strings.TrimSpace(strings.ToLower(spec.Operation))
	if spec.Operation == "" {
		switch {
		case strings.TrimSpace(spec.OldString) != "" && strings.TrimSpace(spec.NewString) == "":
			spec.Operation = "delete_string"
		case strings.TrimSpace(spec.OldString) != "":
			spec.Operation = "search_replace"
		case spec.Line > 0 && strings.TrimSpace(spec.NewString) != "":
			spec.Operation = "insert_line"
		case strings.TrimSpace(spec.NewString) != "":
			spec.Operation = "append"
		default:
			spec.Operation = "search_replace"
		}
	}
	if strings.TrimSpace(spec.NewString) == "" && strings.TrimSpace(spec.Content) != "" {
		spec.NewString = spec.Content
	}
	return spec, nil
}

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type deleteFileInput struct {
	Path string `json:"path"`
}

func parseDeleteFileInput(input string) (deleteFileInput, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return deleteFileInput{}, errors.New("delete_file input is empty")
	}
	if !strings.HasPrefix(input, "{") {
		return deleteFileInput{Path: input}, nil
	}
	var spec deleteFileInput
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		return deleteFileInput{}, fmt.Errorf("parse delete_file input failed: %w", err)
	}
	if strings.TrimSpace(spec.Path) == "" {
		return deleteFileInput{}, errors.New("delete_file path is required")
	}
	return spec, nil
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

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-Command", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
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

// toolRepositoryAdapter adapts [Runtime] to satisfy [biztool.ToolRepository].
type toolRepositoryAdapter struct {
	runtime *Runtime
}

func (a *toolRepositoryAdapter) Bindings() []biztool.BindingSpec {
	specs := a.runtime.Bindings()
	result := make([]biztool.BindingSpec, len(specs))
	for i, s := range specs {
		result[i] = biztool.BindingSpec{
			Name:    s.Name,
			Handler: biztool.Handler(s.Handler),
		}
	}
	return result
}

// AsToolRepository returns a [biztool.ToolRepository] view of this Runtime.
func (r *Runtime) AsToolRepository() biztool.ToolRepository {
	return &toolRepositoryAdapter{runtime: r}
}
