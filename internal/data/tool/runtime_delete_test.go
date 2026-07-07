package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentctx "kratos-demo/internal/data/agent/ctx"
)

func TestDeleteFileRequiresApprovalWithoutApprover(t *testing.T) {
	tools, ctx, dir := setupLocalToolsTest(t)
	path := filepath.Join(dir, "remove-me.txt")
	if err := os.WriteFile(path, []byte("bye"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	input, _ := json.Marshal(map[string]any{"path": "remove-me.txt"})
	_, err := tools.deleteFile(ctx, string(input))
	if err == nil {
		t.Fatal("expected approval error without approver")
	}
	if !strings.Contains(err.Error(), "人工确认") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("file should still exist: %v", statErr)
	}
}

func TestDeleteFileWithApproval(t *testing.T) {
	tools, baseCtx, dir := setupLocalToolsTest(t)
	path := filepath.Join(dir, "gone.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	ctx := agentctx.WithRiskApprover(baseCtx, func(_ context.Context, _ agentctx.RiskAction) (bool, error) {
		return true, nil
	})

	input, _ := json.Marshal(map[string]any{"path": "gone.txt"})
	out, err := tools.deleteFile(ctx, string(input))
	if err != nil {
		t.Fatalf("deleteFile() error = %v", err)
	}
	if !strings.Contains(out, "deleted file") {
		t.Fatalf("deleteFile() output = %q", out)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("file should be deleted, stat err = %v", statErr)
	}
}

func TestExecCommandRiskRequiresApproval(t *testing.T) {
	tools, ctx, _ := setupLocalToolsTest(t)
	_, err := tools.execCommand(ctx, `{"command":"rm popup_flower.py"}`)
	if err == nil {
		t.Fatal("expected approval error")
	}
	if !strings.Contains(err.Error(), "人工确认") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClassifyExecCommandRisk(t *testing.T) {
	risky, reason := classifyExecCommandRisk("rm popup_flower.py")
	if !risky || reason == "" {
		t.Fatalf("expected risky rm command")
	}
	risky, _ = classifyExecCommandRisk("go test ./...")
	if risky {
		t.Fatal("go test should not be risky")
	}
}
