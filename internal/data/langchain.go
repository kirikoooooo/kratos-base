package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	actorpkg "kratos-demo/third_party/actor"
	toolcatalog "kratos-demo/third_party/tools"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultOpenAIModel      = "gpt-4o-mini"
	defaultOpenAITimeout    = 120 * time.Second
	defaultRemoteTimeout    = 8 * time.Second
	runtimeActorPID         = 9001
	maxToolLoopIterations   = 12
	maxToolCorrectionRounds = 5
	maxExecCommandAttempts  = 3
)

var newRuntimeLLM = func(config *conf.AI, logger *log.Helper) (llms.Model, error) {
	return newRuntimeLLMForPurpose(config, logger, "")
}

var newRuntimeLLMForPurpose = func(config *conf.AI, logger *log.Helper, purpose string) (llms.Model, error) {
	if config == nil {
		return nil, errors.New("ai config is nil")
	}
	apiKey := strings.TrimSpace(config.GetOpenai().GetApiKey())
	if apiKey == "" {
		err := errors.New("ai.openai.api_key is empty")
		logger.Warn(err.Error())
		return nil, err
	}

	model := strings.TrimSpace(config.GetOpenai().GetModel())
	if model == "" {
		model = defaultOpenAIModel
	}
	if purpose == "function_calling" {
		model = normalizeFunctionCallingModel(model)
	}

	options := []openai.Option{
		openai.WithToken(apiKey),
		openai.WithModel(model),
	}
	if baseURL := strings.TrimSpace(config.GetOpenai().GetBaseUrl()); baseURL != "" {
		options = append(options, openai.WithBaseURL(baseURL))
	}
	options = append(options, openai.WithHTTPClient(newOpenAIHTTPClient(openAITimeoutFromConfig(config))))

	modelClient, err := openai.New(options...)
	if err != nil {
		logger.Warnf("create openai compatible llm failed: %v", err)
		return nil, fmt.Errorf("create openai compatible llm failed: %w", err)
	}
	return modelClient, nil
}

func openAITimeoutFromConfig(config *conf.AI) time.Duration {
	if config == nil || config.GetOpenai() == nil {
		return defaultOpenAITimeout
	}
	if seconds := config.GetOpenai().GetTimeoutSeconds(); seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultOpenAITimeout
}

func newOpenAIHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultOpenAITimeout
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

type langChainAgentRuntime struct {
	config         *conf.AI
	runtimeConfig  *conf.Runtime
	log            *log.Helper
	pid            actorpkg.PID
	trace          biz.DelegationTraceStore
	memory         *biz.AgentMemoryUsecase
	localTools     *localToolRuntime
	localToolsErr  error
	toolCatalog    *toolcatalog.Catalog
	toolCatalogErr error
}

func NewAgentRuntime(config *conf.AI, runtimeConfig *conf.Runtime, trace biz.DelegationTraceStore, changes biz.SessionChangeStore, memory *biz.AgentMemoryUsecase, logger log.Logger) biz.AgentRuntime {
	helper := log.NewHelper(logger)
	catalog, err := toolcatalog.DefaultCatalog()
	if err != nil {
		helper.Warnf("load embedded tool catalog failed: %v", err)
	}
	localTools, localToolsErr := newLocalToolRuntime(trace, changes)
	if localToolsErr != nil {
		helper.Warnf("create local tool runtime failed: %v", localToolsErr)
	}
	return &langChainAgentRuntime{
		config:         config,
		runtimeConfig:  runtimeConfig,
		log:            helper,
		pid:            actorpkg.NewPID(runtimeActorPID, "langchain-runtime"),
		trace:          trace,
		memory:         memory,
		localTools:     localTools,
		localToolsErr:  localToolsErr,
		toolCatalog:    catalog,
		toolCatalogErr: err,
	}
}

func (r *langChainAgentRuntime) PID() actorpkg.PID {
	return r.pid
}

func (r *langChainAgentRuntime) Process(msg *actorpkg.Message) {
	if msg == nil {
		return
	}

	cmd, ok := msg.Data.(*taskv1.TaskCommand)
	if !ok || cmd == nil {
		msg.Response(actorpkg.RespMessage{Err: errors.New("invalid runtime task command")})
		return
	}

	result, err := r.ReceiveTask(context.Background(), cmd)
	msg.Response(actorpkg.RespMessage{Err: err, Data: result})
}

func (r *langChainAgentRuntime) OnStop() {
	r.log.Infof("agent runtime stopped: %s", r.Name())
}

func (r *langChainAgentRuntime) Name() string {
	return "langchaingo-openai-runtime"
}

