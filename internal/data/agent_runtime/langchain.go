package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
	agentcontext "kratos-demo/internal/data/agent_runtime/context"
	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
	"kratos-demo/internal/data/common"
	"kratos-demo/internal/data/mcp"
	agentmemory "kratos-demo/internal/data/memory"
	"kratos-demo/internal/data/provider"
	datasession "kratos-demo/internal/data/session"
	agenttool "kratos-demo/internal/data/tool"
	datatrace "kratos-demo/internal/data/trace"
	actorpkg "kratos-demo/third_party/actor"
	toolcatalog "kratos-demo/third_party/tools"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/llms"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultRemoteTimeout    = 8 * time.Second
	runtimeActorPID         = 9001
	maxToolLoopIterations   = 12
	maxToolCorrectionRounds = 5
	maxExecCommandAttempts  = 3

	failureAnalysisInstruction = `【系统通知】当前任务因工具多次失败或流程中断，无法继续自动执行。请不要再调用任何工具，直接向用户输出中文失败分析，必须包含：
1. **任务目标**：简要复述用户要什么
2. **已尝试的操作**：调用了哪些工具、每次失败的具体 error 是什么
3. **根因**：为什么会出现这些错误（例如绝对路径、JSON 转义、文件不存在等）
4. **建议**：用户应如何改写指令、提供什么信息，或改用什么相对路径/命令才能完成

语气清晰、具体，不要输出占位语或空内容。`
)

var newProviderFn = provider.NewProvider

type langChainAgentRuntime struct {
	config         *conf.AI
	runtimeConfig  *conf.Runtime
	log            *log.Helper
	pid            actorpkg.PID
	trace          datatrace.DelegationTraceStore
	memory         agentmemory.AgentMemory
	pvd            provider.LLMProvider
	localTools     *agenttool.Runtime
	localToolsErr  error
	toolCatalog    *toolcatalog.Catalog
	toolCatalogErr error
	mcpClients     []*mcp.Client
	mcpTools       []llms.Tool
}

func NewAgentRuntime(config *conf.AI, runtimeConfig *conf.Runtime, trace datatrace.DelegationTraceStore, sessions datasession.SessionStore, memory agentmemory.AgentMemory, logger log.Logger) biz.AgentRuntime {
	helper := log.NewHelper(logger)
	catalog, err := toolcatalog.DefaultCatalog()
	if err != nil {
		helper.Warnf("load embedded tool catalog failed: %v", err)
	}
	localTools, localToolsErr := agenttool.NewRuntime(trace, sessions)
	if localToolsErr != nil {
		helper.Warnf("create local tool runtime failed: %v", localToolsErr)
	}
	runtime := &langChainAgentRuntime{
		config:         config,
		runtimeConfig:  runtimeConfig,
		log:            helper,
		pid:            actorpkg.NewPID(runtimeActorPID, "langchain-runtime"),
		trace:          trace,
		memory:         memory,
		pvd:            nil,
		localTools:     localTools,
		localToolsErr:  localToolsErr,
		toolCatalog:    catalog,
		toolCatalogErr: err,
	}
	if runtimeConfig != nil {
		clients, tools, mcpErr := mcp.NewClients(context.Background(), runtimeConfig.GetMcpServers())
		if mcpErr != nil {
			helper.Warnf("connect MCP servers failed: %v", mcpErr)
		} else {
			runtime.mcpClients = clients
			runtime.mcpTools = tools
		}
	}
	return runtime
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
	for _, client := range r.mcpClients {
		client.Close()
	}
	r.log.Infof("agent runtime stopped: %s", r.Name())
}

func (r *langChainAgentRuntime) Name() string {
	return "langchaingo-openai-runtime"
}

func (r *langChainAgentRuntime) Supports(agent public.AgentKind) bool {
	switch agent {
	case public.AgentKindDefault, public.AgentKindGeneric, public.AgentKindRouter, public.AgentKindCoder, public.AgentKindReviewer:
		return true
	default:
		return false
	}
}

