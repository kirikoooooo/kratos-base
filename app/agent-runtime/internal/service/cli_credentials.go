package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
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
		return public.ProviderOpenAI
	}
	return p
}

func defaultBaseURLForProvider(provider string) string {
	switch provider {
	case public.ProviderDeepSeek:
		return public.DefaultCLIDeepSeekBaseURL
	case public.ProviderGemini:
		return public.GeminiDefaultBaseURL
	case public.ProviderGrok:
		return public.GrokDefaultBaseURL
	case public.ProviderClaude:
		return public.ClaudeDefaultBaseURL
	case public.ProviderOpenRouter:
		return public.OpenRouterDefaultBaseURL
	default:
		return public.DefaultCLIOpenAIBaseURL
	}
}

func cliCredentialsPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(wd, public.DefaultMemoryDir)
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
	case public.ProviderDeepSeek:
		if ai.Deepseek == nil {
			ai.Deepseek = &conf.AI_DeepSeek{}
		}
		if key != "" {
			ai.Deepseek.ApiKey = key
		}
		if baseURL != "" {
			ai.Deepseek.BaseUrl = baseURL
		}
	case public.ProviderGemini:
		if ai.Gemini == nil {
			ai.Gemini = &conf.AI_Gemini{}
		}
		if key != "" {
			ai.Gemini.ApiKey = key
		}
		if baseURL != "" {
			ai.Gemini.BaseUrl = baseURL
		}
	case public.ProviderGrok:
		if ai.Grok == nil {
			ai.Grok = &conf.AI_Grok{}
		}
		if key != "" {
			ai.Grok.ApiKey = key
		}
		if baseURL != "" {
			ai.Grok.BaseUrl = baseURL
		}
	case public.ProviderClaude:
		if ai.Claude == nil {
			ai.Claude = &conf.AI_Claude{}
		}
		if key != "" {
			ai.Claude.ApiKey = key
		}
		if baseURL != "" {
			ai.Claude.BaseUrl = baseURL
		}
	case public.ProviderOpenRouter:
		if ai.Openrouter == nil {
			ai.Openrouter = &conf.AI_OpenRouter{}
		}
		if key != "" {
			ai.Openrouter.ApiKey = key
		}
		if baseURL != "" {
			ai.Openrouter.BaseUrl = baseURL
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
	case public.ProviderDeepSeek:
		if ai.Deepseek == nil {
			return false
		}
		return strings.TrimSpace(ai.Deepseek.GetApiKey()) != "" &&
			strings.TrimSpace(ai.Deepseek.GetBaseUrl()) != ""
	case public.ProviderGemini:
		return ai.Gemini != nil && strings.TrimSpace(ai.Gemini.GetApiKey()) != "" && strings.TrimSpace(ai.Gemini.GetBaseUrl()) != ""
	case public.ProviderGrok:
		return ai.Grok != nil && strings.TrimSpace(ai.Grok.GetApiKey()) != "" && strings.TrimSpace(ai.Grok.GetBaseUrl()) != ""
	case public.ProviderClaude:
		return ai.Claude != nil && strings.TrimSpace(ai.Claude.GetApiKey()) != "" && strings.TrimSpace(ai.Claude.GetBaseUrl()) != ""
	case public.ProviderOpenRouter:
		return ai.Openrouter != nil && strings.TrimSpace(ai.Openrouter.GetApiKey()) != "" && strings.TrimSpace(ai.Openrouter.GetBaseUrl()) != ""
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
	case public.ProviderDeepSeek:
		return "DeepSeek"
	case public.ProviderGemini:
		return "Gemini（OpenAI 兼容）"
	case public.ProviderGrok:
		return "Grok（OpenAI 兼容）"
	case public.ProviderClaude:
		return "Claude（OpenAI 兼容）"
	case public.ProviderOpenRouter:
		return "OpenRouter"
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
