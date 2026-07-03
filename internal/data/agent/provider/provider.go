// Package provider 提供 LLM 厂商（OpenAI / DeepSeek）的抽象，支持统一接口创建模型实例。
package provider

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/llms"
)

// Config 封装创建 provider 所需的通用配置。
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// LLMProvider 抽象不同 AI 厂商的模型创建逻辑。
type LLMProvider interface {
	// CreateModel 创建一个 LLM 实例。
	// opts 支持 WithFunctionCalling 等语义化选项。
	CreateModel(opts ...ModelOption) (llms.Model, error)
}

type modelConfig struct {
	functionCalling bool
}

// ModelOption 模型创建选项。
type ModelOption func(*modelConfig)

// WithFunctionCalling 标记该模型用于 function calling 场景。
// 部分 provider 会据此做模型名归一化（如 OpenAI 将 gpt-5* 降级为 gpt-4o-mini）。
func WithFunctionCalling() ModelOption {
	return func(c *modelConfig) {
		c.functionCalling = true
	}
}

const (
	defaultTimeout = 120 * time.Second
)

// NewProvider 工厂函数，根据 conf.AI 创建对应的 provider。
func NewProvider(aiConf *conf.AI, logger *log.Helper) (LLMProvider, error) {
	if aiConf == nil {
		return nil, fmt.Errorf("ai config is nil")
	}

	providerName := strings.TrimSpace(strings.ToLower(aiConf.GetProvider()))
	switch providerName {
	case "", "openai":
		p, err := newOpenAIProvider(aiConf, logger)
		if err != nil {
			return nil, err
		}
		return p, nil
	case "deepseek":
		p, err := newDeepSeekProvider(aiConf, logger)
		if err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unknown ai provider: %s (supported: openai, deepseek)", providerName)
	}
}

// httpClient 创建统一配置的 HTTP 客户端，供所有 provider 复用。
func httpClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	dialTimeout := 30 * time.Second
	if timeout < dialTimeout {
		dialTimeout = timeout
	}
	tlsTimeout := 30 * time.Second
	if timeout < tlsTimeout {
		tlsTimeout = timeout
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   dialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   tlsTimeout,
			ResponseHeaderTimeout: timeout,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          16,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

// timeoutFromSeconds 从秒数计算 time.Duration，<=0 返回 defaultTimeout。
func timeoutFromSeconds(seconds int64) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultTimeout
}