func (r *langChainAgentRuntime) Supports(agent biz.TaskAgent) bool {
	switch agent {
	case biz.TaskAgentDefault, biz.TaskAgentGeneric, biz.TaskAgentRouter, biz.TaskAgentCoder, biz.TaskAgentReviewer:
		return true
	default:
		return false
	}
}

func (r *langChainAgentRuntime) Execute(ctx context.Context, agent biz.TaskAgent, prompt string) (*taskv1.TaskResult, error) {
	normalized := normalizeRuntimeAgent(agent)
	ctx = withTaskAgent(ctx, normalized)
	if r.memory != nil {
		if sessionID := currentTaskID(ctx); sessionID != "" {
			if err := r.memory.PrepareForTask(ctx, sessionID, normalized); err != nil {
				r.log.Warnf("prepare agent memory failed: %v", err)
			}
		}
	}
	switch normalized {
	case biz.TaskAgentDefault:
		return r.runDefault(ctx, prompt)
	case biz.TaskAgentRouter:
		return r.runRouter(ctx, prompt)
	case biz.TaskAgentCoder:
		return r.runCoder(ctx, prompt)
	case biz.TaskAgentReviewer:
		return r.runReviewer(ctx, prompt)
	default:
		return nil, fmt.Errorf("%w: %s", biz.ErrAgentNotSupported, agent)
	}
}

func (r *langChainAgentRuntime) ReceiveTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if cmd == nil {
		return nil, errors.New("task command is nil")
	}
	return r.Execute(withTaskID(ctx, cmd.TaskID), normalizeRuntimeAgent(biz.TaskAgent(cmd.Agent)), cmd.Prompt)
}

func (r *langChainAgentRuntime) SendTask(_ context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if r == nil {
		return nil, errors.New("agent runtime is not available")
	}
	if cmd == nil {
		return nil, errors.New("task command is nil")
	}
	resp := r.syncRequest(nil, &actorpkg.Message{
		Id:   1001,
		Data: cmd,
	})
	if resp.Err != nil {
		return nil, resp.Err
	}
	result, _ := resp.Data.(*taskv1.TaskResult)
	if result == nil {
		return nil, errors.New("task result is nil")
	}
	return result, nil
}

func (r *langChainAgentRuntime) send(from actorpkg.PID, message *actorpkg.Message) error {
	if r == nil {
		return errors.New("agent runtime is not available")
	}
	return actorpkg.Send(from, r.PID(), message)
}

func (r *langChainAgentRuntime) syncRequest(from actorpkg.PID, message *actorpkg.Message) actorpkg.RespMessage {
	if r == nil {
		return actorpkg.RespMessage{Err: errors.New("agent runtime is not available")}
	}
	return actorpkg.SyncRequest(from, r.PID(), message)
}

func (r *langChainAgentRuntime) asyncRequest(from actorpkg.PID, message *actorpkg.Message, cb func(actorpkg.RespMessage)) {
	if r == nil {
		if cb != nil {
			cb(actorpkg.RespMessage{Err: errors.New("agent runtime is not available")})
		}
		return
	}
	actorpkg.AsyncRequest(from, r.PID(), message, cb)
}

func (r *langChainAgentRuntime) runRouter(ctx context.Context, prompt string) (*taskv1.TaskResult, error) {
	modelClient, err := r.newFunctionCallingLLM()
	if err != nil {
		return nil, err
	}

	bindings := []toolcatalog.BindingSpec{
		{Name: "coder_agent", Handler: func(toolCtx context.Context, input string) (string, error) {
			result, err := r.dispatchSubTask(toolCtx, biz.TaskAgentCoder, input)
			if err != nil {
				return "", err
			}
			return formatTaskResult("CoderAgent", result), nil
		}},
		{Name: "reviewer_agent", Handler: func(toolCtx context.Context, input string) (string, error) {
			result, err := r.dispatchSubTask(toolCtx, biz.TaskAgentReviewer, input)
			if err != nil {
				return "", err
			}
			return formatTaskResult("ReviewerAgent", result), nil
		}},
	}
	toolset, err := r.buildManagedToolset(bindings)
	if err != nil {
		return nil, err
	}

	promptLines := []string{
		"你是 RouterAgent，负责协调本地 Agent 完成任务。",
		"请优先使用可用函数来完成编码、实现、审查等子任务，而不是直接假设工具执行结果。",
		"当任务同时包含开发与审查时，优先调用 coder_agent，再根据结果调用 reviewer_agent。",
		"当你已经拿到足够的工具结果后，再直接输出最终中文结论。",
	}
	systemPrompt := strings.Join(promptLines, "\n")

	return r.runToolCallingLoop(ctx, modelClient, toolset, systemPrompt, prompt, "router agent 已通过本地 tool calling 完成编排")
}

