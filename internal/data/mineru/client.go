// Package mineru 封装 MinerU v4 异步 PDF 解析 API，供 runtime 工具与 RAG 入库共用。
package mineru

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultBaseURL = "https://mineru.net"

// Config MinerU 客户端配置，Token 从环境变量 MINERU_API_TOKEN 读取。
type Config struct {
	Token        string
	BaseURL      string
	Timeout      time.Duration
	PollInterval time.Duration
	MaxWait      time.Duration
	OutputDir    string
}

// ParseOptions 单次 PDF 解析参数。
type ParseOptions struct {
	Language     string
	ModelVersion string
	IsOCR        bool
}

// ParseResult MinerU 解析产物路径。
type ParseResult struct {
	TaskID       string
	OutputDir    string
	MarkdownPath string
}

// Client MinerU HTTP 客户端。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// ConfigFromEnv 从环境变量加载配置。OutputDir 默认 <root>/.myagent/mineru。
func ConfigFromEnv(root string) (Config, error) {
	token := strings.TrimSpace(os.Getenv("MINERU_API_TOKEN"))
	if token == "" {
		return Config{}, errors.New("MinerU API token is missing; set MINERU_API_TOKEN in .env")
	}
	return Config{
		Token:        token,
		BaseURL:      defaultString(os.Getenv("MINERU_BASE_URL"), DefaultBaseURL),
		Timeout:      durationFromEnv("MINERU_TIMEOUT_SECONDS", 60),
		PollInterval: durationFromEnv("MINERU_POLL_INTERVAL_SECONDS", 5),
		MaxWait:      durationFromEnv("MINERU_MAX_WAIT_SECONDS", 900),
		OutputDir:    filepath.Join(root, defaultString(os.Getenv("MINERU_OUTPUT_DIR"), ".myagent/mineru")),
	}, nil
}

func durationFromEnv(name string, fallback int) time.Duration {
	seconds, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)) + "s")
	if err != nil || seconds <= 0 {
		return time.Duration(fallback) * time.Second
	}
	return seconds
}

// NewClient 创建 MinerU 客户端。
func NewClient(config Config) *Client {
	return &Client{
		baseURL: strings.TrimRight(config.BaseURL, "/"),
		token:   config.Token,
		http:    &http.Client{Timeout: config.Timeout},
	}
}

// ParsePDF 上传 PDF、轮询任务并解压 Markdown 到 outputDir。
func (c *Client) ParsePDF(ctx context.Context, pdfPath string, outputDir string, opts ParseOptions) (ParseResult, error) {
	dataID := fmt.Sprintf("kratos-%d", time.Now().UnixNano())
	uploadURL, err := c.createUploadURL(ctx, filepath.Base(pdfPath), dataID)
	if err != nil {
		return ParseResult{}, err
	}
	if err := c.uploadPDF(ctx, uploadURL, pdfPath); err != nil {
		return ParseResult{}, err
	}
	taskID, err := c.createTask(ctx, uploadURL, dataID, opts)
	if err != nil {
		return ParseResult{}, err
	}
	resultURL, err := c.waitForTask(ctx, taskID, durationFromEnv("MINERU_POLL_INTERVAL_SECONDS", 5), durationFromEnv("MINERU_MAX_WAIT_SECONDS", 900))
	if err != nil {
		return ParseResult{}, err
	}
	markdown, err := c.downloadResult(ctx, resultURL, outputDir)
	if err != nil {
		return ParseResult{}, err
	}
	return ParseResult{TaskID: taskID, OutputDir: outputDir, MarkdownPath: markdown}, nil
}

func (c *Client) createUploadURL(ctx context.Context, name, dataID string) (string, error) {
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			FileURLs []string `json:"file_urls"`
		} `json:"data"`
	}
	body := map[string]any{"files": []map[string]string{{"name": name, "data_id": dataID}}}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v4/file-urls/batch", body, &response); err != nil {
		return "", fmt.Errorf("request MinerU upload URL: %w", err)
	}
	if response.Code != 0 || len(response.Data.FileURLs) != 1 || strings.TrimSpace(response.Data.FileURLs[0]) == "" {
		return "", fmt.Errorf("MinerU did not return an upload URL: code=%d message=%s", response.Code, response.Msg)
	}
	return response.Data.FileURLs[0], nil
}

