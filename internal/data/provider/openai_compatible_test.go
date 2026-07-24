package provider

import (
	"context"
	"errors"
	"testing"

	bizllm "kratos-demo/internal/biz/llm"
	bizprovider "kratos-demo/internal/biz/provider"
	"kratos-demo/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

func TestNewProviderGeminiUsesOpenAICompatibleDefaults(t *testing.T) {
	p, err := NewProvider(&conf.AI{Provider: "gemini", Gemini: &conf.AI_Gemini{ApiKey: "key"}}, log.NewHelper(log.NewStdLogger(nil)))
	if err != nil {
		t.Fatal(err)
	}
	model, err := p.CreateModel()
	if err != nil || model == nil {
		t.Fatalf("model=%v err=%v", model, err)
	}
}

func TestFallbackModelClientUsesNextProvider(t *testing.T) {
	client := &fallbackModelClient{models: []namedModelClient{{provider: bizprovider.ProviderTypeDeepSeek, client: failingModel{}}, {provider: bizprovider.ProviderTypeGemini, client: successfulModel{}}}}
	response, err := client.GenerateContent(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Choices[0].Content != "fallback" {
		t.Fatalf("response = %#v", response)
	}
}

type failingModel struct{}

func (failingModel) GenerateContent(context.Context, []bizllm.MessageContent, ...bizllm.CallOption) (*bizllm.ContentResponse, error) {
	return nil, errors.New("unavailable")
}
func (failingModel) Call(context.Context, string, ...bizllm.CallOption) (string, error) {
	return "", errors.New("unavailable")
}

type successfulModel struct{}

func (successfulModel) GenerateContent(context.Context, []bizllm.MessageContent, ...bizllm.CallOption) (*bizllm.ContentResponse, error) {
	return &bizllm.ContentResponse{Choices: []*bizllm.ContentChoice{{Content: "fallback"}}}, nil
}
func (successfulModel) Call(context.Context, string, ...bizllm.CallOption) (string, error) {
	return "fallback", nil
}