func (r *langChainAgentRuntime) runDefault(ctx context.Context, prompt string) (*taskv1.TaskResult, error) {
	modelClient, err := r.newFunctionCallingLLM()
	if err != nil {
		return nil, err
	}

	toolset, err := r.buildManagedToolset(nil)
	if err != nil {
		return nil, err
	}

	systemPrompt := strings.Join([]string{
		"你是一个通用代码代理运行时，负责直接理解任务并给出可执行结果。",
		"当任务涉及查看仓库、修改文件或执行本地验证时，优先调用可用函数，不要假设工具已经执行。",
		"可用工具覆盖读文件、edit_file 增量编辑、write_file 新建文件和执行受限验证命令；拿到工具结果后，再用中文给出真实总结。",
		"如果任务不需要工具，也可以直接回答，但不能编造执行结果。",
	}, "\n")

	return r.runToolCallingLoop(ctx, modelClient, toolset, systemPrompt, prompt, "default agent profile 已通过通用 runtime 完成任务")
}

func (r *langChainAgentRuntime) runCoder(ctx context.Context, prompt string) (*taskv1.TaskResult, error) {
	modelClient, err := r.newFunctionCallingLLM()
	if err != nil {
		return nil, err
	}

	bindings := []toolcatalog.BindingSpec{
		{Name: "router_agent", Handler: func(toolCtx context.Context, input string) (string, error) {
			result, err := r.dispatchSubTask(toolCtx, biz.TaskAgentRouter, input)
			if err != nil {
				return "", err
			}
			return formatTaskResult("RouterAgent", result), nil
		}},
		{Name: "reviewer_agent", Handler: func(toolCtx context.Context, input string) (string, error) {
			result, err := r.dispatchSubTask(toolCtx, biz.TaskAgentReviewer, input)
			if err != nil {
				return "", err
			}
			return formatTaskResult("ReviewerAgent", result), nil
		}},
	}
	toolset, err := r.buildManagedToolset(bindings)
	if err != nil {
		return nil, err
	}

	promptLines := []string{
		"你是 CoderAgent，负责真实完成实现、编码、原型设计、接口定义与技术方案落地。",
		"请直接基于用户输入给出真实中文结果，不要返回模板化占位文本，不要假设自己已经完成未执行的操作。",
		"当任务更适合先做任务拆解、协调或改由 RouterAgent 统筹时，调用 router_agent。",
		"当你已经产出实现方案且需要补充审查意见时，可以调用 reviewer_agent。",
		"如果任务可以直接回答，就直接输出最终中文结果。",
	}
	systemPrompt := strings.Join(promptLines, "\n")

	return r.runToolCallingLoop(ctx, modelClient, toolset, systemPrompt, prompt, "coder agent 已通过本地 tool calling 完成生成")
}

func normalizeRuntimeAgent(agent biz.TaskAgent) biz.TaskAgent {
	switch biz.TaskAgent(strings.TrimSpace(strings.ToLower(agent.String()))) {
	case "", biz.TaskAgentDefault, biz.TaskAgentGeneric:
		return biz.TaskAgentDefault
	default:
		return biz.TaskAgent(strings.TrimSpace(strings.ToLower(agent.String())))
	}
}

func (r *langChainAgentRuntime) runReviewer(ctx context.Context, prompt string) (*taskv1.TaskResult, error) {
	return r.runPlainLLMTask(ctx, strings.Join([]string{
		"你是 ReviewerAgent，负责进行代码审查、设计评审、边界条件检查和风险识别。",
		"请直接给出真实中文评审结论、问题清单、风险点和改进建议。",
		"不要返回占位文本，不要说自己尚未开始，直接输出有用内容。",
	}, "\n"), prompt, "reviewer agent 已通过真实 LLM 完成审查")
}

func (r *langChainAgentRuntime) newLLM() (llms.Model, error) {
	return newRuntimeLLM(r.config, r.log)
}

func (r *langChainAgentRuntime) newFunctionCallingLLM() (llms.Model, error) {
	return newRuntimeLLMForPurpose(r.config, r.log, "function_calling")
}

type managedToolset struct {
	Tools    []llms.Tool
	Handlers map[string]toolcatalog.Handler
}

