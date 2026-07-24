package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	lmm "kratos-demo/internal/biz/llm"
	bizprovider "kratos-demo/internal/biz/provider"

	"kratos-demo/internal/conf"
	"kratos-demo/internal/data/provider"
)

type fakeLLM struct{}

func (fakeLLM) GenerateContent(_ context.Context, messages []lmm.MessageContent, _ ...lmm.CallOption) (*lmm.ContentResponse, error) {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if text, ok := part.(lmm.TextContent); ok {
				parts = append(parts, text.Text)
			}
		}
	}
	return &lmm.ContentResponse{Choices: []*lmm.ContentChoice{{Content: strings.Join(parts, "\n")}}}, nil
}

func (fakeLLM) Call(_ context.Context, prompt string, _ ...lmm.CallOption) (string, error) {
	return prompt, nil
}

type fakeProvider struct{}

func (fakeProvider) GetProviderName() bizprovider.ProviderType { return bizprovider.ProviderTypeOpenAI }

func (fakeProvider) CreateModel(_ ...provider.ModelOption) (lmm.ModelClient, error) {
	return fakeLLM{}, nil
}

func WithFakeRuntimeLLM(t *testing.T) {
	t.Helper()
	prev := newProviderFn
	newProviderFn = func(_ *conf.AI, _ *log.Helper) (provider.LLMProvider, error) {
		return fakeProvider{}, nil
	}
	t.Cleanup(func() {
		newProviderFn = prev
	})
}