func (r *langChainAgentRuntime) Execute(ctx context.Context, agent public.AgentKind, prompt string) (*taskv1.TaskResult, error) {
	normalized := normalizeRuntimeAgent(agent)
	ctx = agentctx.WithAgent(ctx, normalized)
	if r.memory != nil {
		if sessionID := agentctx.TaskID(ctx); sessionID != "" {
			if err := r.memory.PrepareForTask(ctx, sessionID, normalized); err != nil {
				r.log.Warnf("prepare agent memory failed: %v", err)
			}
		}
	}
	switch normalized {
	case public.AgentKindDefault:
		return r.runDefault(ctx, prompt)
	case public.AgentKindRouter:
		return r.runRouter(ctx, prompt)
	case public.AgentKindCoder:
		return r.runCoder(ctx, prompt)
	case public.AgentKindReviewer:
		return r.runReviewer(ctx, prompt)
	default:
		return nil, fmt.Errorf("%w: %s", ErrAgentNotSupported, agent)
	}
}

func (r *langChainAgentRuntime) ReceiveTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if cmd == nil {
		return nil, errors.New("task command is nil")
	}
	return r.Execute(agentctx.WithTaskID(ctx, cmd.TaskID), normalizeRuntimeAgent(public.AgentKind(cmd.Agent)), cmd.Prompt)
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
			result, err := r.dispatchSubTask(toolCtx, public.AgentKindCoder, input)
			if err != nil {
				return "", err
			}
			return formatTaskResult("CoderAgent", result), nil
		}},
		{Name: "reviewer_agent", Handler: func(toolCtx context.Context, input string) (string, error) {
			result, err := r.dispatchSubTask(toolCtx, public.AgentKindReviewer, input)
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
		"若工具反复失败且无法继续，必须向用户输出完整的失败分析（原因、已尝试操作、建议），禁止无说明终止。",
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
		"若工具反复失败且无法继续，必须向用户输出完整的失败分析（原因、已尝试操作、建议），禁止无说明终止。",
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
			result, err := r.dispatchSubTask(toolCtx, public.AgentKindRouter, input)
			if err != nil {
				return "", err
			}
			return formatTaskResult("RouterAgent", result), nil
		}},
		{Name: "reviewer_agent", Handler: func(toolCtx context.Context, input string) (string, error) {
			result, err := r.dispatchSubTask(toolCtx, public.AgentKindReviewer, input)
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
		"若工具反复失败且无法继续，必须向用户输出完整的失败分析（原因、已尝试操作、建议），禁止无说明终止。",
	}
	systemPrompt := strings.Join(promptLines, "\n")

	return r.runToolCallingLoop(ctx, modelClient, toolset, systemPrompt, prompt, "coder agent 已通过本地 tool calling 完成生成")
}

func normalizeRuntimeAgent(agent public.AgentKind) public.AgentKind {
	switch public.AgentKind(strings.TrimSpace(strings.ToLower(string(agent)))) {
	case "", public.AgentKindDefault, public.AgentKindGeneric:
		return public.AgentKindDefault
	default:
		return public.AgentKind(strings.TrimSpace(strings.ToLower(string(agent))))
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
	pvd, err := r.getProvider()
	if err != nil {
		return nil, err
	}
	return pvd.CreateModel()
}

func (r *langChainAgentRuntime) newFunctionCallingLLM() (llms.Model, error) {
	pvd, err := r.getProvider()
	if err != nil {
		return nil, err
	}
	return pvd.CreateModel(provider.WithFunctionCalling())
}

// getProvider 懒加载 provider，每次调用时检查 r.config 是否已更新（如 CLI 凭证注入后）。
func (r *langChainAgentRuntime) getProvider() (provider.LLMProvider, error) {
	if r.pvd != nil {
		return r.pvd, nil
	}
	pvd, err := newProviderFn(r.config, r.log)
	if err != nil {
		return nil, err
	}
	r.pvd = pvd
	return r.pvd, nil
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
		bindings = append(bindings, r.localTools.Bindings()...)
	}
	tools := make([]llms.Tool, 0, len(bindings)+len(r.mcpTools))
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
	localToolDefs, err := r.toolCatalog.AsLLMTools(names)
	if err != nil {
		return nil, err
	}
	tools = append(tools, localToolDefs...)
	for _, client := range r.mcpClients {
		for _, tool := range client.Tools() {
			if tool.Function == nil {
				continue
			}
			name := strings.TrimSpace(tool.Function.Name)
			if _, exists := handlers[name]; exists {
				return nil, fmt.Errorf("duplicate MCP tool name: %s", name)
			}
			mcpClient := client
			toolName := name
			handlers[name] = func(ctx context.Context, input string) (string, error) {
				return mcpClient.Call(ctx, toolName, input)
			}
			tools = append(tools, tool)
		}
	}
	return &managedToolset{
		Tools:    tools,
		Handlers: handlers,
	}, nil
}