func (r *langChainAgentRuntime) buildManagedToolset(bindings []toolcatalog.BindingSpec) (*managedToolset, error) {
	if r == nil {
		return nil, errors.New("agent runtime is nil")
	}
	if r.localToolsErr != nil {
		return nil, fmt.Errorf("local tool runtime unavailable: %w", r.localToolsErr)
	}
	if r.toolCatalogErr != nil {
		return nil, fmt.Errorf("tool catalog unavailable: %w", r.toolCatalogErr)
	}
	if r.toolCatalog == nil {
		return nil, errors.New("tool catalog is nil")
	}
	if r.localTools != nil {
		bindings = append(bindings, r.localTools.bindings()...)
	}
	names := make([]string, 0, len(bindings))
	handlers := make(map[string]toolcatalog.Handler, len(bindings))
	for _, binding := range bindings {
		name := strings.TrimSpace(binding.Name)
		if name == "" {
			return nil, errors.New("tool binding name is empty")
		}
		if binding.Handler == nil {
			return nil, fmt.Errorf("tool handler is nil: %s", name)
		}
		names = append(names, name)
		handlers[name] = binding.Handler
	}
	tools, err := r.toolCatalog.AsLLMTools(names)
	if err != nil {
		return nil, err
	}
	return &managedToolset{
		Tools:    tools,
		Handlers: handlers,
	}, nil
}

