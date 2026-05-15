package data

import (
	"context"
	"fmt"
	"time"

	"kratos-demo/internal/biz"
	actorpkg "kratos-demo/third_party/actor"

	"github.com/go-kratos/kratos/v2/log"
)

const (
	taskActorMailboxSize = 128
	taskCommandMessageID = 1001
	routerAgentPID       = 1
	coderAgentPID        = 2
	reviewerAgentPID     = 3
)

type taskDispatcher struct {
	agents map[biz.TaskAgent]*taskAgent
}

type taskAgent struct {
	pid     actorpkg.PID
	agent   biz.TaskAgent
	repo    biz.TaskRepo
	runtime biz.AgentRuntime
	log     *log.Helper
}

func NewTaskDispatcher(repo biz.TaskRepo, runtime biz.AgentRuntime, logger log.Logger) biz.TaskDispatcher {
	helper := log.NewHelper(logger)
	dispatcher := &taskDispatcher{
		agents: make(map[biz.TaskAgent]*taskAgent),
	}

	for _, agent := range supportedAgents(runtime) {
		worker := newTaskAgent(agent, repo, runtime, helper)
		actorpkg.StopActor(worker.PID())
		if err := actorpkg.RegisterActor(worker, taskActorMailboxSize); err != nil {
			panic(fmt.Errorf("register task agent %s failed: %w", agent, err))
		}
		dispatcher.agents[agent] = worker
	}

	return dispatcher
}

func supportedAgents(runtime biz.AgentRuntime) []biz.TaskAgent {
	if runtime == nil {
		return nil
	}

	candidates := []biz.TaskAgent{
		biz.TaskAgentRouter,
		biz.TaskAgentCoder,
		biz.TaskAgentReviewer,
	}
	result := make([]biz.TaskAgent, 0, len(candidates))
	for _, agent := range candidates {
		if runtime.Supports(agent) {
			result = append(result, agent)
		}
	}
	return result
}

func newTaskAgent(agent biz.TaskAgent, repo biz.TaskRepo, runtime biz.AgentRuntime, helper *log.Helper) *taskAgent {
	return &taskAgent{
		pid:     actorpkg.NewPID(agentPID(agent), "task-"+agent.String()),
		agent:   agent,
		repo:    repo,
		runtime: runtime,
		log:     helper,
	}
}

func agentPID(agent biz.TaskAgent) uint64 {
	switch agent {
	case biz.TaskAgentRouter:
		return routerAgentPID
	case biz.TaskAgentCoder:
		return coderAgentPID
	case biz.TaskAgentReviewer:
		return reviewerAgentPID
	default:
		return 1000
	}
}

func (d *taskDispatcher) Dispatch(_ context.Context, cmd *biz.TaskCommand) error {
	if cmd == nil {
		return biz.ErrTaskNotFound
	}

	worker, ok := d.agents[cmd.Agent]
	if !ok {
		return fmt.Errorf("%w: %s", biz.ErrAgentNotSupported, cmd.Agent)
	}

	return actorpkg.Send(nil, worker.PID(), &actorpkg.Message{
		Id:   taskCommandMessageID,
		Data: cmd,
	})
}

func (a *taskAgent) PID() actorpkg.PID {
	return a.pid
}

func (a *taskAgent) Process(msg *actorpkg.Message) {
	if msg == nil {
		return
	}

	cmd, ok := msg.Data.(*biz.TaskCommand)
	if !ok || cmd == nil {
		a.log.Warnf("task agent %s received invalid message: %#v", a.agent, msg.Data)
		return
	}

	a.handle(context.Background(), cmd)
}

func (a *taskAgent) OnStop() {
	a.log.Infof("task agent stopped: %s", a.agent)
}

func (a *taskAgent) handle(ctx context.Context, cmd *biz.TaskCommand) {
	if cmd == nil {
		return
	}

	task, err := a.repo.Get(ctx, cmd.TaskID)
	if err != nil {
		a.log.Errorf("load task failed: id=%s err=%v", cmd.TaskID, err)
		return
	}

	task.MarkRunning(time.Now())
	if err := a.repo.Update(ctx, task); err != nil {
		a.log.Errorf("update task running failed: id=%s err=%v", cmd.TaskID, err)
		return
	}

	result, execErr := a.runtime.Execute(ctx, cmd.Agent, cmd.Prompt)
	updated, err := a.repo.Get(ctx, cmd.TaskID)
	if err != nil {
		a.log.Errorf("reload task failed: id=%s err=%v", cmd.TaskID, err)
		return
	}

	if execErr != nil {
		updated.MarkFailed(execErr, time.Now())
		if err := a.repo.Update(ctx, updated); err != nil {
			a.log.Errorf("update task failed status failed: id=%s err=%v", cmd.TaskID, err)
		}
		return
	}

	updated.MarkDone(result, time.Now())
	if err := a.repo.Update(ctx, updated); err != nil {
		a.log.Errorf("update task done failed: id=%s err=%v", cmd.TaskID, err)
	}
}