func toolCallingSystemSuffix() string {
	return strings.Join([]string{
		"修改已有文件必须先 read_file，再用 edit_file 原子操作：insert_line/insert_after_line/prepend/append 插入，delete_line/delete_lines/delete_string 删除行或片段，replace_line/replace_lines/search_replace 替换；行号从 1 开始；write_file 仅新建文件。",
		"删除整个文件必须使用 delete_file（不要用 edit_file 清空全部行代替）；delete_file 与 rm/del/git clean 等 exec_command 会在 CLI 终端弹出确认菜单（↑↓ 选择，Enter 确认），未批准时不要换工具绕过。",
		"分析目录结构时对目录调用 read_file（如 read_file internal/biz）会返回条目列表，再逐个 read_file 具体 .go 文件；不要依赖 exec_command 做目录搜索。",
		"若 read_file 因路径不存在失败，先尝试 read_file 父目录或修正相对路径；exec_command 全任务最多 3 次且失败后会禁用，优先 read_file。",
		"",
		"## 路径与工具参数",
		"- read_file / write_file / edit_file 的路径必须是相对于工作区根的相对路径，禁止使用绝对路径（如 D:\\code\\...）。",
		"- 工具 JSON 参数中的路径请使用正斜杠 /（如 scripts/hello.ps1），避免 Windows 反斜杠导致 JSON 转义错误。",
		"",
		"## 执行流程（必须遵守）",
		"- 第一步：分析用户问题，在调用任何工具之前，先输出「## 执行计划」章节，用编号列出 2-6 条可执行步骤，让用户看到你将做什么。",
		"- 第二步：按计划逐步执行；每完成一个计划步骤（一次工具调用或子 agent 调用结束后），在下一次回复开头输出一行简短进度，格式如：「✓ 步骤 1 完成：已读取 README.md」。",
		"- 第三步：全部步骤完成后，输出最终中文结论；不要把计划或进度反馈留到最后一刻才一次性输出。",
		"",
		"## 错误与收尾",
		"- 工具返回 status: error 时，先阅读 error 与 partial_output，修正参数或换用合适工具后重试。",
		"- 若确认任务无法完成，必须向用户输出中文失败分析（目标、已尝试操作、根因、下一步建议），禁止无说明终止或返回空内容。",
	}, "\n")
}

