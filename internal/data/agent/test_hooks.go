package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/llms"

	"kratos-demo/internal/conf"
	"kratos-demo/internal/data/provider"
)

type fakeLLM struct{}

func (fakeLLM) GenerateContent(_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if text, ok := part.(llms.TextContent); ok {
				parts = append(parts, text.Text)
			}
		}
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: strings.Join(parts, "\n")}}}, nil
}

func (fakeLLM) Call(_ context.Context, prompt string, _ ...llms.CallOption) (string, error) {
	return prompt, nil
}

type fakeProvider struct{}

func (fakeProvider) CreateModel(_ ...provider.ModelOption) (llms.Model, error) {
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
