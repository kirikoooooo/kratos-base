package trace

import (
	"context"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/consts/public"
)

type mirroredTraceStore struct {
	DelegationTraceStore
	observer DelegationTraceObserver
}

func NewMirroredTraceStore(store DelegationTraceStore, observer DelegationTraceObserver) DelegationTraceStore {
	if store == nil || observer == nil {
		return store
	}
	return &mirroredTraceStore{DelegationTraceStore: store, observer: observer}
}

func (s *mirroredTraceStore) StartTask(taskID string, agent public.AgentKind, status string) {
	s.DelegationTraceStore.StartTask(taskID, agent, status)
	s.observer.StartTask(taskID, agent, status)
}

func (s *mirroredTraceStore) UpdateTask(taskID, status string, result *taskv1.TaskResult, err error) {
	s.DelegationTraceStore.UpdateTask(taskID, status, result, err)
	s.observer.UpdateTask(taskID, status, result, err)
}

func (s *mirroredTraceStore) SetTaskMetadata(taskID string, metadata map[string]any) {
	s.DelegationTraceStore.SetTaskMetadata(taskID, metadata)
	s.observer.SetTaskMetadata(taskID, metadata)
}

func (s *mirroredTraceStore) AppendEvent(event DelegationEvent) {
	s.DelegationTraceStore.AppendEvent(event)
	s.observer.AppendEvent(event)
}

func (s *mirroredTraceStore) UpdatePlan(taskID string, steps []PlanStep) {
	s.DelegationTraceStore.UpdatePlan(taskID, steps)
	s.observer.UpdatePlan(taskID, steps)
}

func (s *mirroredTraceStore) UpdateContextUsage(taskID string, usage ContextUsageSnapshot, compress *ContextCompressResult) {
	s.DelegationTraceStore.UpdateContextUsage(taskID, usage, compress)
	s.observer.UpdateContextUsage(taskID, usage, compress)
}

func (s *mirroredTraceStore) Shutdown(ctx context.Context) error {
	if s.observer == nil {
		return nil
	}
	return s.observer.Shutdown(ctx)
}

func (s *mirroredTraceStore) Subscribe() (<-chan []DelegationSession, func()) {
	if subscriber, ok := s.DelegationTraceStore.(DelegationTraceSubscriber); ok {
		return subscriber.Subscribe()
	}
	ch := make(chan []DelegationSession)
	close(ch)
	return ch, func() {}
}

func (s *mirroredTraceStore) SubscribeEvents() (<-chan TraceStreamEvent, func()) {
	if subscriber, ok := s.DelegationTraceStore.(DelegationEventSubscriber); ok {
		return subscriber.SubscribeEvents()
	}
	ch := make(chan TraceStreamEvent)
	close(ch)
	return ch, func() {}
}