func (r *langChainAgentRuntime) runToolCallingLoop(ctx context.Context, modelClient llms.Model, toolset *managedToolset, systemPrompt, prompt, summary string) (*taskv1.TaskResult, error) {
	if modelClient == nil {
		return nil, errors.New("llm model is nil")
	}
	systemText := strings.TrimSpace(systemPrompt) + "\n" + toolCallingSystemSuffix()
	if r.memory != nil {
		if block := strings.TrimSpace(r.memory.RenderPromptContext(ctx, agentctx.TaskID(ctx))); block != "" {
			systemText += "\n\n## Agent Memory（提示词调优）\n" + block
		}
	}

	sessionID := agentctx.TaskID(ctx)
	turns := []agentcontext.ConversationTurn{}
	if r.memory != nil && sessionID != "" {
		if conv, err := r.memory.LoadConversation(ctx, sessionID); err == nil && conv != nil {
			turns = conv.Turns
		}
	}
	turns = agentcontext.AppendHumanTurnIfNeeded(turns, prompt)
	if r.memory != nil {
		var meta agentcontext.PrepareMeta
		turns, meta = r.memory.PrepareTurnsForLLMWithMeta(ctx, sessionID, turns)
		r.publishContextUsage(ctx, sessionID, meta)
	}
	messages := agentcontext.BuildLLMMessages(systemText, turns)
	correctionRounds := 0
	toolAttempts := map[string]int{}

	for i := 0; i < maxToolLoopIterations; i++ {
		if r.memory != nil && sessionID != "" && len(messages) > 1 {
			loopTurns := agentcontext.TurnsFromLLM(messages[1:])
			var meta agentcontext.PrepareMeta
			loopTurns, meta = r.memory.PrepareTurnsForLLMWithMeta(ctx, sessionID, loopTurns)
			r.publishContextUsage(ctx, sessionID, meta)
			messages = agentcontext.BuildLLMMessages(systemText, loopTurns)
		}
		callOptions := []llms.CallOption{}
		if toolset != nil && len(toolset.Tools) > 0 {
			callOptions = append(callOptions, llms.WithTools(toolset.Tools), llms.WithToolChoice("auto"))
		}
		response, err := modelClient.GenerateContent(ctx, messages, callOptions...)
		if err != nil {
			r.log.Warnf("tool calling loop failed: %v", err)
			if len(messages) > 1 {
				return r.finishWithFailureAnalysis(ctx, modelClient, messages, systemText, sessionID, summary, fmt.Errorf("tool calling loop failed: %w", err), "llm_generate")
			}
			r.recordSessionError(ctx, "llm_generate", "", err.Error(), "")
			return nil, fmt.Errorf("tool calling loop failed: %w", err)
		}
		if response == nil || len(response.Choices) == 0 {
			if len(messages) > 1 {
				return r.finishWithFailureAnalysis(ctx, modelClient, messages, systemText, sessionID, summary, errors.New("tool calling loop returned empty response"), "llm_empty_response")
			}
			r.recordSessionError(ctx, "llm_empty_response", "", "tool calling loop returned empty response", "")
			return nil, errors.New("tool calling loop returned empty response")
		}

		choice := response.Choices[0]
		output := strings.TrimSpace(choice.Content)
		toolCalls := normalizeToolCalls(choice)
		if output != "" {
			r.publishAgentProgress(ctx, output, len(toolCalls) > 0)
		}
		if len(toolCalls) == 0 {
			if output == "" {
				if len(messages) > 1 {
					return r.finishWithFailureAnalysis(ctx, modelClient, messages, systemText, sessionID, summary, errors.New("tool calling loop returned empty output"), "llm_empty_output")
				}
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
				r.publishToolFailure(ctx, toolName, tc, err)
				observation = formatToolErrorObservation(tc, observation, err, correctionRounds+1)
			} else if r.memory != nil && toolName != "" {
				if recordErr := r.memory.RecordSessionToolUsage(ctx, agentctx.TaskID(ctx), toolName); recordErr != nil {
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
				cause := lastToolErr
				if cause == nil {
					cause = fmt.Errorf("tool self-correction exhausted after %d rounds", maxToolCorrectionRounds)
				} else {
					cause = fmt.Errorf("tool self-correction exhausted after %d rounds: %w", maxToolCorrectionRounds, cause)
				}
				return r.finishWithFailureAnalysis(ctx, modelClient, messages, systemText, sessionID, summary, cause, "tool_correction_exhausted")
			}
			continue
		}
		correctionRounds = 0
	}
	return r.finishWithFailureAnalysis(ctx, modelClient, messages, systemText, sessionID, summary, errors.New("tool calling loop exceeded max iterations"), "tool_loop_exhausted")
}

func (r *langChainAgentRuntime) finishWithFailureAnalysis(
	ctx context.Context,
	modelClient llms.Model,
	messages []llms.MessageContent,
	systemText, sessionID, summary string,
	cause error,
	stage string,
) (*taskv1.TaskResult, error) {
	instruction := failureAnalysisInstruction
	if cause != nil {
		instruction += "\n\n最后一次错误：" + cause.Error()
	}

	analysisMessages := append([]llms.MessageContent{}, messages...)
	analysisMessages = append(analysisMessages, llms.TextParts(llms.ChatMessageTypeHuman, instruction))

	response, err := modelClient.GenerateContent(ctx, analysisMessages)
	output := ""
	if err != nil || response == nil || len(response.Choices) == 0 {
		output = buildFallbackFailureAnalysis(cause, stage)
	} else {
		output = strings.TrimSpace(response.Choices[0].Content)
		if output == "" {
			output = buildFallbackFailureAnalysis(cause, stage)
		}
	}

	analysisMessages = append(analysisMessages, llms.TextParts(llms.ChatMessageTypeAI, output))
	r.persistConversation(ctx, sessionID, analysisMessages)

	msg := stage
	if cause != nil {
		msg = cause.Error()
	}
	r.recordSessionError(ctx, stage, "", msg, output)

	failSummary := "任务未能完成"
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		failSummary = "任务未能完成 · " + trimmed
	}
	return &taskv1.TaskResult{
		Summary: failSummary,
		Output:  output,
	}, nil
}

