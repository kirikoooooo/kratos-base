package tasking

import (
	"context"
	"testing"

	"kratos-demo/internal/biz"
)

type stubTaskRepo struct {
	saved *Task
}

func (r *stubTaskRepo) Save(_ context.Context, task *Task) error {
	if task == nil {
		r.saved = nil
		return nil
	}
	copyTask := *task
	if task.Result != nil {
		result := *task.Result
		copyTask.Result = &result
	}
	r.saved = &copyTask
	return nil
}

func (r *stubTaskRepo) Update(_ context.Context, _ *Task) error {
	return nil
}

func (r *stubTaskRepo) Get(_ context.Context, _ string) (*Task, error) {
	if r.saved == nil {
		return nil, nil
	}
	copyTask := *r.saved
	if r.saved.Result != nil {
		result := *r.saved.Result
		copyTask.Result = &result
	}
	return &copyTask, nil
}

func TestNormalizeAgent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  biz.Agent
	}{
		{name: "blank defaults to default profile", input: "", want: biz.AgentDefault},
		{name: "whitespace defaults to default profile", input: "   ", want: biz.AgentDefault},
		{name: "default stays default", input: "default", want: biz.AgentDefault},
		{name: "generic aliases to default profile", input: "generic", want: biz.AgentDefault},
		{name: "router remains compatible", input: "RoUtEr", want: biz.AgentRouter},
		{name: "coder remains compatible", input: " coder ", want: biz.AgentCoder},
		{name: "reviewer remains compatible", input: "reviewer", want: biz.AgentReviewer},
		{name: "unknown value preserved", input: "planner", want: biz.Agent("planner")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeAgent(tt.input); got != tt.want {
				t.Fatalf("NormalizeAgent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTaskUsecaseCreateUsesDefaultAgentProfile(t *testing.T) {
	repo := &stubTaskRepo{}
	dispatcher := &captureDispatcher{}
	uc := NewTaskUsecase(repo, dispatcher)

	task, err := uc.Create(context.Background(), "", "implement generic agent runtime")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if task.Agent != biz.AgentDefault {
		t.Fatalf("created task agent = %q, want %q", task.Agent, biz.AgentDefault)
	}
	if repo.saved == nil || repo.saved.Agent != biz.AgentDefault {
		t.Fatalf("saved task agent = %v, want %q", repo.saved, biz.AgentDefault)
	}
	if dispatcher.last == nil || dispatcher.last.Agent != biz.AgentDefault {
		t.Fatalf("dispatched agent = %v, want %q", dispatcher.last, biz.AgentDefault)
	}
}

type captureDispatcher struct {
	last *Task
}

func (d *captureDispatcher) Dispatch(_ context.Context, task *Task) error {
	if task == nil {
		d.last = nil
		return nil
	}
	copyTask := *task
	if task.Result != nil {
		result := *task.Result
		copyTask.Result = &result
	}
	d.last = &copyTask
	return nil
}
