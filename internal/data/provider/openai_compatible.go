package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	bizllm "kratos-demo/internal/biz/llm"
	bizprovider "kratos-demo/internal/biz/provider"
	"kratos-demo/internal/conf"
	datallm "kratos-demo/internal/data/llm"

	"github.com/go-kratos/kratos/v2/log"
)

type compatibleProvider struct {
	config Config
	name   bizprovider.ProviderType
}

func (p *compatibleProvider) GetProviderName() bizprovider.ProviderType {
	return p.name
}

func (p *compatibleProvider) CreateModel(_ ...ModelOption) (bizllm.ModelClient, error) {
	return datallm.NewOpenAIModelClient(datallm.ModelConfig{
		APIKey: p.config.APIKey, BaseURL: p.config.BaseURL, Model: p.config.Model, Timeout: p.config.Timeout,
	}), nil
}

// newOpenAICompatibleProvider builds every supported provider around the
// OpenAI Chat Completions protocol implemented by github.com/sashabaranov/go-openai.
func newOpenAICompatibleProvider(ai *conf.AI, name bizprovider.ProviderType, logger *log.Helper) (LLMProvider, error) {
	var apiKey, baseURL, model string
	var timeoutSeconds int64
	switch name {
	case bizprovider.ProviderTypeOpenAI:
		c := ai.GetOpenai()
		apiKey, baseURL, model, timeoutSeconds = c.GetApiKey(), c.GetBaseUrl(), c.GetModel(), c.GetTimeoutSeconds()
		if model == "" {
			model = "gpt-4o-mini"
		}
	case bizprovider.ProviderTypeDeepSeek:
		c := ai.GetDeepseek()
		apiKey, baseURL, model, timeoutSeconds = c.GetApiKey(), c.GetBaseUrl(), c.GetModel(), c.GetTimeoutSeconds()
		if baseURL == "" {
			baseURL = "https://api.deepseek.com"
		}
		if model == "" {
			model = "deepseek-v4-flash"
		}
	case bizprovider.ProviderTypeGemini:
		c := ai.GetGemini()
		apiKey, baseURL, model, timeoutSeconds = c.GetApiKey(), c.GetBaseUrl(), c.GetModel(), c.GetTimeoutSeconds()
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com/v1beta/openai/"
		}
		if model == "" {
			model = "gemini-2.5-flash"
		}
	case bizprovider.ProviderTypeGrok:
		c := ai.GetGrok()
		apiKey, baseURL, model, timeoutSeconds = c.GetApiKey(), c.GetBaseUrl(), c.GetModel(), c.GetTimeoutSeconds()
		if baseURL == "" {
			baseURL = "https://api.x.ai/v1"
		}
		if model == "" {
			model = "grok-3-mini"
		}
	case bizprovider.ProviderTypeClaude:
		c := ai.GetClaude()
		apiKey, baseURL, model, timeoutSeconds = c.GetApiKey(), c.GetBaseUrl(), c.GetModel(), c.GetTimeoutSeconds()
		if baseURL == "" {
			baseURL = "https://api.anthropic.com/v1"
		}
		if model == "" {
			model = "claude-sonnet-4-5"
		}
	case bizprovider.ProviderTypeOpenRouter:
		c := ai.GetOpenrouter()
		apiKey, baseURL, model, timeoutSeconds = c.GetApiKey(), c.GetBaseUrl(), c.GetModel(), c.GetTimeoutSeconds()
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
	default:
		return nil, fmt.Errorf("unsupported OpenAI-compatible provider: %s", name)
	}
	if strings.TrimSpace(apiKey) == "" {
		logger.Warnf("ai.%s.api_key is empty", name)
		return nil, fmt.Errorf("ai.%s.api_key is empty", name)
	}
	return &compatibleProvider{name: name, config: Config{
		APIKey: strings.TrimSpace(apiKey), BaseURL: strings.TrimSpace(baseURL), Model: strings.TrimSpace(model), Timeout: timeoutFromSeconds(timeoutSeconds),
	}}, nil
}

type fallbackProvider struct {
	providers []LLMProvider
	retries   int
	delay     time.Duration
}

func (*fallbackProvider) GetProviderName() bizprovider.ProviderType { return "fallback" }

func (p *fallbackProvider) CreateModel(opts ...ModelOption) (bizllm.ModelClient, error) {
	models := make([]namedModelClient, 0, len(p.providers))
	var failures []string
	for _, provider := range p.providers {
		model, err := provider.CreateModel(opts...)
		if err == nil {
			models = append(models, namedModelClient{provider: provider.GetProviderName(), client: model})
			continue
		}
		failures = append(failures, err.Error())
	}
	if len(models) > 0 {
		return &fallbackModelClient{models: models, retries: p.retries, delay: p.delay}, nil
	}
	return nil, fmt.Errorf("all AI providers failed to create a model: %s", strings.Join(failures, "; "))
}

type fallbackModelClient struct {
	models  []namedModelClient
	retries int
	delay   time.Duration
}

type namedModelClient struct {
	provider bizprovider.ProviderType
	client   bizllm.ModelClient
}

func (c *fallbackModelClient) GenerateContent(ctx context.Context, messages []bizllm.MessageContent, opts ...bizllm.CallOption) (*bizllm.ContentResponse, error) {
	var failures []string
	for _, model := range c.models {
		for attempt := 0; attempt <= c.retries; attempt++ {
			response, err := model.client.GenerateContent(ctx, messages, opts...)
			if err == nil {
				return response, nil
			}
			failures = append(failures, (&bizprovider.ProviderError{Provider: model.provider, Operation: "generate content", Err: err}).Error())
			if attempt < c.retries && c.delay > 0 {
				time.Sleep(c.delay)
			}
		}
	}
	return nil, fmt.Errorf("all AI providers failed: %s", strings.Join(failures, "; "))
}

func (c *fallbackModelClient) Call(ctx context.Context, prompt string, opts ...bizllm.CallOption) (string, error) {
	response, err := c.GenerateContent(ctx, []bizllm.MessageContent{bizllm.TextParts(bizllm.RoleUser, prompt)}, opts...)
	if err != nil {
		return "", err
	}
	if response == nil || len(response.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(response.Choices[0].Content), nil
}