func buildFallbackFailureAnalysis(cause error, stage string) string {
	var b strings.Builder
	b.WriteString("## 任务未能完成\n\n")
	if cause != nil {
		b.WriteString("**错误原因**：")
		b.WriteString(cause.Error())
		b.WriteString("\n\n")
	}
	b.WriteString("**说明**：代理在多次重试后仍无法自动完成任务。")
	switch stage {
	case "tool_loop_exhausted":
		b.WriteString("已达到单次对话的工具循环上限。")
	case "llm_generate", "llm_empty_response", "llm_empty_output":
		b.WriteString("模型调用未返回有效结果。")
	default:
		b.WriteString("工具调用连续失败。")
	}
	b.WriteString("\n\n**建议**：\n")
	b.WriteString("- 文件路径使用相对于项目根的相对路径，不要使用 `D:\\...` 这类绝对路径\n")
	b.WriteString("- 工具 JSON 参数中的路径请使用正斜杠 `/`（如 `scripts/hello.ps1`）\n")
	b.WriteString("- 简化任务或分步描述需求后重试\n")
	return b.String()
}

func (r *langChainAgentRuntime) publishAgentProgress(ctx context.Context, text string, beforeTools bool) {
	if r == nil || r.trace == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	sessionID := agentctx.TaskID(ctx)
	if sessionID == "" {
		return
	}
	stage := classifyAgentProgressStage(text, beforeTools)
	summary := firstMeaningfulLine(text)
	r.trace.AppendEvent(datatrace.DelegationEvent{
		Time:       time.Now(),
		TaskID:     sessionID,
		Agent:      string(agentctx.Agent(ctx)),
		Stage:      stage,
		Summary:    summary,
		ToolOutput: text,
	})
}

func classifyAgentProgressStage(text string, beforeTools bool) string {
	lower := strings.ToLower(text)
	if strings.Contains(text, "执行计划") || (beforeTools && strings.Contains(text, "步骤")) {
		return "plan_presented"
	}
	if strings.Contains(text, "步骤") && (strings.Contains(text, "完成") || strings.Contains(text, "✓")) {
		return "step_progress"
	}
	if strings.Contains(lower, "plan") && beforeTools {
		return "plan_presented"
	}
	return "agent_progress"
}

func firstMeaningfulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len([]rune(line)) > 96 {
			return string([]rune(line)[:96]) + "…"
		}
		return line
	}
	return ""
}

func (r *langChainAgentRuntime) publishToolFailure(ctx context.Context, toolName string, call llms.ToolCall, err error) {
	if r == nil || r.trace == nil || err == nil {
		return
	}
	sessionID := agentctx.TaskID(ctx)
	if sessionID == "" {
		return
	}
	target := toolCallTarget(call)
	stage := "tool_" + strings.TrimSpace(toolName)
	if stage == "tool_" {
		stage = "tool_error"
	}
	r.trace.AppendEvent(datatrace.DelegationEvent{
		Time:      time.Now(),
		TaskID:    sessionID,
		Agent:     string(agentctx.Agent(ctx)),
		Stage:     stage,
		ToolName:  toolName,
		ToolInput: target,
		Error:     err.Error(),
	})
}

func toolCallTarget(call llms.ToolCall) string {
	if call.FunctionCall == nil {
		return ""
	}
	input, err := normalizeToolInput(call.FunctionCall.Arguments)
	if err != nil {
		return strings.TrimSpace(call.FunctionCall.Arguments)
	}
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "{") {
		var payload struct {
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(input), &payload) == nil && strings.TrimSpace(payload.Path) != "" {
			return strings.TrimSpace(payload.Path)
		}
	}
	return input
}

