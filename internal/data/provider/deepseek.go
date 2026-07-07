package provider

import (
	"fmt"
	"strings"
	"time"

	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

const (
	deepseekDefaultModel   = "deepseek-v4-flash"
	deepseekDefaultBaseURL = "https://api.deepseek.com"
)

// DeepSeekProvider 封装 DeepSeek API 的模型创建。
// DeepSeek API 兼容 OpenAI 接口格式，且原生支持 function calling，无需模型名归一化。
type DeepSeekProvider struct {
	config  Config
	timeout time.Duration
	log     *log.Helper
}

func newDeepSeekProvider(aiConf *conf.AI, logger *log.Helper) (*DeepSeekProvider, error) {
	dc := aiConf.GetDeepseek()

	apiKey := strings.TrimSpace(dc.GetApiKey())
	if apiKey == "" {
		// 回退到 openai 的 api_key（兼容只配了 openai.api_key 的场景）
		apiKey = strings.TrimSpace(aiConf.GetOpenai().GetApiKey())
	}
	if apiKey == "" {
		logger.Warn("ai.deepseek.api_key is empty")
		return nil, fmt.Errorf("ai.deepseek.api_key is empty")
	}

	model := strings.TrimSpace(dc.GetModel())
	if model == "" {
		model = deepseekDefaultModel
	}

	baseURL := strings.TrimSpace(dc.GetBaseUrl())
	if baseURL == "" {
		baseURL = deepseekDefaultBaseURL
	}

	timeout := timeoutFromSeconds(dc.GetTimeoutSeconds())
	return &DeepSeekProvider{
		config: Config{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   model,
			Timeout: timeout,
		},
		timeout: timeout,
		log:     logger,
	}, nil
}

// CreateModel 创建 DeepSeek 兼容的 LLM 实例。
// DeepSeek 原生支持 function calling，无需模型名归一化。
func (p *DeepSeekProvider) CreateModel(opts ...ModelOption) (llms.Model, error) {
	model := p.config.Model
	if model == "" {
		model = deepseekDefaultModel
	}

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
		p.log.Warnf("create deepseek llm failed: %v", err)
		return nil, fmt.Errorf("create deepseek llm failed: %w", err)
	}
	return client, nil
}
