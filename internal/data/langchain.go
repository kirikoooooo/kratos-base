package data

import (
	"context"
	"errors"
	"fmt"
	"strings"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	actorpkg "kratos-demo/third_party/actor"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"
)

const (
	defaultOpenAIModel = "gpt-4o-mini"
	runtimeActorPID    = 9001
)

type langChainAgentRuntime struct {
	config *conf.AI
	log    *log.Helper
	pid    actorpkg.PID
}

func NewAgentRuntime(config *conf.AI, logger log.Logger) biz.AgentRuntime {
	return &langChainAgentRuntime{
		config: config,
		log:    log.NewHelper(logger),
		pid:    actorpkg.NewPID(runtimeActorPID, "langchain-runtime"),
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
		newLocalAgentTool("coder_agent", "适合处理编码、实现、原型设计、接口定义等任务。输入应为要交给 coder 的具体任务描述。", func(input string) string {
			return strings.Join([]string{
				"CoderAgent 已准备开始处理任务。",
				"CoderAgent 已收到任务：" + input,
				"建议输出最小可运行版本，优先保证主链路可验证，再补充扩展能力。",
			}, "\n")
		}),
		newLocalAgentTool("reviewer_agent", "适合处理代码审查、设计评审、风险识别、边界条件检查等任务。输入应为待审查内容或审查目标。", func(input string) string {
			return strings.Join([]string{
				"ReviewerAgent 已准备开始审查任务。",
				"ReviewerAgent 已收到审查任务：" + input,
				"建议重点检查错误处理、模块边界、可扩展性与后续演进风险。",
			}, "\n")
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

type localAgentTool struct {
	name        string
	description string
	run         func(input string) string
}

func newLocalAgentTool(name, description string, run func(input string) string) tools.Tool {
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

func (t *localAgentTool) Call(_ context.Context, input string) (string, error) {
	return t.run(strings.TrimSpace(input)), nil
}
