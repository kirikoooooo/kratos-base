package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	actorpkg "kratos-demo/third_party/actor"
	toolcatalog "kratos-demo/third_party/tools"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	langtools "github.com/tmc/langchaingo/tools"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultOpenAIModel   = "gpt-4o-mini"
	defaultRemoteTimeout = 8 * time.Second
	runtimeActorPID      = 9001
)

var newRuntimeLLM = func(config *conf.AI, logger *log.Helper) (llms.Model, error) {
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

	options := []openai.Option{
		openai.WithToken(apiKey),
		openai.WithModel(model),
	}
	if baseURL := strings.TrimSpace(config.GetOpenai().GetBaseUrl()); baseURL != "" {
		options = append(options, openai.WithBaseURL(baseURL))
	}

	modelClient, err := openai.New(options...)
	if err != nil {
		logger.Warnf("create openai compatible llm failed: %v", err)
		return nil, fmt.Errorf("create openai compatible llm failed: %w", err)
	}
	return modelClient, nil
}

type langChainAgentRuntime struct {
	config         *conf.AI
	runtimeConfig  *conf.Runtime
	log            *log.Helper
	pid            actorpkg.PID
	trace          biz.DelegationTraceStore
	sandbox        sandboxExecutor
	toolCatalog    *toolcatalog.Catalog
	toolCatalogErr error
}

func NewAgentRuntime(config *conf.AI, runtimeConfig *conf.Runtime, trace biz.DelegationTraceStore, logger log.Logger) biz.AgentRuntime {
	helper := log.NewHelper(logger)
	catalog, err := toolcatalog.DefaultCatalog()
	if err != nil {
		helper.Warnf("load embedded tool catalog failed: %v", err)
	}
	return &langChainAgentRuntime{
		config:         config,
		runtimeConfig:  runtimeConfig,
		log:            helper,
		pid:            actorpkg.NewPID(runtimeActorPID, "langchain-runtime"),
		trace:          trace,
		sandbox:        newSandboxExecutor(runtimeConfig, helper),
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
	case biz.TaskAgentRouter, biz.TaskAgentCoder, biz.TaskAgentReviewer:
		return true
	default:
		return false
	}
}

func (r *langChainAgentRuntime) Execute(ctx context.Context, agent biz.TaskAgent, prompt string) (*taskv1.TaskResult, error) {
	switch agent {
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
	return r.Execute(withTaskID(ctx, cmd.TaskID), biz.TaskAgent(cmd.Agent), cmd.Prompt)
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
	modelClient, err := r.newLLM()
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
	agentTools, err := r.buildManagedTools(r.appendSandboxBindings(bindings))
	if err != nil {
		return nil, err
	}

	promptLines := []string{
		"你是 RouterAgent，负责协调本地 Agent 完成任务。",
		"请优先使用可用函数来完成编码、实现、审查等子任务，而不是直接假设工具执行结果。",
		"当任务同时包含开发与审查时，优先调用 coder_agent，再根据结果调用 reviewer_agent。",
		"当你已经拿到足够的工具结果后，再直接输出最终中文结论。",
	}
	if r.hasSandboxTools() {
		promptLines = append(promptLines, "如需要在 E2B Code Sandbox 中执行系统命令或 skills，请调用 sandbox_exec 或 sandbox_skill，并基于返回结果继续决策。")
	}
	systemPrompt := strings.Join(promptLines, "\n")

	agent := agents.NewOpenAIFunctionsAgent(
		modelClient,
		agentTools,
		agents.NewOpenAIOption().WithSystemMessage(systemPrompt),
	)
	return r.runFunctionsAgent(ctx, agent, prompt, "router agent 已通过 LangChainGo function calling 完成编排")
}

func (r *langChainAgentRuntime) runCoder(ctx context.Context, prompt string) (*taskv1.TaskResult, error) {
	modelClient, err := r.newLLM()
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
	agentTools, err := r.buildManagedTools(r.appendSandboxBindings(bindings))
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
	if r.hasSandboxTools() {
		promptLines = append(promptLines, "如需要进入 E2B Code Sandbox 执行系统命令、验证实现、运行构建或调用 skills，请使用 sandbox_exec 或 sandbox_skill。")
	}
	systemPrompt := strings.Join(promptLines, "\n")

	agent := agents.NewOpenAIFunctionsAgent(
		modelClient,
		agentTools,
		agents.NewOpenAIOption().WithSystemMessage(systemPrompt),
	)
	return r.runFunctionsAgent(ctx, agent, prompt, "coder agent 已通过 LangChainGo function calling 完成生成")
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

func (r *langChainAgentRuntime) buildManagedTools(bindings []toolcatalog.BindingSpec) ([]langtools.Tool, error) {
	if r == nil {
		return nil, errors.New("agent runtime is nil")
	}
	if r.toolCatalogErr != nil {
		return nil, fmt.Errorf("tool catalog unavailable: %w", r.toolCatalogErr)
	}
	if r.toolCatalog == nil {
		return nil, errors.New("tool catalog is nil")
	}
	return r.toolCatalog.BindLangChainTools(bindings)
}

func (r *langChainAgentRuntime) hasSandboxTools() bool {
	return r != nil && r.sandbox != nil && r.sandbox.Enabled()
}

func (r *langChainAgentRuntime) appendSandboxBindings(bindings []toolcatalog.BindingSpec) []toolcatalog.BindingSpec {
	if !r.hasSandboxTools() {
		return bindings
	}
	return append(bindings,
		toolcatalog.BindingSpec{Name: "sandbox_exec", Handler: func(toolCtx context.Context, input string) (string, error) {
			return r.sandbox.ExecuteCommand(toolCtx, input)
		}},
		toolcatalog.BindingSpec{Name: "sandbox_skill", Handler: func(toolCtx context.Context, input string) (string, error) {
			skillName, args, err := parseSkillInvocation(input)
			if err != nil {
				return "", err
			}
			return r.sandbox.ExecuteSkill(toolCtx, skillName, args)
		}},
	)
}

func (r *langChainAgentRuntime) runFunctionsAgent(ctx context.Context, agent agents.Agent, prompt, summary string) (*taskv1.TaskResult, error) {
	executor := agents.NewExecutor(agent, agents.WithMaxIterations(4))

	values, err := executor.Call(ctx, map[string]any{"input": prompt})
	if err != nil {
		r.log.Warnf("function calling executor call failed: %v", err)
		return nil, fmt.Errorf("function calling executor call failed: %w", err)
	}

	output, _ := values["output"].(string)
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, errors.New("react executor returned empty output")
	}

	return &taskv1.TaskResult{
		Summary: summary,
		Output:  output,
	}, nil
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
	if r.trace != nil {
		r.trace.StartTask(taskID, biz.TaskAgentRouter, prompt, biz.TaskStatusRunning)
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
