package data

import (
	"context"
	"sync"

	"kratos-demo/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type memoryTaskRepo struct {
	mu    sync.RWMutex
	tasks map[string]*biz.Task
	log   *log.Helper
}

func NewTaskRepo(logger log.Logger) biz.TaskRepo {
	return &memoryTaskRepo{
		tasks: make(map[string]*biz.Task),
		log:   log.NewHelper(logger),
	}
}

func (r *memoryTaskRepo) Save(_ context.Context, task *biz.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[task.ID] = cloneBizTask(task)
	r.log.Infof("save task to memory store: id=%s status=%s", task.ID, task.Status)
	return nil
}

func (r *memoryTaskRepo) Update(_ context.Context, task *biz.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[task.ID]; !ok {
		return biz.ErrTaskNotFound
	}
	r.tasks[task.ID] = cloneBizTask(task)
	r.log.Infof("update task in memory store: id=%s status=%s", task.ID, task.Status)
	return nil
}

func (r *memoryTaskRepo) Get(_ context.Context, id string) (*biz.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[id]
	if !ok {
		return nil, biz.ErrTaskNotFound
	}
	return cloneBizTask(task), nil
}

func cloneBizTask(task *biz.Task) *biz.Task {
	if task == nil {
		return nil
	}
	cloned := *task
	if task.Result != nil {
		result := *task.Result
		cloned.Result = &result
	}
	return &cloned
}
