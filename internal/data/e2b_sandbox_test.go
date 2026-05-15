package data

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

func TestParseSkillInvocation(t *testing.T) {
	t.Run("plain text", func(t *testing.T) {
		skill, args, err := parseSkillInvocation("lint\n--path ./internal")
		if err != nil {
			t.Fatalf("parseSkillInvocation() error = %v", err)
		}
		if skill != "lint" {
			t.Fatalf("skill = %q, want lint", skill)
		}
		if args != "--path ./internal" {
			t.Fatalf("args = %q, want --path ./internal", args)
		}
	})

	t.Run("json payload", func(t *testing.T) {
		skill, args, err := parseSkillInvocation(`{"skill":"build","args":"./cmd/kratos-demo"}`)
		if err != nil {
			t.Fatalf("parseSkillInvocation() error = %v", err)
		}
		if skill != "build" {
			t.Fatalf("skill = %q, want build", skill)
		}
		if args != "./cmd/kratos-demo" {
			t.Fatalf("args = %q, want ./cmd/kratos-demo", args)
		}
	})
}

func TestE2BSandboxExecuteCommand(t *testing.T) {
	var commandPath string
	var commandBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"sbx-test"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-test/commands":
			raw, _ := io.ReadAll(r.Body)
			commandPath = r.URL.Path
			commandBody = string(raw)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"stdout":"done","stderr":"","exitCode":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	executor := newSandboxExecutor(&conf.Runtime{
		Sandbox: &conf.Runtime_Sandbox{
			Enabled: true,
			ApiKey:  "test-key",
			BaseUrl: server.URL,
		},
	}, log.NewHelper(log.NewStdLogger(io.Discard)))

	output, err := executor.ExecuteCommand(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("ExecuteCommand() error = %v", err)
	}
	if commandPath != "/sandboxes/sbx-test/commands" {
		t.Fatalf("command path = %q", commandPath)
	}
	if !strings.Contains(commandBody, `"command":"echo hello"`) {
		t.Fatalf("command body = %q", commandBody)
	}
	if !strings.Contains(output, "sandbox_id: sbx-test") {
		t.Fatalf("output = %q", output)
	}
	if !strings.Contains(output, "stdout:\ndone") {
		t.Fatalf("output = %q", output)
	}
}
