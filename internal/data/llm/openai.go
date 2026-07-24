// Package llm provides LLM client implementations using the official OpenAI Go SDK.
package llm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	bizllm "kratos-demo/internal/biz/llm"

	openai "github.com/sashabaranov/go-openai"
)

// ModelConfig 创建 ModelClient 的配置。
type ModelConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// NewOpenAIModelClient creates a ModelClient backed by the OpenAI-compatible API.
func NewOpenAIModelClient(config ModelConfig) bizllm.ModelClient {
	model := strings.TrimSpace(config.Model)
	cfg := openai.DefaultConfig(config.APIKey)
	if strings.TrimSpace(config.BaseURL) != "" {
		cfg.BaseURL = strings.TrimSpace(config.BaseURL)
	}
	if config.Timeout > 0 {
		cfg.HTTPClient = httpClient(config.Timeout)
	}
	return &openaiModelClient{
		client: openai.NewClientWithConfig(cfg),
		model:  model,
	}
}

func httpClient(timeout time.Duration) *http.Client {
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

type openaiModelClient struct {
	client *openai.Client
	model  string
}

func (c *openaiModelClient) GenerateContent(ctx context.Context, messages []bizllm.MessageContent, opts ...bizllm.CallOption) (*bizllm.ContentResponse, error) {
	cc := &bizllm.CallConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(cc)
	}

	chatMessages := toOpenAIMessages(messages)
	req := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: chatMessages,
	}
	if len(cc.Tools) > 0 {
		req.Tools = toOpenAITools(cc.Tools)
		if cc.ToolChoice != "" {
			req.ToolChoice = cc.ToolChoice
		} else {
			req.ToolChoice = "auto"
		}
	}
	if cc.HasTemp {
		req.Temperature = float32(cc.Temperature)
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai create chat completion: %w", err)
	}
	return fromOpenAIResponse(resp), nil
}

func (c *openaiModelClient) Call(ctx context.Context, prompt string, opts ...bizllm.CallOption) (string, error) {
	messages := []bizllm.MessageContent{
		bizllm.TextParts(bizllm.RoleUser, prompt),
	}
	resp, err := c.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(resp.Choices[0].Content), nil
}

// ——— 转换函数 ———

func toOpenAIMessages(messages []bizllm.MessageContent) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case bizllm.RoleSystem:
			out = append(out, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: textFromParts(msg.Parts),
			})
		case bizllm.RoleUser:
			out = append(out, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: textFromParts(msg.Parts),
			})
		case bizllm.RoleAssistant:
			assistantMsg := openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: textFromParts(msg.Parts),
			}
			toolCalls := toolCallsFromParts(msg.Parts)
			if len(toolCalls) > 0 {
				assistantMsg.ToolCalls = toolCalls
			}
			out = append(out, assistantMsg)
		case bizllm.RoleTool:
			for _, part := range msg.Parts {
				toolResp, ok := part.(bizllm.ToolCallResponse)
				if !ok {
					continue
				}
				out = append(out, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    toolResp.Content,
					ToolCallID: toolResp.ToolCallID,
					Name:       toolResp.Name,
				})
			}
		}
	}
	return out
}

func toOpenAITools(tools []bizllm.Tool) []openai.Tool {
	out := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		params := t.Function.Parameters
		if params == nil {
			params = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		out = append(out, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

func fromOpenAIResponse(resp openai.ChatCompletionResponse) *bizllm.ContentResponse {
	if len(resp.Choices) == 0 {
		return &bizllm.ContentResponse{}
	}
	choice := resp.Choices[0]
	cr := &bizllm.ContentResponse{
		Model:        strings.TrimSpace(resp.Model),
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		Choices: []*bizllm.ContentChoice{{
			Content:   strings.TrimSpace(choice.Message.Content),
			ToolCalls: make([]bizllm.ToolCallPart, 0, len(choice.Message.ToolCalls)),
		}},
	}
	for _, tc := range choice.Message.ToolCalls {
		cr.Choices[0].ToolCalls = append(cr.Choices[0].ToolCalls, bizllm.ToolCallPart{
			ID:   tc.ID,
			Type: string(tc.Type),
			FunctionCall: &bizllm.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return cr
}

func textFromParts(parts []bizllm.ContentPart) string {
	var b strings.Builder
	for _, part := range parts {
		text, ok := part.(bizllm.TextContent)
		if !ok {
			continue
		}
		if line := strings.TrimSpace(text.Text); line != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(line)
		}
	}
	return b.String()
}

func toolCallsFromParts(parts []bizllm.ContentPart) []openai.ToolCall {
	var out []openai.ToolCall
	for _, part := range parts {
		tc, ok := part.(bizllm.ToolCallPart)
		if !ok {
			continue
		}
		fc := openai.FunctionCall{}
		if tc.FunctionCall != nil {
			fc.Name = tc.FunctionCall.Name
			fc.Arguments = tc.FunctionCall.Arguments
		}
		out = append(out, openai.ToolCall{
			ID:       tc.ID,
			Type:     openai.ToolType(tc.Type),
			Function: fc,
		})
	}
	return out
}
