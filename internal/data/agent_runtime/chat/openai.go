package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// openaiClient 使用 OpenAI 原生 Go SDK 实现 ChatClient 接口。
type openaiClient struct {
	client *openai.Client
	model  string
}

// NewOpenAIClient 创建一个使用 OpenAI 原生 SDK 的 ChatClient。
// baseURL 可选，为空时使用 OpenAI 官方 API 地址。
func NewOpenAIClient(apiKey, baseURL, model string) ChatClient {
	cfg := openai.DefaultConfig(apiKey)
	if strings.TrimSpace(baseURL) != "" {
		cfg.BaseURL = strings.TrimSpace(baseURL)
	}
	return &openaiClient{
		client: openai.NewClientWithConfig(cfg),
		model:  strings.TrimSpace(model),
	}
}

func (c *openaiClient) Chat(ctx context.Context, messages []ChatMessage, tools []ToolDef) (*ChatResponse, error) {
	chatMessages := toOpenAIMessages(messages)
	req := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: chatMessages,
	}
	if len(tools) > 0 {
		req.Tools = toOpenAITools(tools)
		req.ToolChoice = "auto"
	}
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai create chat completion: %w", err)
	}
	return fromOpenAIResponse(resp), nil
}

// toOpenAIMessages 将 neutral ChatMessage 转为 OpenAI SDK 的消息格式。
func toOpenAIMessages(messages []ChatMessage) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		switch role {
		case "system":
			out = append(out, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: msg.Content,
			})
		case "user":
			out = append(out, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: msg.Content,
			})
		case "assistant":
			assistantMsg := openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: msg.Content,
			}
			if len(msg.ToolCalls) > 0 {
				assistantMsg.ToolCalls = make([]openai.ToolCall, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, openai.ToolCall{
						ID:   tc.ID,
						Type: openai.ToolType(tc.Type),
						Function: openai.FunctionCall{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
			}
			out = append(out, assistantMsg)
		case "tool":
			out = append(out, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    msg.Content,
				ToolCallID: msg.ToolCallID,
				Name:       msg.Name,
			})
		}
	}
	return out
}

// toOpenAITools 将 neutral ToolDef 转为 OpenAI SDK 的工具定义格式。
func toOpenAITools(tools []ToolDef) []openai.Tool {
	out := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if params == nil {
			params = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		out = append(out, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

// fromOpenAIResponse 将 OpenAI SDK 响应转为 ChatResponse。
func fromOpenAIResponse(resp openai.ChatCompletionResponse) *ChatResponse {
	if len(resp.Choices) == 0 {
		return nil
	}
	choice := resp.Choices[0]
	cr := &ChatResponse{
		Content:   strings.TrimSpace(choice.Message.Content),
		ToolCalls: make([]ToolCall, 0, len(choice.Message.ToolCalls)),
	}
	for _, tc := range choice.Message.ToolCalls {
		cr.ToolCalls = append(cr.ToolCalls, ToolCall{
			ID:        tc.ID,
			Type:      string(tc.Type),
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return cr
}

// IsOpenAIError 检查错误是否为 OpenAI API 错误，并返回状态码（如果不是则返回 0）。
func IsOpenAIError(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatusCode, true
	}
	return 0, false
}
