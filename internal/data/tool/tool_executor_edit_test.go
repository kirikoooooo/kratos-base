package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kratos-demo/internal/consts/public"
	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
	datatrace "kratos-demo/internal/data/trace"
)

func TestEditFileAppendPreservesExistingContent(t *testing.T) {
	tools, ctx, dir := setupLocalToolsTest(t)

	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Title\n\nexisting content\n"), 0o644); err != nil {
		t.Fatalf("seed readme: %v", err)
	}

	input, _ := json.Marshal(map[string]any{
		"path":       "README.md",
		"operation":  "append",
		"new_string": "i love u",
	})
	out, err := tools.editFile(ctx, string(input))
	if err != nil {
		t.Fatalf("editFile() error = %v", err)
	}
	if !strings.Contains(out, "append") {
		t.Fatalf("editFile() output = %q", out)
	}

	raw, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "existing content") {
		t.Fatalf("original content lost: %q", content)
	}
	if !strings.HasSuffix(strings.TrimSpace(content), "i love u") {
		t.Fatalf("append missing, got: %q", content)
	}
}

func TestEditFileSearchReplace(t *testing.T) {
	tools, ctx, dir := setupLocalToolsTest(t)

	target := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(target, []byte("foo bar foo"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	input, _ := json.Marshal(map[string]any{
		"path":       "a.txt",
		"operation":  "search_replace",
		"old_string": "bar",
		"new_string": "baz",
	})
	if _, err := tools.editFile(ctx, string(input)); err != nil {
		t.Fatalf("editFile() error = %v", err)
	}

	raw, _ := os.ReadFile(target)
	if string(raw) != "foo baz foo" {
		t.Fatalf("content = %q, want foo baz foo", string(raw))
	}
}

func TestEditFileInsertLine(t *testing.T) {
	tools, ctx, dir := setupLocalToolsTest(t)

	readmePath := filepath.Join(dir, "README.md")
	seed := "# Title\n\nline40\nline41placeholder\nline42\n"
	if err := os.WriteFile(readmePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed readme: %v", err)
	}

	input, _ := json.Marshal(map[string]any{
		"path":       "README.md",
		"operation":  "insert_line",
		"line":       4,
		"new_string": "i hate u",
	})
	if _, err := tools.editFile(ctx, string(input)); err != nil {
		t.Fatalf("editFile() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(mustRead(t, readmePath))), "\n")
	if len(lines) < 5 || lines[3] != "i hate u" {
		t.Fatalf("insert at line 4 failed, lines=%v", lines)
	}
	if lines[4] != "line41placeholder" {
		t.Fatalf("following line shifted wrong, lines=%v", lines)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return raw
}

func TestEditFileDeleteAndReplaceLine(t *testing.T) {
	tools, ctx, dir := setupLocalToolsTest(t)
	path := filepath.Join(dir, "doc.txt")
	_ = os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0o644)

	delInput, _ := json.Marshal(map[string]any{
		"path": "doc.txt", "operation": "delete_line", "line": 2,
	})
	if _, err := tools.editFile(ctx, string(delInput)); err != nil {
		t.Fatalf("delete_line: %v", err)
	}
	if string(mustRead(t, path)) != "a\nc\nd\n" {
		t.Fatalf("after delete_line: %q", mustRead(t, path))
	}

	repInput, _ := json.Marshal(map[string]any{
		"path": "doc.txt", "operation": "replace_line", "line": 2, "new_string": "B2",
	})
	if _, err := tools.editFile(ctx, string(repInput)); err != nil {
		t.Fatalf("replace_line: %v", err)
	}
	if string(mustRead(t, path)) != "a\nB2\nd\n" {
		t.Fatalf("after replace_line: %q", mustRead(t, path))
	}
}

func TestEditFileDeleteLinesAndDeleteString(t *testing.T) {
	tools, ctx, dir := setupLocalToolsTest(t)
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte("keep\nremove1\nremove2\nkeep2\n"), 0o644)

	rangeInput, _ := json.Marshal(map[string]any{
		"path": "x.txt", "operation": "delete_lines", "line": 2, "line_end": 3,
	})
	if _, err := tools.editFile(ctx, string(rangeInput)); err != nil {
		t.Fatalf("delete_lines: %v", err)
	}
	if string(mustRead(t, path)) != "keep\nkeep2\n" {
		t.Fatalf("after delete_lines: %q", mustRead(t, path))
	}

	strInput, _ := json.Marshal(map[string]any{
		"path": "x.txt", "operation": "delete_string", "old_string": "keep2\n",
	})
	if _, err := tools.editFile(ctx, string(strInput)); err != nil {
		t.Fatalf("delete_string: %v", err)
	}
	if string(mustRead(t, path)) != "keep\n" {
		t.Fatalf("final content=%q", mustRead(t, path))
	}
}

func TestWriteFileRejectsExistingFile(t *testing.T) {
	tools, ctx, dir := setupLocalToolsTest(t)

	target := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(target, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	input, _ := json.Marshal(map[string]any{
		"path":    "keep.txt",
		"content": "i love u",
	})
	_, err := tools.writeFile(ctx, string(input))
	if err == nil {
		t.Fatal("writeFile() error = nil, want rejection for existing file")
	}
	if !strings.Contains(err.Error(), "edit_file") {
		t.Fatalf("writeFile() error = %v, want edit_file hint", err)
	}

	raw, _ := os.ReadFile(target)
	if string(raw) != "keep me" {
		t.Fatalf("file overwritten: %q", string(raw))
	}
}

func setupLocalToolsTest(t *testing.T) (*ToolExecutor, context.Context, string) {
	t.Helper()
	trace := datatrace.NewDelegationTraceStore()
	tools, err := NewToolExecutor(trace, nil)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	dir := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	tools.root = dir
	ctx := agentctx.WithAgent(agentctx.WithTaskID(context.Background(), "edit-tool-task"), public.AgentKindDefault)
	return tools, ctx, dir
}
