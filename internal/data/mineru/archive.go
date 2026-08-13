package mineru

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractArchive 解压 MinerU 结果 ZIP，返回首个 Markdown 文件路径。
func ExtractArchive(archive []byte, outputDir string) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return "", fmt.Errorf("MinerU result is not a ZIP archive: %w", err)
	}
	var markdown string
	for _, file := range reader.File {
		name := filepath.Clean(file.Name)
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe file path in MinerU archive: %q", file.Name)
		}
		destination := filepath.Join(outputDir, name)
		if !strings.HasPrefix(destination, outputDir+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe file path in MinerU archive: %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return "", err
		}
		input, err := file.Open()
		if err != nil {
			return "", err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err == nil {
			_, err = io.Copy(output, io.LimitReader(input, 64<<20))
			closeErr := output.Close()
			if err == nil {
				err = closeErr
			}
		}
		_ = input.Close()
		if err != nil {
			return "", err
		}
		if strings.HasSuffix(strings.ToLower(destination), ".md") && markdown == "" {
			markdown = destination
		}
	}
	if markdown == "" {
		return "", fmt.Errorf("MinerU result ZIP does not contain a Markdown file")
	}
	return markdown, nil
}

// SafeTaskArtifactDir 清理 task ID 以用于目录名。
func SafeTaskArtifactDir(taskID string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	return replacer.Replace(strings.TrimSpace(taskID))
}