func (r *langChainAgentRuntime) recordSessionError(ctx context.Context, stage, tool, message, detail string) {
	if r == nil || r.memory == nil {
		return
	}
	sessionID := agentctx.TaskID(ctx)
	if sessionID == "" {
		return
	}
	record := agentmemory.SessionErrorRecord{
		SessionID: sessionID,
		Agent:     string(agentctx.Agent(ctx)),
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
	errText := strings.TrimSpace(err.Error())
	parts := []string{
		"status: error",
		"tool: " + name,
		fmt.Sprintf("attempt: %d/%d", attempt, maxToolCorrectionRounds),
		"error: " + errText,
		"请根据这个错误修正参数、路径或工具选择后重试。",
	}
	if strings.Contains(errText, "absolute paths are not allowed") {
		parts = append(parts, "hint: 路径必须是相对工作区根的相对路径（如 hello.ps1 或 scripts/foo.ps1），不要使用 D:\\... 绝对路径。")
	}
	if strings.Contains(errText, "invalid character") && strings.Contains(errText, "string escape") {
		parts = append(parts, "hint: JSON 参数中的路径请使用正斜杠 /，例如 scripts/hello.ps1，避免反斜杠转义问题。")
	}
	if strings.Contains(errText, "file does not exist") {
		parts = append(parts, "hint: 先 read_file 父目录确认路径，或修正为存在的相对路径。")
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
			out[i] = agentcontext.NormalizeLLMToolCall(tc)
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

func (r *langChainAgentRuntime) publishContextUsage(ctx context.Context, sessionID string, meta agentcontext.PrepareMeta) {
	if r.trace == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	usage := agentcontext.NewUsageSnapshot(meta.Stats)
	var traceCompress *datatrace.ContextCompressResult
	if meta.Compress != nil {
		c := datatrace.ContextCompressResult{
			OriginalChars:   meta.Compress.OriginalChars,
			CompressedChars: meta.Compress.CompressedChars,
			Compressed:      meta.Compress.Compressed,
			OmittedTurns:    meta.Compress.OmittedTurns,
			TruncatedTools:  meta.Compress.TruncatedTools,
		}
		traceCompress = &c
	}
	r.trace.UpdateContextUsage(sessionID, usage, traceCompress)
	if meta.Compress == nil || !meta.Compress.Compressed {
		return
	}
	compressTrace := datatrace.ContextCompressResult{
		OriginalChars:   meta.Compress.OriginalChars,
		CompressedChars: meta.Compress.CompressedChars,
		Compressed:      meta.Compress.Compressed,
		OmittedTurns:    meta.Compress.OmittedTurns,
		TruncatedTools:  meta.Compress.TruncatedTools,
	}
	agent := string(agentctx.Agent(ctx))
	if agent == "" {
		agent = "runtime"
	}
	r.trace.AppendEvent(datatrace.DelegationEvent{
		Time:       time.Now(),
		TaskID:     sessionID,
		Agent:      agent,
		Stage:      "context_compress",
		Summary:    agentcontext.FormatCompressEventSummary(compressTrace),
		ToolOutput: agentcontext.FormatCompressEventOutput(compressTrace, meta.Stats.Threshold),
	})
}

func (r *langChainAgentRuntime) persistConversation(ctx context.Context, sessionID string, messages []llms.MessageContent) {
	if r.memory == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	turns := agentcontext.TurnsFromLLM(messages)
	if len(turns) == 0 {
		return
	}
	conv := &agentmemory.SessionConversation{
		SessionID: sessionID,
		Agent:     string(agentctx.Agent(ctx)),
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

	sessionID := agentctx.TaskID(ctx)
	if r.memory != nil && sessionID != "" {
		_ = r.memory.SaveConversation(ctx, &agentmemory.SessionConversation{
			SessionID: sessionID,
			Agent:     string(agentctx.Agent(ctx)),
			Turns: []agentcontext.ConversationTurn{
				{Role: agentcontext.ConversationRoleHuman, Content: strings.TrimSpace(prompt)},
				{Role: agentcontext.ConversationRoleAI, Content: output},
			},
		})
	}

	return &taskv1.TaskResult{
		Summary: summary,
		Output:  output,
	}, nil
}

func (r *langChainAgentRuntime) dispatchSubTask(ctx context.Context, agent public.AgentKind, prompt string) (*taskv1.TaskResult, error) {
	cmd := &taskv1.TaskCommand{
		Agent:  string(agent),
		Prompt: strings.TrimSpace(prompt),
	}
	if cmd.Prompt == "" {
		return nil, errors.New("sub task prompt is empty")
	}

	if remote := r.lookupRemoteAgent(agent); remote != nil && strings.TrimSpace(remote.GetTarget()) != "" {
		r.log.Infof("router delegating sub task to remote agent=%s target=%s", agent, remote.GetTarget())
		if r.trace != nil {
			r.trace.AppendEvent(datatrace.DelegationEvent{
				Time:          time.Now(),
				TaskID:        agentctx.TaskID(ctx),
				Agent:         string(public.AgentKindRouter),
				Stage:         "delegate_remote",
				Mode:          string(agent),
				Target:        remote.GetTarget(),
				PromptPreview: common.PreviewPrompt(prompt),
			})
		}
		return r.executeRemoteTask(ctx, remote, cmd)
	}

	if r.trace != nil {
		r.trace.AppendEvent(datatrace.DelegationEvent{
			Time:          time.Now(),
			TaskID:        agentctx.TaskID(ctx),
			Agent:         string(public.AgentKindRouter),
			Stage:         "delegate_local",
			Mode:          string(agent),
			PromptPreview: common.PreviewPrompt(prompt),
		})
	}
	return r.ReceiveTask(ctx, cmd)
}

func (r *langChainAgentRuntime) lookupRemoteAgent(agent public.AgentKind) *conf.Runtime_RemoteAgent {
	if r == nil || r.runtimeConfig == nil {
		return nil
	}
	for _, remote := range r.runtimeConfig.GetRemotes() {
		if remote == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(remote.GetAgent()), string(agent)) {
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
			r.trace.AppendEvent(datatrace.DelegationEvent{
				Time:   time.Now(),
				TaskID: agentctx.TaskID(ctx),
				Agent:  string(public.AgentKindRouter),
				Stage:  "remote_execute_failed",
				Target: target,
				Error:  err.Error(),
				Mode:   cmd.GetAgent(),
			})
		}
		return nil, fmt.Errorf("remote execute task failed: %w", err)
	}
	if r.trace != nil {
		r.trace.AppendEvent(datatrace.DelegationEvent{
			Time:       time.Now(),
			TaskID:     agentctx.TaskID(ctx),
			Agent:      string(public.AgentKindRouter),
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
		parts = append(parts, "杈撳嚭锛歕n"+output)
	}
	return strings.Join(parts, "\n")
}

func (r *langChainAgentRuntime) VerifyDelegation(ctx context.Context, taskID string, agent public.AgentKind, prompt string) (*taskv1.TaskResult, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "请验证 router -> " + string(agent) + " 的委派链路"
	}
	if taskID == "" {
		taskID = fmt.Sprintf("verify-%d", time.Now().UnixNano())
	}
	if r.memory != nil {
		if err := r.memory.StartConversation(ctx, taskID, public.AgentKindRouter, prompt); err != nil {
			return nil, fmt.Errorf("start conversation memory: %w", err)
		}
	}
	if r.trace != nil {
		r.trace.StartTask(taskID, public.AgentKindRouter, "running")
		r.trace.AppendEvent(datatrace.DelegationEvent{
			Time:          time.Now(),
			TaskID:        taskID,
			Agent:         string(public.AgentKindRouter),
			Stage:         "verification_start",
			Mode:          string(agent),
			PromptPreview: common.PreviewPrompt(prompt),
		})
	}
	result, err := r.dispatchSubTask(agentctx.WithTaskID(ctx, taskID), agent, prompt)
	if r.trace != nil {
		if err != nil {
			r.trace.AppendEvent(datatrace.DelegationEvent{
				Time:   time.Now(),
				TaskID: taskID,
				Agent:  string(public.AgentKindRouter),
				Stage:  "verification_failed",
				Mode:   string(agent),
				Error:  err.Error(),
			})
			r.trace.UpdateTask(taskID, "failed", nil, err)
		} else {
			r.trace.AppendEvent(datatrace.DelegationEvent{
				Time:       time.Now(),
				TaskID:     taskID,
				Agent:      string(public.AgentKindRouter),
				Stage:      "verification_done",
				Mode:       string(agent),
				Summary:    result.GetSummary(),
				DurationMS: 0,
			})
			r.trace.UpdateTask(taskID, "done", result, nil)
		}
	}
	return result, err
}