func (r *langChainAgentRuntime) runToolCallingLoop(ctx context.Context, modelClient llms.Model, toolset *managedToolset, systemPrompt, prompt, summary string) (*taskv1.TaskResult, error) {
	if modelClient == nil {
		return nil, errors.New("llm model is nil")
	}
	systemText := strings.TrimSpace(systemPrompt) + "\n修改已有文件必须先 read_file，再用 edit_file 原子操作：insert_line/insert_after_line/prepend/append 插入，delete_line/delete_lines/delete_string 删除，replace_line/replace_lines/search_replace 替换；行号从 1 开始；write_file 仅新建文件。\n分析目录结构时对目录调用 read_file（如 read_file internal/biz）会返回条目列表，再逐个 read_file 具体 .go 文件；不要依赖 exec_command 做目录搜索。\n若 read_file 因路径不存在失败，先尝试 read_file 父目录或修正相对路径；exec_command 全任务最多 3 次且失败后会禁用，优先 read_file。"
	if r.memory != nil {
		if block := strings.TrimSpace(r.memory.RenderPromptContext(ctx, currentTaskID(ctx))); block != "" {
			systemText += "\n\n## Agent Memory（提示词调优）\n" + block
		}
	}

	sessionID := currentTaskID(ctx)
	turns := []biz.ConversationTurn{}
	if r.memory != nil && sessionID != "" {
		if conv, err := r.memory.LoadConversation(ctx, sessionID); err == nil && conv != nil {
			turns = conv.Turns
		}
	}
	turns = biz.AppendHumanTurnIfNeeded(turns, prompt)
	if r.memory != nil {
		var meta biz.ContextPrepareMeta
		turns, meta = r.memory.PrepareTurnsForLLMWithMeta(ctx, sessionID, turns)
		r.publishContextUsage(ctx, sessionID, meta)
	}
	messages := buildLLMMessages(systemText, turns)
	correctionRounds := 0
	toolAttempts := map[string]int{}

	for i := 0; i < maxToolLoopIterations; i++ {
		if r.memory != nil && sessionID != "" && len(messages) > 1 {
			loopTurns := conversationTurnsFromLLM(messages[1:])
			var meta biz.ContextPrepareMeta
			loopTurns, meta = r.memory.PrepareTurnsForLLMWithMeta(ctx, sessionID, loopTurns)
			r.publishContextUsage(ctx, sessionID, meta)
			messages = buildLLMMessages(systemText, loopTurns)
		}
		callOptions := []llms.CallOption{}
		if toolset != nil && len(toolset.Tools) > 0 {
			callOptions = append(callOptions, llms.WithTools(toolset.Tools), llms.WithToolChoice("auto"))
		}
		response, err := modelClient.GenerateContent(ctx, messages, callOptions...)
		if err != nil {
			r.log.Warnf("tool calling loop failed: %v", err)
			r.recordSessionError(ctx, "llm_generate", "", err.Error(), "")
			return nil, fmt.Errorf("tool calling loop failed: %w", err)
		}
		if response == nil || len(response.Choices) == 0 {
			r.recordSessionError(ctx, "llm_empty_response", "", "tool calling loop returned empty response", "")
			return nil, errors.New("tool calling loop returned empty response")
		}

		choice := response.Choices[0]
		output := strings.TrimSpace(choice.Content)
		toolCalls := normalizeToolCalls(choice)
		if len(toolCalls) == 0 {
			if output == "" {
				r.recordSessionError(ctx, "llm_empty_output", "", "tool calling loop returned empty output", "")
				return nil, errors.New("tool calling loop returned empty output")
			}
			messages = append(messages, llms.TextParts(llms.ChatMessageTypeAI, output))
			r.persistConversation(ctx, sessionID, messages)
			return &taskv1.TaskResult{
				Summary: summary,
				Output:  output,
			}, nil
		}

		assistantParts := make([]llms.ContentPart, 0, len(toolCalls)+1)
		if output != "" {
			assistantParts = append(assistantParts, llms.TextContent{Text: output})
		}
		for _, tc := range toolCalls {
			assistantParts = append(assistantParts, tc)
		}
		messages = append(messages, llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: assistantParts,
		})

		roundFailed := false
		var lastToolErr error
		for _, tc := range toolCalls {
			toolName := ""
			if tc.FunctionCall != nil {
				toolName = strings.TrimSpace(tc.FunctionCall.Name)
			}
			if toolName != "" {
				toolAttempts[toolName]++
			}
			if toolName == "exec_command" && toolAttempts[toolName] > maxExecCommandAttempts {
				quotaErr := errors.New("exec_command limit reached for this task; use read_file on directories (e.g. read_file internal/biz) and specific files instead")
				r.recordSessionError(ctx, "exec_command_quota", toolName, quotaErr.Error(), "")
				observation := formatToolErrorObservation(tc, "", quotaErr, correctionRounds+1)
				toolRespName := toolName
				if tc.FunctionCall != nil && strings.TrimSpace(tc.FunctionCall.Name) != "" {
					toolRespName = tc.FunctionCall.Name
				}
				messages = append(messages, llms.MessageContent{
					Role: llms.ChatMessageTypeTool,
					Parts: []llms.ContentPart{llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       toolRespName,
						Content:    observation,
					}},
				})
				roundFailed = true
				lastToolErr = quotaErr
				continue
			}

			observation, err := callManagedTool(ctx, toolset, tc)
			if err != nil {
				roundFailed = true
				lastToolErr = err
				r.recordSessionError(ctx, "tool_error", toolName, err.Error(), observation)
				observation = formatToolErrorObservation(tc, observation, err, correctionRounds+1)
			} else if r.memory != nil && toolName != "" {
				if recordErr := r.memory.RecordSessionToolUsage(ctx, currentTaskID(ctx), toolName); recordErr != nil {
					r.log.Warnf("record session tool usage failed: %v", recordErr)
				}
			}
			toolRespName := toolName
			if tc.FunctionCall != nil && strings.TrimSpace(tc.FunctionCall.Name) != "" {
				toolRespName = tc.FunctionCall.Name
			}
			messages = append(messages, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{llms.ToolCallResponse{
					ToolCallID: tc.ID,
					Name:       toolRespName,
					Content:    observation,
				}},
			})
		}
		r.persistConversation(ctx, sessionID, messages)
		if roundFailed {
			correctionRounds++
			if correctionRounds >= maxToolCorrectionRounds {
				if lastToolErr != nil {
					msg := fmt.Sprintf("tool self-correction exhausted after %d rounds: %v", maxToolCorrectionRounds, lastToolErr)
					r.recordSessionError(ctx, "tool_correction_exhausted", "", msg, lastToolErr.Error())
					return nil, fmt.Errorf("tool self-correction exhausted after %d rounds: %w", maxToolCorrectionRounds, lastToolErr)
				}
				r.recordSessionError(ctx, "tool_correction_exhausted", "", fmt.Sprintf("tool self-correction exhausted after %d rounds", maxToolCorrectionRounds), "")
				return nil, fmt.Errorf("tool self-correction exhausted after %d rounds", maxToolCorrectionRounds)
			}
			continue
		}
		correctionRounds = 0
	}
	r.recordSessionError(ctx, "tool_loop_exhausted", "", "tool calling loop exceeded max iterations", "")
	return nil, errors.New("tool calling loop exceeded max iterations")
}

func (r *langChainAgentRuntime) recordSessionError(ctx context.Context, stage, tool, message, detail string) {
	if r == nil || r.memory == nil {
		return
	}
	sessionID := currentTaskID(ctx)
	if sessionID == "" {
		return
	}
	record := biz.SessionErrorRecord{
		SessionID: sessionID,
		Agent:     currentTaskAgent(ctx).String(),
		Stage:     strings.TrimSpace(stage),
		Tool:      strings.TrimSpace(tool),
		Message:   strings.TrimSpace(message),
		Detail:    strings.TrimSpace(detail),
	}
	if err := r.memory.RecordSessionError(ctx, record); err != nil {
		r.log.Warnf("record session error failed: %v", err)
	}
}

