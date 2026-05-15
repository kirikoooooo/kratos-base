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

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultOpenAIModel   = "gpt-4o-mini"
	defaultRemoteTimeout = 8 * time.Second
	runtimeActorPID      = 9001
)

type langChainAgentRuntime struct {
	config        *conf.AI
	runtimeConfig *conf.Runtime
	log           *log.Helper
	pid           actorpkg.PID
}

func NewAgentRuntime(config *conf.AI, runtimeConfig *conf.Runtime, logger log.Logger) biz.AgentRuntime {
	return &langChainAgentRuntime{
		config:        config,
		runtimeConfig: runtimeConfig,
		log:           log.NewHelper(logger),
		pid:           actorpkg.NewPID(runtimeActorPID, "langchain-runtime"),
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
		return r.runCoder(prompt), nil
	case biz.TaskAgentReviewer:
		return r.runReviewer(prompt), nil
	default:
		return nil, fmt.Errorf("%w: %s", biz.ErrAgentNotSupported, agent)
	}
}

func (r *langChainAgentRuntime) ReceiveTask(ctx context.Context, cmd *taskv1.TaskCommand) (*taskv1.TaskResult, error) {
	if cmd == nil {
		return nil, errors.New("task command is nil")
	}
	return r.Execute(ctx, biz.TaskAgent(cmd.Agent), cmd.Prompt)
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
	apiKey := strings.TrimSpace(r.config.GetOpenai().GetApiKey())
	if apiKey == "" {
		err := errors.New("ai.openai.api_key is empty")
		r.log.Warn(err.Error())
		return nil, err
	}

	model := strings.TrimSpace(r.config.GetOpenai().GetModel())
	if model == "" {
		model = defaultOpenAIModel
	}

	options := []openai.Option{
		openai.WithToken(apiKey),
		openai.WithModel(model),
	}
	if baseURL := strings.TrimSpace(r.config.GetOpenai().GetBaseUrl()); baseURL != "" {
		options = append(options, openai.WithBaseURL(baseURL))
	}

	llm, err := openai.New(options...)
	if err != nil {
		r.log.Warnf("create openai compatible llm failed: %v", err)
		return nil, fmt.Errorf("create openai compatible llm failed: %w", err)
	}

	agentTools := []tools.Tool{
		newLocalAgentTool("coder_agent", "适合处理编码、实现、原型设计、接口定义等任务。输入应为要交给 coder 的具体任务描述。", func(toolCtx context.Context, input string) (string, error) {
			result, err := r.dispatchSubTask(toolCtx, biz.TaskAgentCoder, input)
			if err != nil {
				return "", err
			}
			return formatTaskResult("CoderAgent", result), nil
		}),
		newLocalAgentTool("reviewer_agent", "适合处理代码审查、设计评审、风险识别、边界条件检查等任务。输入应为待审查内容或审查目标。", func(toolCtx context.Context, input string) (string, error) {
			result, err := r.dispatchSubTask(toolCtx, biz.TaskAgentReviewer, input)
			if err != nil {
				return "", err
			}
			return formatTaskResult("ReviewerAgent", result), nil
		}),
	}

	systemPrompt := strings.Join([]string{
		"你是 RouterAgent，负责协调本地 Agent 完成任务。",
		"请优先使用可用函数来完成编码、实现、审查等子任务，而不是直接假设工具执行结果。",
		"当任务同时包含开发与审查时，优先调用 coder_agent，再根据结果调用 reviewer_agent。",
		"当你已经拿到足够的工具结果后，再直接输出最终中文结论。",
	}, "\n")

	agent := agents.NewOpenAIFunctionsAgent(
		llm,
		agentTools,
		agents.NewOpenAIOption().WithSystemMessage(systemPrompt),
	)
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
		Summary: "router agent 已通过 LangChainGo function calling 完成编排",
		Output:  output,
	}, nil
}

func (r *langChainAgentRuntime) runCoder(prompt string) *taskv1.TaskResult {
	return &taskv1.TaskResult{
		Summary: "coder agent 已生成初始方案",
		Output: strings.Join([]string{
			"CoderAgent 已准备开始处理任务。",
			"任务内容：" + prompt,
			"建议先完成最小可运行版本，再逐步增加复杂能力。",
		}, "\n"),
	}
}

func (r *langChainAgentRuntime) runReviewer(prompt string) *taskv1.TaskResult {
	return &taskv1.TaskResult{
		Summary: "reviewer agent 已完成审查",
		Output: strings.Join([]string{
			"ReviewerAgent 已准备开始审查任务。",
			"审查对象：" + prompt,
			"建议关注边界条件、错误处理和后续可扩展性。",
		}, "\n"),
	}
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
		return r.executeRemoteTask(ctx, remote, cmd)
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
	result, err := client.ExecuteTask(callCtx, cmd)
	if err != nil {
		return nil, fmt.Errorf("remote execute task failed: %w", err)
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

type localAgentTool struct {
	name        string
	description string
	run         func(ctx context.Context, input string) (string, error)
}

func newLocalAgentTool(name, description string, run func(ctx context.Context, input string) (string, error)) tools.Tool {
	return &localAgentTool{
		name:        name,
		description: description,
		run:         run,
	}
}

func (t *localAgentTool) Name() string {
	return t.name
}

func (t *localAgentTool) Description() string {
	return t.description
}

func (t *localAgentTool) Call(ctx context.Context, input string) (string, error) {
	return t.run(ctx, strings.TrimSpace(input))
}
