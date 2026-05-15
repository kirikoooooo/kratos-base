package data

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	defaultE2BBaseURL      = "https://api.e2b.dev"
	defaultE2BTimeout      = 60 * time.Second
	defaultSandboxTemplate = "base"
	defaultSkillCommand    = "skills run"
)

type sandboxExecutor interface {
	Enabled() bool
	ExecuteCommand(ctx context.Context, command string) (string, error)
	ExecuteSkill(ctx context.Context, skillName, input string) (string, error)
}

func newSandboxExecutor(runtimeConfig *conf.Runtime, logger *log.Helper) sandboxExecutor {
	if runtimeConfig == nil || runtimeConfig.GetSandbox() == nil || !runtimeConfig.GetSandbox().GetEnabled() {
		return disabledSandboxExecutor{reason: "e2b sandbox is not enabled"}
	}

	sandboxConfig := runtimeConfig.GetSandbox()
	if strings.TrimSpace(sandboxConfig.GetApiKey()) == "" {
		if logger != nil {
			logger.Warn("runtime.sandbox.api_key is empty, sandbox tools disabled")
		}
		return disabledSandboxExecutor{reason: "e2b sandbox api key is empty"}
	}

	baseURL := strings.TrimSpace(sandboxConfig.GetBaseUrl())
	if baseURL == "" {
		baseURL = defaultE2BBaseURL
	}

	timeout := defaultE2BTimeout
	if sandboxConfig.GetTimeout() > 0 {
		timeout = time.Duration(sandboxConfig.GetTimeout()) * time.Second
	}

	return &e2bCodeSandbox{
		config:  sandboxConfig,
		log:     logger,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

type disabledSandboxExecutor struct {
	reason string
}

func (s disabledSandboxExecutor) Enabled() bool {
	return false
}

func (s disabledSandboxExecutor) ExecuteCommand(_ context.Context, _ string) (string, error) {
	return "", errors.New(s.reason)
}

func (s disabledSandboxExecutor) ExecuteSkill(_ context.Context, _, _ string) (string, error) {
	return "", errors.New(s.reason)
}

type e2bCodeSandbox struct {
	config    *conf.Runtime_Sandbox
	log       *log.Helper
	baseURL   string
	client    *http.Client
	mu        sync.Mutex
	sandboxID string
}

func (s *e2bCodeSandbox) Enabled() bool {
	return s != nil && s.config != nil && s.client != nil && strings.TrimSpace(s.config.GetApiKey()) != ""
}

func (s *e2bCodeSandbox) ExecuteCommand(ctx context.Context, command string) (string, error) {
	if s == nil || !s.Enabled() {
		return "", errors.New("e2b sandbox is unavailable")
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("sandbox command is empty")
	}

	sandboxID, err := s.ensureSandbox(ctx)
	if err != nil {
		return "", err
	}

	body := map[string]any{"command": command}
	responseBody, err := s.doJSON(ctx, http.MethodPost, s.baseURL+"/sandboxes/"+sandboxID+"/commands", body)
	if err != nil {
		return "", err
	}

	stdout := lookupString(responseBody, []string{"stdout"}, []string{"data", "stdout"}, []string{"result", "stdout"})
	stderr := lookupString(responseBody, []string{"stderr"}, []string{"data", "stderr"}, []string{"result", "stderr"})
	exitCode := lookupInt(responseBody, []string{"exitCode"}, []string{"exit_code"}, []string{"data", "exitCode"}, []string{"result", "exitCode"})

	output := formatSandboxExecutionResult(sandboxID, command, stdout, stderr, exitCode)
	if exitCode != 0 {
		return output, fmt.Errorf("sandbox command failed with exit code %d", exitCode)
	}
	return output, nil
}

func (s *e2bCodeSandbox) ExecuteSkill(ctx context.Context, skillName, input string) (string, error) {
	if s == nil || !s.Enabled() {
		return "", errors.New("e2b sandbox is unavailable")
	}
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return "", errors.New("sandbox skill name is empty")
	}

	skillCommand := strings.TrimSpace(s.config.GetSkillCommand())
	if skillCommand == "" {
		skillCommand = defaultSkillCommand
	}

	command := skillCommand + " " + shellQuote(skillName)
	if trimmedInput := strings.TrimSpace(input); trimmedInput != "" {
		command += " " + shellQuote(trimmedInput)
	}
	return s.ExecuteCommand(ctx, command)
}

func (s *e2bCodeSandbox) ensureSandbox(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sandboxID != "" {
		return s.sandboxID, nil
	}

	template := strings.TrimSpace(s.config.GetTemplate())
	if template == "" {
		template = defaultSandboxTemplate
	}

	responseBody, err := s.doJSON(ctx, http.MethodPost, s.baseURL+"/sandboxes", map[string]any{
		"template": template,
	})
	if err != nil {
		return "", err
	}

	s.sandboxID = lookupString(responseBody, []string{"id"}, []string{"sandboxID"}, []string{"sandboxId"}, []string{"data", "id"})
	if s.sandboxID == "" {
		return "", errors.New("e2b sandbox create response missing sandbox id")
	}
	if s.log != nil {
		s.log.Infof("e2b sandbox ready: %s", s.sandboxID)
	}
	return s.sandboxID, nil
}

func (s *e2bCodeSandbox) doJSON(ctx context.Context, method, url string, payload any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal sandbox request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create sandbox request failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(s.config.GetApiKey()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sandbox request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read sandbox response failed: %w", err)
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sandbox request status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}, nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode sandbox response failed: %w", err)
	}
	return decoded, nil
}