func formatToolErrorObservation(call llms.ToolCall, output string, err error, attempt int) string {
	name := ""
	if call.FunctionCall != nil {
		name = strings.TrimSpace(call.FunctionCall.Name)
	}
	parts := []string{
		"status: error",
		"tool: " + name,
		fmt.Sprintf("attempt: %d/%d", attempt, maxToolCorrectionRounds),
		"error: " + strings.TrimSpace(err.Error()),
		"请根据这个错误修正参数、路径或工具选择后重试。",
	}
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		parts = append(parts, "partial_output:\n"+trimmed)
	}
	return strings.Join(parts, "\n")
}

func normalizeToolCalls(choice *llms.ContentChoice) []llms.ToolCall {
	if choice == nil {
		return nil
	}
	if len(choice.ToolCalls) > 0 {
		out := make([]llms.ToolCall, len(choice.ToolCalls))
		for i, tc := range choice.ToolCalls {
			out[i] = normalizeLLMToolCall(tc)
		}
		return out
	}
	if choice.FuncCall == nil {
		return nil
	}
	return []llms.ToolCall{{
		ID:   fmt.Sprintf("legacy-func-%d", time.Now().UnixNano()),
		Type: "function",
		FunctionCall: &llms.FunctionCall{
			Name:      choice.FuncCall.Name,
			Arguments: choice.FuncCall.Arguments,
		},
	}}
}

func callManagedTool(ctx context.Context, toolset *managedToolset, call llms.ToolCall) (string, error) {
	if toolset == nil {
		return "", errors.New("managed toolset is nil")
	}
	if call.FunctionCall == nil {
		return "", errors.New("tool call function payload is nil")
	}
	handler, ok := toolset.Handlers[strings.TrimSpace(call.FunctionCall.Name)]
	if !ok {
		return "", fmt.Errorf("tool handler not found: %s", call.FunctionCall.Name)
	}
	input, err := normalizeToolInput(call.FunctionCall.Arguments)
	if err != nil {
		return "", err
	}
	output, callErr := handler(ctx, input)
	if callErr != nil {
		if strings.TrimSpace(output) != "" {
			return output, fmt.Errorf("tool %s failed: %w", call.FunctionCall.Name, callErr)
		}
		return "", fmt.Errorf("tool %s failed: %w", call.FunctionCall.Name, callErr)
	}
	return output, nil
}

func normalizeToolInput(arguments string) (string, error) {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return "", nil
	}
	var payload struct {
		Input string `json:"input"`
	}
	if strings.HasPrefix(arguments, "{") && json.Unmarshal([]byte(arguments), &payload) == nil {
		if strings.TrimSpace(payload.Input) != "" {
			return strings.TrimSpace(payload.Input), nil
		}
	}
	var legacy struct {
		Arg1 string `json:"__arg1"`
	}
	if strings.HasPrefix(arguments, "{") && json.Unmarshal([]byte(arguments), &legacy) == nil {
		if strings.TrimSpace(legacy.Arg1) != "" {
			return strings.TrimSpace(legacy.Arg1), nil
		}
	}
	return arguments, nil
}

func (r *langChainAgentRuntime) runFunctionsAgent(_ context.Context, _ any, _, _ string) (*taskv1.TaskResult, error) {
	return nil, errors.New("runFunctionsAgent is no longer used")
}

func (r *langChainAgentRuntime) publishContextUsage(ctx context.Context, sessionID string, meta biz.ContextPrepareMeta) {
	if r.trace == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	usage := biz.NewContextUsageSnapshot(meta.Stats)
	r.trace.UpdateContextUsage(sessionID, usage, meta.Compress)
	if meta.Compress == nil || !meta.Compress.Compressed {
		return
	}
	agent := currentTaskAgent(ctx).String()
	if agent == "" {
		agent = "runtime"
	}
	r.trace.AppendEvent(biz.DelegationEvent{
		Time:       time.Now(),
		TaskID:     sessionID,
		Agent:      agent,
		Stage:      "context_compress",
		Summary:    biz.FormatContextCompressEventSummary(*meta.Compress),
		ToolOutput: biz.FormatContextCompressEventOutput(*meta.Compress, meta.Stats.Threshold),
	})
}

