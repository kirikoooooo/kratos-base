package data

import (
	"context"
	"fmt"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	actorpkg "kratos-demo/third_party/actor"

	"github.com/go-kratos/kratos/v2/log"
)

const runtimeMailboxSize = 128

type taskDispatcher struct {
	repo    biz.TaskRepo
	runtime biz.AgentRuntime
	log     *log.Helper
}

func NewTaskDispatcher(repo biz.TaskRepo, runtime biz.AgentRuntime, logger log.Logger) biz.TaskDispatcher {
	if runtime != nil {
		actorpkg.StopActor(runtime.PID())
		if err := actorpkg.RegisterActor(runtime, runtimeMailboxSize); err != nil {
			panic(fmt.Errorf("register agent runtime %s failed: %w", runtime.Name(), err))
		}
	}

	return &taskDispatcher{
		repo:    repo,
		runtime: runtime,
		log:     log.NewHelper(logger),
	}
}

func (d *taskDispatcher) Dispatch(ctx context.Context, cmd *taskv1.TaskCommand) error {
	if cmd == nil {
		return biz.ErrTaskNotFound
	}
	if d.runtime == nil {
		return fmt.Errorf("%w: runtime is nil", biz.ErrAgentNotSupported)
	}
	if !d.runtime.Supports(biz.TaskAgent(cmd.Agent)) {
		return fmt.Errorf("%w: %s", biz.ErrAgentNotSupported, cmd.Agent)
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

	task.MarkRunning(time.Now())
	if err := d.repo.Update(ctx, task); err != nil {
		d.log.Errorf("update task running failed: id=%s err=%v", cmd.TaskID, err)
		return
	}

	result, execErr := d.runtime.SendTask(ctx, cmd)
	updated, err := d.repo.Get(ctx, cmd.TaskID)
	if err != nil {
		d.log.Errorf("reload task failed: id=%s err=%v", cmd.TaskID, err)
		return
	}

	if execErr != nil {
		updated.MarkFailed(execErr, time.Now())
		if err := d.repo.Update(ctx, updated); err != nil {
			d.log.Errorf("update task failed status failed: id=%s err=%v", cmd.TaskID, err)
		}
		return
	}

	updated.MarkDone(result, time.Now())
	if err := d.repo.Update(ctx, updated); err != nil {
		d.log.Errorf("update task done failed: id=%s err=%v", cmd.TaskID, err)
	}
}
