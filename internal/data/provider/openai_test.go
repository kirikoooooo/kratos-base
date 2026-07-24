package provider

import (
	"testing"

	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

func TestEveryProviderUsesOpenAICompatibleClient(t *testing.T) {
	tests := []struct {
		name string
		ai   *conf.AI
	}{
		{"openai", &conf.AI{Provider: "openai", Openai: &conf.AI_OpenAI{ApiKey: "key"}}},
		{"deepseek", &conf.AI{Provider: "deepseek", Deepseek: &conf.AI_DeepSeek{ApiKey: "key"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.ai, log.NewHelper(log.NewStdLogger(nil)))
			if err != nil {
				t.Fatal(err)
			}
			model, err := provider.CreateModel(WithFunctionCalling())
			if err != nil || model == nil {
				t.Fatalf("model=%v err=%v", model, err)
			}
		})
	}
}