func (r *langChainAgentRuntime) persistConversation(ctx context.Context, sessionID string, messages []llms.MessageContent) {
	if r.memory == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	turns := conversationTurnsFromLLM(messages)
	if len(turns) == 0 {
		return
	}
	conv := &biz.SessionConversation{
		SessionID: sessionID,
		Agent:     currentTaskAgent(ctx).String(),
		Turns:     turns,
	}
	if err := r.memory.SaveConversation(ctx, conv); err != nil {
		r.log.Warnf("persist conversation failed: session=%s err=%v", sessionID, err)
	}
}

func (r *langChainAgentRuntime) runPlainLLMTask(ctx context.Context, systemPrompt, prompt, summary string) (*taskv1.TaskResult, error) {
	modelClient, err := r.newLLM()
	if err != nil {
		return nil, err
	}

	fullPrompt := strings.Join([]string{
		systemPrompt,
		"",
		"用户任务：",
		strings.TrimSpace(prompt),
	}, "\n")

	output, err := llms.GenerateFromSinglePrompt(ctx, modelClient, fullPrompt, llms.WithTemperature(0.2))
	if err != nil {
		r.log.Warnf("plain llm task call failed: %v", err)
		return nil, fmt.Errorf("plain llm task call failed: %w", err)
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, errors.New("plain llm returned empty output")
	}

	sessionID := currentTaskID(ctx)
	if r.memory != nil && sessionID != "" {
		_ = r.memory.SaveConversation(ctx, &biz.SessionConversation{
			SessionID: sessionID,
			Agent:     currentTaskAgent(ctx).String(),
			Turns: []biz.ConversationTurn{
				{Role: biz.ConversationRoleHuman, Content: strings.TrimSpace(prompt)},
				{Role: biz.ConversationRoleAI, Content: output},
			},
		})
	}

	return &taskv1.TaskResult{
		Summary: summary,
		Output:  output,
	}, nil
}

func (r *langChainAgentRuntime) dispatchSubTask(ctx context.Context, agent biz.TaskAgent, prompt string) (*taskv1.TaskResult, error) {
	cmd := &taskv1.TaskCommand{
		Agent:  agent.String(),
		Prompt: strings.TrimSpace(prompt),
	}
	if cmd.Prompt == "" {
		return nil, errors.New("sub task prompt is empty")
	}

	if remote := r.lookupRemoteAgent(agent); remote != nil && strings.TrimSpace(remote.GetTarget()) != "" {
		r.log.Infof("router delegating sub task to remote agent=%s target=%s", agent, remote.GetTarget())
		if r.trace != nil {
			r.trace.AppendEvent(biz.DelegationEvent{
				Time:          time.Now(),
				TaskID:        currentTaskID(ctx),
				Agent:         biz.TaskAgentRouter.String(),
				Stage:         "delegate_remote",
				Mode:          agent.String(),
				Target:        remote.GetTarget(),
				PromptPreview: previewPrompt(prompt),
			})
		}
		return r.executeRemoteTask(ctx, remote, cmd)
	}

	if r.trace != nil {
		r.trace.AppendEvent(biz.DelegationEvent{
			Time:          time.Now(),
			TaskID:        currentTaskID(ctx),
			Agent:         biz.TaskAgentRouter.String(),
			Stage:         "delegate_local",
			Mode:          agent.String(),
			PromptPreview: previewPrompt(prompt),
		})
	}
	return r.ReceiveTask(ctx, cmd)
}

func (r *langChainAgentRuntime) lookupRemoteAgent(agent biz.TaskAgent) *conf.Runtime_RemoteAgent {
	if r == nil || r.runtimeConfig == nil {
		return nil
	}
	for _, remote := range r.runtimeConfig.GetRemotes() {
		if remote == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(remote.GetAgent()), agent.String()) {
			return remote
		}
	}
	return nil
}

