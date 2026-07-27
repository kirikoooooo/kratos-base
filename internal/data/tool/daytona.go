package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
	daytonatypes "github.com/daytona/clients/sdk-go/pkg/types"

	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
)

type daytonaAnalysisInput struct {
	Code       string   `json:"code"`
	OutputPath string   `json:"output_path"`
	Files      []string `json:"files"`
}

func (r *ToolExecutor) daytonaDataAnalysis(ctx context.Context, input string) (string, error) {
	if r.daytona == nil || !r.daytona.GetEnabled() {
		return "", errors.New("Daytona data analysis is disabled; set runtime.daytona.enabled=true")
	}
	if err := r.authorize(ctx, "tool.daytona_data_analysis", "", ""); err != nil {
		return "", err
	}
	var spec daytonaAnalysisInput
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		return "", fmt.Errorf("parse daytona_data_analysis input: %w", err)
	}
	if strings.TrimSpace(spec.Code) == "" {
		return "", errors.New("daytona_data_analysis code is required")
	}
	if !strings.HasPrefix(spec.OutputPath, "/tmp/") || filepath.Ext(spec.OutputPath) != ".png" {
		return "", errors.New("output_path must be a /tmp/*.png sandbox path")
	}

	apiKey := strings.TrimSpace(r.daytona.GetApiKey())
	apiKeyName := strings.TrimSpace(r.daytona.GetApiKeyEnv())
	if apiKey == "" {
		if apiKeyName == "" {
			apiKeyName = "DAYTONA_API_KEY"
		}
		apiKey = strings.TrimSpace(os.Getenv(apiKeyName))
	}
	if apiKey == "" {
		return "", fmt.Errorf("Daytona API key is missing; set runtime.daytona.api_key or export %s", apiKeyName)
	}
	client, err := daytona.NewClientWithConfig(&daytonatypes.DaytonaConfig{
		APIKey:      apiKey,
		APIUrl:      strings.TrimSpace(r.daytona.GetApiUrl()),
		Target:      strings.TrimSpace(r.daytona.GetTarget()),
		OtelEnabled: false,
	})
	if err != nil {
		return "", fmt.Errorf("create Daytona client: %w", err)
	}
	defer client.Close(context.Background())

	autoDelete := int(r.daytona.GetAutoDeleteMinutes())
	sandbox, err := client.Create(ctx, daytonatypes.SnapshotParams{
		Snapshot: r.daytona.GetSnapshot(),
		SandboxBaseParams: daytonatypes.SandboxBaseParams{
			AutoDeleteInterval: &autoDelete,
			NetworkBlockAll:    true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("create Daytona sandbox: %w", err)
	}
	defer sandbox.Delete(context.Background())

	if err := sandbox.FileSystem.CreateFolder(ctx, "/tmp/input"); err != nil {
		return "", fmt.Errorf("create Daytona analysis input directory: %w", err)
	}
	for _, requested := range spec.Files {
		local, err := r.resolvePath(requested)
		if err != nil {
			return "", fmt.Errorf("resolve analysis input %q: %w", requested, err)
		}
		if err := sandbox.FileSystem.UploadFile(ctx, local, "/tmp/input/"+filepath.Base(local)); err != nil {
			return "", fmt.Errorf("upload analysis input %q: %w", requested, err)
		}
	}
	result, err := sandbox.Process.CodeRun(ctx, spec.Code)
	if err != nil {
		return "", fmt.Errorf("run analysis code in Daytona: %w", err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("Daytona analysis failed (exit %d): %s", result.ExitCode, strings.TrimSpace(result.Result))
	}
	image, err := sandbox.FileSystem.DownloadFile(ctx, spec.OutputPath, nil)
	if err != nil {
		return "", fmt.Errorf("download analysis chart %q: %w", spec.OutputPath, err)
	}
	artifactDir := filepath.Join(r.root, ".myagent", "artifacts", safeTaskArtifactDir(agentctx.TaskID(ctx)))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return "", fmt.Errorf("create chart artifact directory: %w", err)
	}
	localOutput := filepath.Join(artifactDir, filepath.Base(spec.OutputPath))
	if err := os.WriteFile(localOutput, image, 0o644); err != nil {
		return "", fmt.Errorf("write chart artifact: %w", err)
	}
	output := fmt.Sprintf("sandbox_id: %s\nexit_code: %d\nchart: %s\nresult:\n%s", sandbox.ID, result.ExitCode, filepath.ToSlash(localOutput), strings.TrimSpace(result.Result))
	r.appendToolEvent(ctx, "tool_daytona_data_analysis", "daytona_data_analysis", input, output, "", result.ExitCode)
	return output, nil
}

func safeTaskArtifactDir(taskID string) string {
	if taskID == "" {
		return "adhoc"
	}
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, taskID)
}
