package biz

import (
	"context"
	"testing"

	taskv1 "kratos-demo/api/task/v1"
)

type stubTaskRepo struct {
	saved *Task
}

func (r *stubTaskRepo) Save(_ context.Context, task *Task) error {
	r.saved = cloneTask(task)
	return nil
}

func (r *stubTaskRepo) Update(_ context.Context, _ *Task) error {
	return nil
}

func (r *stubTaskRepo) Get(_ context.Context, _ string) (*Task, error) {
	return cloneTask(r.saved), nil
}

func TestNormalizeAgent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  TaskAgent
	}{
		{name: "blank defaults to default profile", input: "", want: TaskAgentDefault},
		{name: "whitespace defaults to default profile", input: "   ", want: TaskAgentDefault},
		{name: "default stays default", input: "default", want: TaskAgentDefault},
		{name: "generic aliases to default profile", input: "generic", want: TaskAgentDefault},
		{name: "router remains compatible", input: "RoUtEr", want: TaskAgentRouter},
		{name: "coder remains compatible", input: " coder ", want: TaskAgentCoder},
		{name: "reviewer remains compatible", input: "reviewer", want: TaskAgentReviewer},
		{name: "unknown value preserved", input: "planner", want: TaskAgent("planner")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAgent(tt.input); got != tt.want {
				t.Fatalf("normalizeAgent(%q) = %q, want %q", tt.input, got, tt.want)
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

	if task.Agent != TaskAgentDefault {
		t.Fatalf("created task agent = %q, want %q", task.Agent, TaskAgentDefault)
	}
	if repo.saved == nil || repo.saved.Agent != TaskAgentDefault {
		t.Fatalf("saved task agent = %v, want %q", repo.saved, TaskAgentDefault)
	}
	if dispatcher.last == nil || dispatcher.last.Agent != TaskAgentDefault.String() {
		t.Fatalf("dispatched agent = %v, want %q", dispatcher.last, TaskAgentDefault.String())
	}
}

type captureDispatcher struct {
	last *taskv1.TaskCommand
}

func (d *captureDispatcher) Dispatch(_ context.Context, cmd *taskv1.TaskCommand) error {
	d.last = cmd
	return nil
}
