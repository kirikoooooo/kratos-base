package provider

import (
	"time"

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

// NewModelConfig creates the internal option state for applying [ModelOption] callbacks.
// Data-layer implementations call this to get a zero-value receiver for option processing.
func NewModelConfig() *modelConfig {
	return &modelConfig{}
}

// ModelConfigFunctionCalling reports whether function calling was requested via options.
func ModelConfigFunctionCalling(mc *modelConfig) bool {
	if mc == nil {
		return false
	}
	return mc.functionCalling
}
