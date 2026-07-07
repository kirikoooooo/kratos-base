package provider

import (
	"fmt"
	"strings"
	"time"

	bizprovider "kratos-demo/internal/biz/provider"
	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

const (
	openaiDefaultModel = "gpt-4o-mini"
)

// OpenAIProvider 封装 OpenAI 及 OpenAI 兼容 API 的模型创建。
type OpenAIProvider struct {
	config  Config
	timeout time.Duration
	log     *log.Helper
}

func newOpenAIProvider(aiConf *conf.AI, logger *log.Helper) (*OpenAIProvider, error) {
	oc := aiConf.GetOpenai()
	apiKey := strings.TrimSpace(oc.GetApiKey())
	if apiKey == "" {
		logger.Warn("ai.openai.api_key is empty")
		return nil, fmt.Errorf("ai.openai.api_key is empty")
	}
	model := strings.TrimSpace(oc.GetModel())
	if model == "" {
		model = openaiDefaultModel
	}
	timeout := timeoutFromSeconds(oc.GetTimeoutSeconds())
	return &OpenAIProvider{
		config: Config{
			APIKey:  apiKey,
			BaseURL: strings.TrimSpace(oc.GetBaseUrl()),
			Model:   model,
			Timeout: timeout,
		},
		timeout: timeout,
		log:     logger,
	}, nil
}

// CreateModel 创建 OpenAI 兼容的 LLM 实例。
// 当启用 function calling 时，gpt-5* 前缀的模型会被归一化为 gpt-4o-mini。
func (p *OpenAIProvider) CreateModel(opts ...ModelOption) (llms.Model, error) {
	mc := bizprovider.NewModelConfig()
	for _, opt := range opts {
		opt(mc)
	}
	model := p.config.Model
	if bizprovider.ModelConfigFunctionCalling(mc) {
		model = normalizeForFunctionCalling(model)
	}
	return p.create(model)
}

func (p *OpenAIProvider) create(model string) (llms.Model, error) {
	options := []openai.Option{
		openai.WithToken(p.config.APIKey),
		openai.WithModel(model),
	}
	if p.config.BaseURL != "" {
		options = append(options, openai.WithBaseURL(p.config.BaseURL))
	}
	options = append(options, openai.WithHTTPClient(httpClient(p.config.Timeout)))

	client, err := openai.New(options...)
	if err != nil {
		p.log.Warnf("create openai llm failed: %v", err)
		return nil, fmt.Errorf("create openai llm failed: %w", err)
	}
	return client, nil
}

// normalizeForFunctionCalling 将 function calling 场景下不兼容的模型名归一化。
// 当前策略：gpt-5* 前缀的模型（gpt-5.4-mini 等）在部分代理中不支持 function calling，
// 统一降级为 openaiDefaultModel。
func normalizeForFunctionCalling(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return openaiDefaultModel
	}
	if strings.HasPrefix(strings.ToLower(model), "gpt-5") {
		return openaiDefaultModel
	}
	return model
}
