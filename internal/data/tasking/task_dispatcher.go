package tasking

import (
	"context"
	"fmt"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	actorpkg "kratos-demo/third_party/actor"

	"github.com/go-kratos/kratos/v2/log"
	dataagent "kratos-demo/internal/data/agent_runtime"
	datatrace "kratos-demo/internal/data/trace"
)

const runtimeMailboxSize = 128

type taskDispatcher struct {
	repo    TaskRepo
	runtime biz.AgentRuntime
	trace   datatrace.DelegationTraceStore
	memory  dataagent.AgentMemory
	log     *log.Helper
}

func NewTaskDispatcher(repo TaskRepo, runtime biz.AgentRuntime, trace datatrace.DelegationTraceStore, memory dataagent.AgentMemory, logger log.Logger) TaskDispatcher {
	if runtime != nil {
		actorpkg.StopActor(runtime.PID())
		if err := actorpkg.RegisterActor(runtime, runtimeMailboxSize); err != nil {
			panic(fmt.Errorf("register agent runtime %s failed: %w", runtime.Name(), err))
		}
	}

	return &taskDispatcher{
		repo:    repo,
		runtime: runtime,
		trace:   trace,
		memory:  memory,
		log:     log.NewHelper(logger),
	}
}

func (d *taskDispatcher) Dispatch(ctx context.Context, task *Task) error {
	if task == nil {
		return ErrTaskNotFound
	}
	cmd := taskToCommand(task)
	if d.runtime == nil {
		return fmt.Errorf("%w: runtime is nil", dataagent.ErrAgentNotSupported)
	}
	if !d.runtime.Supports(biz.AgentKind(cmd.Agent)) {
		return fmt.Errorf("%w: %s", dataagent.ErrAgentNotSupported, cmd.Agent)
	}

	go d.handle(context.WithoutCancel(ctx), cmd)
	return nil
}

func (d *taskDispatcher) handle(ctx context.Context, cmd *taskv1.TaskCommand) {
	if cmd == nil {
		return
	}

	task, err := d.repo.Get(ctx, cmd.TaskID)
	if err != nil {
		d.log.Errorf("load task failed: id=%s err=%v", cmd.TaskID, err)
		return
	}

	markTaskRunning(task, time.Now())
	if err := d.repo.Update(ctx, task); err != nil {
		d.log.Errorf("update task running failed: id=%s err=%v", cmd.TaskID, err)
		return
	}
	if d.memory != nil {
		if err := d.memory.StartConversation(ctx, task.ID, task.Agent, task.Prompt); err != nil {
			d.log.Warnf("start conversation memory failed: id=%s err=%v", task.ID, err)
		}
	}
	if d.trace != nil {
		d.trace.StartTask(task.ID, task.Agent, string(task.Status))
		d.trace.UpdatePlan(task.ID, InitialPlan(task.Agent, task.Prompt))
		d.trace.AppendEvent(datatrace.DelegationEvent{
			Time:          time.Now(),
			TaskID:        task.ID,
			Agent:         string(task.Agent),
			Stage:         "task_running",
			PromptPreview: previewPrompt(task.Prompt),
		})
	}

	result, execErr := d.runtime.SendTask(ctx, cmd)
	updated, err := d.repo.Get(ctx, cmd.TaskID)
	if err != nil {
		d.log.Errorf("reload task failed: id=%s err=%v", cmd.TaskID, err)
		return
	}

	if execErr != nil {
		markTaskFailed(updated, execErr, time.Now())
		if err := d.repo.Update(ctx, updated); err != nil {
			d.log.Errorf("update task failed status failed: id=%s err=%v", cmd.TaskID, err)
		}
		if d.memory != nil {
			_ = d.memory.RecordSessionError(ctx, dataagent.SessionErrorRecord{
				SessionID: updated.ID,
				Agent:     string(updated.Agent),
				Stage:     "task_failed",
				Message:   execErr.Error(),
			})
		}
		if d.trace != nil {
			d.trace.AppendEvent(datatrace.DelegationEvent{
				Time:    time.Now(),
				TaskID:  updated.ID,
				Agent:   string(updated.Agent),
				Stage:   "task_failed",
				Error:   execErr.Error(),
				Summary: "task execution failed",
			})
			d.trace.UpdateTask(updated.ID, string(updated.Status), nil, execErr)
		}
		return
	}

	markTaskDone(updated, result, time.Now())
	if err := d.repo.Update(ctx, updated); err != nil {
		d.log.Errorf("update task done failed: id=%s err=%v", cmd.TaskID, err)
	}
	if d.trace != nil {
		d.trace.AppendEvent(datatrace.DelegationEvent{
			Time:       time.Now(),
			TaskID:     updated.ID,
			Agent:      string(updated.Agent),
			Stage:      "task_done",
			Summary:    result.GetSummary(),
			DurationMS: updated.UpdatedAt.Sub(task.CreatedAt).Milliseconds(),
		})
		d.trace.UpdateTask(updated.ID, string(updated.Status), result, nil)
	}
}
