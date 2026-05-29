package tasking

import (
	"context"
	"sync"


	"github.com/go-kratos/kratos/v2/log"
)

type memoryTaskRepo struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	log   *log.Helper
}

func NewTaskRepo(logger log.Logger) TaskRepo {
	return &memoryTaskRepo{
		tasks: make(map[string]*Task),
		log:   log.NewHelper(logger),
	}
}

func (r *memoryTaskRepo) Save(_ context.Context, task *Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[task.ID] = cloneBizTask(task)
	r.log.Infof("save task to memory store: id=%s status=%s", task.ID, task.Status)
	return nil
}

func (r *memoryTaskRepo) Update(_ context.Context, task *Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[task.ID]; !ok {
		return ErrTaskNotFound
	}
	r.tasks[task.ID] = cloneBizTask(task)
	r.log.Infof("update task in memory store: id=%s status=%s", task.ID, task.Status)
	return nil
}

func (r *memoryTaskRepo) Get(_ context.Context, id string) (*Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[id]
	if !ok {
		return nil, ErrTaskNotFound
	}
	return cloneBizTask(task), nil
}

func cloneBizTask(task *Task) *Task {
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