func (r *langChainAgentRuntime) executeRemoteTask(ctx context.Context, remote *conf.Runtime_RemoteAgent, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if remote == nil {
		return nil, errors.New("remote agent config is nil")
	}
	target := strings.TrimSpace(remote.GetTarget())
	if target == "" {
		return nil, errors.New("remote agent target is empty")
	}
	timeout := defaultRemoteTimeout
	if remote.GetTimeout() > 0 {
		timeout = time.Duration(remote.GetTimeout()) * time.Second
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, timeout)
	defer dialCancel()

	conn, err := grpcclient.DialContext(dialCtx, target,
		grpcclient.WithTransportCredentials(insecure.NewCredentials()),
		grpcclient.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial remote agent %s failed: %w", target, err)
	}
	defer conn.Close()

	callCtx, callCancel := context.WithTimeout(ctx, timeout)
	defer callCancel()

	client := taskv1.NewAgentRuntimeServiceClient(conn)
	start := time.Now()
	result, err := client.ExecuteTask(callCtx, cmd)
	if err != nil {
		if r.trace != nil {
			r.trace.AppendEvent(biz.DelegationEvent{
				Time:   time.Now(),
				TaskID: currentTaskID(ctx),
				Agent:  biz.TaskAgentRouter.String(),
				Stage:  "remote_execute_failed",
				Target: target,
				Error:  err.Error(),
				Mode:   cmd.GetAgent(),
			})
		}
		return nil, fmt.Errorf("remote execute task failed: %w", err)
	}
	if r.trace != nil {
		r.trace.AppendEvent(biz.DelegationEvent{
			Time:       time.Now(),
			TaskID:     currentTaskID(ctx),
			Agent:      biz.TaskAgentRouter.String(),
			Stage:      "remote_execute_done",
			Target:     target,
			Mode:       cmd.GetAgent(),
			Summary:    result.GetSummary(),
			DurationMS: time.Since(start).Milliseconds(),
		})
	}
	return result, nil
}

func formatTaskResult(agentName string, result *taskv1.TaskResult) string {
	if result == nil {
		return agentName + " 未返回结果。"
	}

	parts := make([]string, 0, 3)
	parts = append(parts, agentName+" 已完成子任务。")
	if summary := strings.TrimSpace(result.GetSummary()); summary != "" {
		parts = append(parts, "总结："+summary)
	}
	if output := strings.TrimSpace(result.GetOutput()); output != "" {
		parts = append(parts, "输出：\n"+output)
	}
	return strings.Join(parts, "\n")
}

func (r *langChainAgentRuntime) VerifyDelegation(ctx context.Context, taskID string, agent biz.TaskAgent, prompt string) (*taskv1.TaskResult, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "请验证 router -> " + agent.String() + " 的委派链路"
	}
	if taskID == "" {
		taskID = fmt.Sprintf("verify-%d", time.Now().UnixNano())
	}
	if r.memory != nil {
		if err := r.memory.StartConversation(ctx, taskID, biz.TaskAgentRouter, prompt); err != nil {
			return nil, fmt.Errorf("start conversation memory: %w", err)
		}
	}
	if r.trace != nil {
		r.trace.StartTask(taskID, biz.TaskAgentRouter, biz.TaskStatusRunning)
		r.trace.AppendEvent(biz.DelegationEvent{
			Time:          time.Now(),
			TaskID:        taskID,
			Agent:         biz.TaskAgentRouter.String(),
			Stage:         "verification_start",
			Mode:          agent.String(),
			PromptPreview: previewPrompt(prompt),
		})
	}
	result, err := r.dispatchSubTask(withTaskID(ctx, taskID), agent, prompt)
	if r.trace != nil {
		if err != nil {
			r.trace.AppendEvent(biz.DelegationEvent{
				Time:   time.Now(),
				TaskID: taskID,
				Agent:  biz.TaskAgentRouter.String(),
				Stage:  "verification_failed",
				Mode:   agent.String(),
				Error:  err.Error(),
			})
			r.trace.UpdateTask(taskID, biz.TaskStatusFailed, nil, err)
		} else {
			r.trace.AppendEvent(biz.DelegationEvent{
				Time:       time.Now(),
				TaskID:     taskID,
				Agent:      biz.TaskAgentRouter.String(),
				Stage:      "verification_done",
				Mode:       agent.String(),
				Summary:    result.GetSummary(),
				DurationMS: 0,
			})
			r.trace.UpdateTask(taskID, biz.TaskStatusDone, result, nil)
		}
	}
	return result, err
}

type taskIDContextKey struct{}
type taskAgentContextKey struct{}

func withTaskID(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, taskIDContextKey{}, taskID)
}

func currentTaskID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	taskID, _ := ctx.Value(taskIDContextKey{}).(string)
	return taskID
}

func withTaskAgent(ctx context.Context, agent biz.TaskAgent) context.Context {
	return context.WithValue(ctx, taskAgentContextKey{}, agent)
}

func currentTaskAgent(ctx context.Context) biz.TaskAgent {
	if ctx == nil {
		return biz.TaskAgentDefault
	}
	agent, _ := ctx.Value(taskAgentContextKey{}).(biz.TaskAgent)
	if agent.IsEmpty() {
		return biz.TaskAgentDefault
	}
	return agent
}

func normalizeFunctionCallingModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return defaultOpenAIModel
	}
	if strings.HasPrefix(strings.ToLower(model), "gpt-5") {
		return defaultOpenAIModel
	}
	return model
}
