package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
	"kratos-demo/internal/data/mineru"
)

type mineruParseDocumentInput struct {
	Path         string `json:"path"`
	Language     string `json:"language"`
	ModelVersion string `json:"model_version"`
	IsOCR        bool   `json:"is_ocr"`
}

// mineruParseDocument uploads a workspace PDF, waits for the MinerU v4 task, and saves its ZIP result.
func (r *ToolExecutor) mineruParseDocument(ctx context.Context, input string) (string, error) {
	if err := r.authorize(ctx, "tool.mineru_parse_document", "", ""); err != nil {
		return "", err
	}
	var spec mineruParseDocumentInput
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		return "", fmt.Errorf("parse mineru_parse_document input: %w", err)
	}
	if strings.TrimSpace(spec.Path) == "" {
		return "", errors.New("mineru_parse_document path is required")
	}
	localPDF, err := r.resolvePath(spec.Path)
	if err != nil {
		return "", fmt.Errorf("resolve PDF path %q: %w", spec.Path, err)
	}
	if strings.ToLower(filepath.Ext(localPDF)) != ".pdf" {
		return "", errors.New("mineru_parse_document only accepts .pdf files")
	}
	info, err := os.Stat(localPDF)
	if err != nil {
		return "", fmt.Errorf("stat PDF %q: %w", spec.Path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("PDF path %q is a directory", spec.Path)
	}

	config, err := mineru.ConfigFromEnv(r.root)
	if err != nil {
		return "", err
	}
	outputDir := filepath.Join(config.OutputDir, mineru.SafeTaskArtifactDir(agentctx.TaskID(ctx)))
	result, err := mineru.NewClient(config).ParsePDF(ctx, localPDF, outputDir, mineru.ParseOptions{
		Language:     spec.Language,
		ModelVersion: spec.ModelVersion,
		IsOCR:        spec.IsOCR,
	})
	if err != nil {
		return "", err
	}
	output := fmt.Sprintf("task_id: %s\noutput_dir: %s\nmarkdown: %s", result.TaskID, filepath.ToSlash(result.OutputDir), filepath.ToSlash(result.MarkdownPath))
	r.appendToolEvent(ctx, "tool_mineru_parse_document", "mineru_parse_document", input, output, "", 0)
	return output, nil
}
