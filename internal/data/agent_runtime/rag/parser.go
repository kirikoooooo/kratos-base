package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
	"kratos-demo/internal/data/mineru"
)

// ParseResult 文档解析产物，供 RAG 切块入库使用。
type ParseResult struct {
	SourcePath   string // 原始相对路径
	Parser       string // mineru | text | markdown
	MarkdownPath string // 规范化 Markdown 路径（PDF 经 MinerU 后）
	Content      string // 可直接切块的全文
}

// DocumentParser 按扩展名路由文档解析：PDF 走 MinerU，TXT/MD 直接读取。
// 解析职责在 runtime 层，ragdemo 只负责切块/嵌入/召回。
type DocumentParser struct {
	root string
}

// NewDocumentParser 创建解析器。root 为空时使用当前工作目录。
func NewDocumentParser(root string) (*DocumentParser, error) {
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get workspace root: %w", err)
		}
	}
	return &DocumentParser{root: root}, nil
}

// Parse 解析工作区内相对路径文档，返回 Markdown 文本。
func (p *DocumentParser) Parse(ctx context.Context, relPath string) (ParseResult, error) {
	absPath, err := p.resolvePath(relPath)
	if err != nil {
		return ParseResult{}, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return ParseResult{}, fmt.Errorf("stat %q: %w", relPath, err)
	}
	if info.IsDir() {
		return ParseResult{}, fmt.Errorf("%q is a directory", relPath)
	}

	ext := strings.ToLower(filepath.Ext(absPath))
	switch ext {
	case ".pdf":
		return p.parsePDF(ctx, relPath, absPath)
	case ".txt", ".md", ".markdown":
		return p.parseText(relPath, absPath, ext)
	default:
		return ParseResult{}, fmt.Errorf("unsupported document type %q (supported: .pdf, .txt, .md)", ext)
	}
}

func (p *DocumentParser) parsePDF(ctx context.Context, relPath, absPath string) (ParseResult, error) {
	config, err := mineru.ConfigFromEnv(p.root)
	if err != nil {
		return ParseResult{}, err
	}
	taskScope := mineru.SafeTaskArtifactDir(agentctx.TaskID(ctx))
	if taskScope == "" {
		taskScope = "ingest"
	}
	outputDir := filepath.Join(config.OutputDir, taskScope, mineru.SafeTaskArtifactDir(filepath.Base(absPath)))
	result, err := mineru.NewClient(config).ParsePDF(ctx, absPath, outputDir, mineru.ParseOptions{})
	if err != nil {
		return ParseResult{}, fmt.Errorf("mineru parse %q: %w", relPath, err)
	}
	content, err := os.ReadFile(result.MarkdownPath)
	if err != nil {
		return ParseResult{}, fmt.Errorf("read mineru markdown: %w", err)
	}
	return ParseResult{
		SourcePath:   relPath,
		Parser:       "mineru",
		MarkdownPath: result.MarkdownPath,
		Content:      string(content),
	}, nil
}

func (p *DocumentParser) parseText(relPath, absPath, ext string) (ParseResult, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return ParseResult{}, fmt.Errorf("read %q: %w", relPath, err)
	}
	parser := "text"
	if ext == ".md" || ext == ".markdown" {
		parser = "markdown"
	}
	return ParseResult{
		SourcePath:   relPath,
		Parser:       parser,
		MarkdownPath: absPath,
		Content:      string(content),
	}, nil
}

func (p *DocumentParser) resolvePath(raw string) (string, error) {
	clean := filepath.Clean(raw)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute paths are not allowed: %q", raw)
	}
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("path escapes workspace: %q", raw)
	}
	abs := filepath.Join(p.root, clean)
	if !strings.HasPrefix(abs, p.root+string(filepath.Separator)) && abs != p.root {
		return "", fmt.Errorf("path escapes workspace: %q", raw)
	}
	return abs, nil
}
