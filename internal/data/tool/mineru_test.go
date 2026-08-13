package tool

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kratos-demo/internal/data/mineru"
)

func TestMinerUParseDocumentUploadsPollsAndExtractsMarkdown(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	file, err := writer.Create("Sublinear.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("# Sublinear\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	outputDir := filepath.Join(root, ".myagent", "mineru-test", "task-1")
	markdown, err := mineru.ExtractArchive(archive.Bytes(), outputDir)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	content, err := os.ReadFile(markdown)
	if err != nil {
		t.Fatalf("read extracted markdown: %v", err)
	}
	if string(content) != "# Sublinear\n" {
		t.Fatalf("markdown = %q", content)
	}
}

func TestMinerURejectsArchivePathTraversal(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	file, err := writer.Create("../escape.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("nope"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := mineru.ExtractArchive(archive.Bytes(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("ExtractArchive() error = %v, want unsafe path error", err)
	}
}

func TestMinerUSublinearPDFIntegration(t *testing.T) {
	if os.Getenv("KRATOS_RUN_MINERU_INTEGRATION") != "1" {
		t.Skip("set KRATOS_RUN_MINERU_INTEGRATION=1 after configuring MINERU_API_TOKEN to run against MinerU")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sublinear.pdf")); err != nil {
		t.Fatalf("Sublinear.pdf test resource unavailable: %v", err)
	}
	config, err := mineru.ConfigFromEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.MaxWait+config.Timeout)
	defer cancel()
	outputDir := filepath.Join(config.OutputDir, "integration")
	result, err := mineru.NewClient(config).ParsePDF(ctx, filepath.Join(root, "Sublinear.pdf"), outputDir, mineru.ParseOptions{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(result.MarkdownPath); err != nil || info.Size() == 0 {
		t.Fatalf("MinerU markdown output unavailable: %s (%v)", result.MarkdownPath, err)
	}
}

func TestMinerUConfigFromEnv(t *testing.T) {
	t.Setenv("MINERU_API_TOKEN", "token")
	t.Setenv("MINERU_TIMEOUT_SECONDS", "bad")
	cfg, err := mineru.ConfigFromEnv(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Timeout != 60*time.Second {
		t.Fatalf("timeout = %s", cfg.Timeout)
	}
}