func parseSkillInvocation(input string) (string, string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", errors.New("sandbox skill input is empty")
	}

	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			Skill string `json:"skill"`
			Args  string `json:"args"`
			Input string `json:"input"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			args := strings.TrimSpace(payload.Args)
			if args == "" {
				args = strings.TrimSpace(payload.Input)
			}
			if skill := strings.TrimSpace(payload.Skill); skill != "" {
				return skill, args, nil
			}
		}
	}

	parts := strings.SplitN(trimmed, "\n", 2)
	skill := strings.TrimSpace(parts[0])
	if skill == "" {
		return "", "", errors.New("sandbox skill name is empty")
	}
	if len(parts) == 1 {
		return skill, "", nil
	}
	return skill, strings.TrimSpace(parts[1]), nil
}

func formatSandboxExecutionResult(sandboxID, command, stdout, stderr string, exitCode int) string {
	parts := []string{
		"sandbox_id: " + sandboxID,
		"command: " + command,
		fmt.Sprintf("exit_code: %d", exitCode),
	}
	if strings.TrimSpace(stdout) != "" {
		parts = append(parts, "stdout:\n"+strings.TrimSpace(stdout))
	}
	if strings.TrimSpace(stderr) != "" {
		parts = append(parts, "stderr:\n"+strings.TrimSpace(stderr))
	}
	return strings.Join(parts, "\n")
}

func lookupString(root map[string]any, paths ...[]string) string {
	for _, path := range paths {
		if value := lookupValue(root, path...); value != nil {
			switch v := value.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			}
		}
	}
	return ""
}

func lookupInt(root map[string]any, paths ...[]string) int {
	for _, path := range paths {
		if value := lookupValue(root, path...); value != nil {
			switch v := value.(type) {
			case float64:
				return int(v)
			case int:
				return v
			case int32:
				return int(v)
			case int64:
				return int(v)
			}
		}
	}
	return 0
}

func lookupValue(root map[string]any, path ...string) any {
	var current any = root
	for _, segment := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapping[segment]
		if current == nil {
			return nil
		}
	}
	return current
}

func shellQuote(value string) string {
	quoted := strings.ReplaceAll(value, "'", "'\"'\"'")
	return "'" + quoted + "'"
}
