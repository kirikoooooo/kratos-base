package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kratos-demo/internal/conf"
	"kratos-demo/internal/data/common"
)

const (
	defaultCLIOpenAIBaseURL   = "https://api.openai-proxy.org/v1"
	defaultCLIDeepSeekBaseURL = "https://api.deepseek.com"
)

const (
	ProviderOpenAI   = "openai"
	ProviderDeepSeek = "deepseek"
)

// CLICredentials stores user-provided API settings for CLI mode.
type CLICredentials struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

// effectiveProvider returns the active provider string, defaulting to "openai".
func effectiveProvider(ai *conf.AI) string {
	p := strings.TrimSpace(strings.ToLower(ai.GetProvider()))
	if p == "" {
		return ProviderOpenAI
	}
	return p
}

func defaultBaseURLForProvider(provider string) string {
	switch provider {
	case ProviderDeepSeek:
		return defaultCLIDeepSeekBaseURL
	default:
		return defaultCLIOpenAIBaseURL
	}
}

func cliCredentialsPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(wd, common.DefaultMemoryDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create credentials dir: %w", err)
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func LoadCLICredentials() (*CLICredentials, error) {
	path, err := cliCredentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CLICredentials{}, nil
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var creds CLICredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	creds.APIKey = strings.TrimSpace(creds.APIKey)
	creds.BaseURL = strings.TrimSpace(creds.BaseURL)
	return &creds, nil
}

func SaveCLICredentials(creds *CLICredentials) error {
	if creds == nil {
		return errors.New("credentials is nil")
	}
	path, err := cliCredentialsPath()
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(&CLICredentials{
		APIKey:  strings.TrimSpace(creds.APIKey),
		BaseURL: strings.TrimSpace(creds.BaseURL),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}

// ApplyCLICredentials 根据 ai.GetProvider() 将凭证写入对应的 provider 子配置。
func ApplyCLICredentials(ai *conf.AI, creds *CLICredentials) {
	if ai == nil || creds == nil {
		return
	}
	key := strings.TrimSpace(creds.APIKey)
	baseURL := strings.TrimSpace(creds.BaseURL)

	switch effectiveProvider(ai) {
	case ProviderDeepSeek:
		if ai.Deepseek == nil {
			ai.Deepseek = &conf.AI_DeepSeek{}
		}
		if key != "" {
			ai.Deepseek.ApiKey = key
		}
		if baseURL != "" {
			ai.Deepseek.BaseUrl = baseURL
		}
	default:
		if ai.Openai == nil {
			ai.Openai = &conf.AI_OpenAI{}
		}
		if key != "" {
			ai.Openai.ApiKey = key
		}
		if baseURL != "" {
			ai.Openai.BaseUrl = baseURL
		}
	}
}

// aiConfigReady 检查当前 provider 的凭证是否已配置齐全。
func aiConfigReady(ai *conf.AI) bool {
	if ai == nil {
		return false
	}
	switch effectiveProvider(ai) {
	case ProviderDeepSeek:
		if ai.Deepseek == nil {
			return false
		}
		return strings.TrimSpace(ai.Deepseek.GetApiKey()) != "" &&
			strings.TrimSpace(ai.Deepseek.GetBaseUrl()) != ""
	default:
		if ai.Openai == nil {
			return false
		}
		return strings.TrimSpace(ai.Openai.GetApiKey()) != "" &&
			strings.TrimSpace(ai.Openai.GetBaseUrl()) != ""
	}
}

// providerLabel 返回 provider 的中文展示名。
func providerLabel(ai *conf.AI) string {
	switch effectiveProvider(ai) {
	case ProviderDeepSeek:
		return "DeepSeek"
	default:
		return "OpenAI 兼容"
	}
}

func maskCLIAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return "(未设置)"
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

func (c *CLICredentials) syncToAI(ai *conf.AI) {
	ApplyCLICredentials(ai, c)
}