func (c *Client) uploadPDF(ctx context.Context, uploadURL, pdfPath string) error {
	file, err := os.Open(pdfPath)
	if err != nil {
		return fmt.Errorf("open PDF for MinerU upload: %w", err)
	}
	defer file.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, file)
	if err != nil {
		return fmt.Errorf("create MinerU upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("upload PDF to MinerU storage: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("MinerU upload returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}

func (c *Client) createTask(ctx context.Context, fileURL, dataID string, spec ParseOptions) (string, error) {
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Tasks []struct {
				TaskID string `json:"task_id"`
			} `json:"tasks"`
		} `json:"data"`
	}
	modelVersion := defaultString(spec.ModelVersion, "pipeline")
	body := map[string]any{
		"is_ocr":         spec.IsOCR,
		"enable_formula": true,
		"enable_table":   true,
		"model_version":  modelVersion,
		"files": []map[string]string{{
			"url":     fileURL,
			"data_id": dataID,
		}},
	}
	if language := strings.TrimSpace(spec.Language); language != "" {
		body["language"] = language
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v4/extract/task/batch", body, &response); err != nil {
		return "", fmt.Errorf("create MinerU extract task: %w", err)
	}
	if response.Code != 0 || len(response.Data.Tasks) != 1 || strings.TrimSpace(response.Data.Tasks[0].TaskID) == "" {
		return "", fmt.Errorf("MinerU did not return a task ID: code=%d message=%s", response.Code, response.Msg)
	}
	return response.Data.Tasks[0].TaskID, nil
}

func (c *Client) waitForTask(ctx context.Context, taskID string, interval, maxWait time.Duration) (string, error) {
	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	for {
		resultURL, state, message, err := c.getTask(ctx, taskID)
		if err != nil {
			return "", err
		}
		switch strings.ToLower(state) {
		case "done", "success", "completed":
			if resultURL == "" {
				return "", errors.New("MinerU task completed without a result ZIP URL")
			}
			return resultURL, nil
		case "failed", "error":
			return "", fmt.Errorf("MinerU task %s failed: %s", taskID, message)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-deadline.C:
			timer.Stop()
			return "", fmt.Errorf("MinerU task %s did not finish within %s", taskID, maxWait)
		case <-timer.C:
		}
	}
}

func (c *Client) getTask(ctx context.Context, taskID string) (string, string, string, error) {
	var response struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			State      string `json:"state"`
			Status     string `json:"status"`
			Message    string `json:"message"`
			FullZipURL string `json:"full_zip_url"`
			ZipURL     string `json:"zip_url"`
		} `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v4/extract/task/"+taskID, nil, &response); err != nil {
		return "", "", "", fmt.Errorf("get MinerU task %s: %w", taskID, err)
	}
	if response.Code != 0 {
		return "", "", "", fmt.Errorf("MinerU task %s returned code=%d message=%s", taskID, response.Code, response.Msg)
	}
	return firstNonEmpty(response.Data.FullZipURL, response.Data.ZipURL), firstNonEmpty(response.Data.State, response.Data.Status), firstNonEmpty(response.Data.Message, response.Msg), nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, requestBody any, target any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode MinerU response: %w", err)
	}
	return nil
}

func (c *Client) downloadResult(ctx context.Context, resultURL, outputDir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resultURL, nil)
	if err != nil {
		return "", fmt.Errorf("create MinerU result download: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("download MinerU result: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("MinerU result download returned HTTP %d", resp.StatusCode)
	}
	archive, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return "", fmt.Errorf("read MinerU result: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create MinerU output directory: %w", err)
	}
	return ExtractArchive(archive, outputDir)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
