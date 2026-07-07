package tasking

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"kratos-demo/internal/biz"
)

type TaskUsecase struct {
	repo       TaskRepo
	dispatcher TaskDispatcher
	seq        atomic.Uint64
}

func NewTaskUsecase(repo TaskRepo, dispatcher TaskDispatcher) *TaskUsecase {
	return &TaskUsecase{
		repo:       repo,
		dispatcher: dispatcher,
	}
}

func (uc *TaskUsecase) Create(ctx context.Context, agent, prompt string) (*Task, error) {
	normalizedAgent := NormalizeAgent(agent)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, ErrPromptRequired
	}

	now := time.Now()
	task := &Task{
		ID:        fmt.Sprintf("task-%d", uc.seq.Add(1)),
		Agent:     normalizedAgent,
		Prompt:    prompt,
		Status:    TaskStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := uc.repo.Save(ctx, task); err != nil {
		return nil, err
	}

	if uc.dispatcher == nil {
		return nil, ErrTaskDispatcherUnavailable
	}

	if err := uc.dispatcher.Dispatch(ctx, task); err != nil {
		return nil, err
	}

	copyTask := *task
	if task.Result != nil {
		result := *task.Result
		copyTask.Result = &result
	}
	return &copyTask, nil
}

func (uc *TaskUsecase) Get(ctx context.Context, id string) (*Task, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrTaskNotFound
	}
	return uc.repo.Get(ctx, id)
}

func NormalizeAgent(agent string) biz.AgentKind {
	agent = strings.TrimSpace(strings.ToLower(agent))
	if agent == "" {
		return biz.AgentKindDefault
	}
	switch biz.AgentKind(agent) {
	case biz.AgentKindDefault, biz.AgentKindGeneric:
		return biz.AgentKindDefault
	}
	return biz.AgentKind(agent)
}
