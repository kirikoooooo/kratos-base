package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

// NewLangChainClient 将 langchaingo llms.Model 适配为 ChatClient。
func NewLangChainClient(model llms.Model) ChatClient {
	return &langChainClient{model: model}
}

type langChainClient struct {
	model llms.Model
}

func (c *langChainClient) Chat(ctx context.Context, messages []ChatMessage, tools []ToolDef) (*ChatResponse, error) {
	llmMessages := toLLMMessages(messages)
	var opts []llms.CallOption
	if len(tools) > 0 {
		llmTools := toLLMTools(tools)
		opts = append(opts, llms.WithTools(llmTools), llms.WithToolChoice("auto"))
	}
	resp, err := c.model.GenerateContent(ctx, llmMessages, opts...)
	if err != nil {
		return nil, fmt.Errorf("langchain generate content: %w", err)
	}
	return fromLLMResponse(resp), nil
}

// toLLMMessages 将 neutral ChatMessage 切片转为 langchaingo MessageContent 切片。
func toLLMMessages(messages []ChatMessage) []llms.MessageContent {
	out := make([]llms.MessageContent, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		switch role {
		case "system":
			out = append(out, llms.TextParts(llms.ChatMessageTypeSystem, msg.Content))
		case "user":
			out = append(out, llms.TextParts(llms.ChatMessageTypeHuman, msg.Content))
		case "assistant":
			parts := make([]llms.ContentPart, 0, len(msg.ToolCalls)+1)
			if content := strings.TrimSpace(msg.Content); content != "" {
				parts = append(parts, llms.TextContent{Text: content})
			}
			for _, tc := range msg.ToolCalls {
				parts = append(parts, llms.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					FunctionCall: &llms.FunctionCall{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
			if len(parts) == 0 {
				// 确保 assistant 消息至少有一个空文本部分，避免 API 报错
				parts = append(parts, llms.TextContent{Text: ""})
			}
			out = append(out, llms.MessageContent{Role: llms.ChatMessageTypeAI, Parts: parts})
		case "tool":
			out = append(out, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{llms.ToolCallResponse{
					ToolCallID: msg.ToolCallID,
					Name:       msg.Name,
					Content:    msg.Content,
				}},
			})
		}
	}
	return out
}

// toLLMTools 将 neutral ToolDef 切片转为 langchaingo Tool 切片。
func toLLMTools(tools []ToolDef) []llms.Tool {
	out := make([]llms.Tool, 0, len(tools))
	for _, t := range tools {
		out = append(out, llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return out
}

// fromLLMResponse 将 langchaingo ContentResponse 转为 ChatResponse。
func fromLLMResponse(resp *llms.ContentResponse) *ChatResponse {
	if resp == nil || len(resp.Choices) == 0 {
		return &ChatResponse{}
	}
	choice := resp.Choices[0]
	cr := &ChatResponse{
		Content:   strings.TrimSpace(choice.Content),
		ToolCalls: make([]ToolCall, 0, len(choice.ToolCalls)),
	}
	for _, tc := range choice.ToolCalls {
		name := ""
		if tc.FunctionCall != nil {
			name = strings.TrimSpace(tc.FunctionCall.Name)
		}
		cr.ToolCalls = append(cr.ToolCalls, ToolCall{
			ID:   strings.TrimSpace(tc.ID),
			Type: strings.TrimSpace(tc.Type),
			Name: name,
			Arguments: func() string {
				if tc.FunctionCall == nil {
					return ""
				}
				return strings.TrimSpace(tc.FunctionCall.Arguments)
			}(),
		})
	}
	// 兼容旧的 FuncCall 字段
	if len(cr.ToolCalls) == 0 && choice.FuncCall != nil {
		cr.ToolCalls = append(cr.ToolCalls, ToolCall{
			ID:   fmt.Sprintf("legacy-func-%d", 0),
			Type: "function",
			Name: strings.TrimSpace(choice.FuncCall.Name),
			Arguments: func() string {
				if choice.FuncCall == nil {
					return ""
				}
				return strings.TrimSpace(choice.FuncCall.Arguments)
			}(),
		})
	}
	return cr
}
