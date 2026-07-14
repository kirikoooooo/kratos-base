package tasking

import (
	"context"
	"kratos-demo/internal/consts/public"
	"testing"
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
		want  public.AgentKind
	}{
		{name: "blank defaults to default profile", input: "", want: public.AgentKindDefault},
		{name: "whitespace defaults to default profile", input: "   ", want: public.AgentKindDefault},
		{name: "default stays default", input: "default", want: public.AgentKindDefault},
		{name: "generic aliases to default profile", input: "generic", want: public.AgentKindDefault},
		{name: "router remains compatible", input: "RoUtEr", want: public.AgentKindRouter},
		{name: "coder remains compatible", input: " coder ", want: public.AgentKindCoder},
		{name: "reviewer remains compatible", input: "reviewer", want: public.AgentKindReviewer},
		{name: "unknown value preserved", input: "planner", want: public.AgentKind("planner")},
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

	if task.Agent != public.AgentKindDefault {
		t.Fatalf("created task agent = %q, want %q", task.Agent, public.AgentKindDefault)
	}
	if repo.saved == nil || repo.saved.Agent != public.AgentKindDefault {
		t.Fatalf("saved task agent = %v, want %q", repo.saved, public.AgentKindDefault)
	}
	if dispatcher.last == nil || dispatcher.last.Agent != public.AgentKindDefault {
		t.Fatalf("dispatched agent = %v, want %q", dispatcher.last, public.AgentKindDefault)
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
