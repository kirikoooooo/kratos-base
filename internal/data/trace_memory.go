package data

import (
	"slices"
	"strings"
	"sync"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
)

const maxTraceSessions = 32

type memoryTraceStore struct {
	mu       sync.RWMutex
	sessions map[string]*biz.DelegationSession
	order    []string
	subs     map[chan []biz.DelegationSession]struct{}
}

func NewDelegationTraceStore() biz.DelegationTraceStore {
	return &memoryTraceStore{
		sessions: make(map[string]*biz.DelegationSession),
		order:    make([]string, 0, maxTraceSessions),
		subs:     make(map[chan []biz.DelegationSession]struct{}),
	}
}

func (s *memoryTraceStore) StartTask(taskID string, agent biz.TaskAgent, status biz.TaskStatus) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.sessions[taskID]; ok {
		existing.RootAgent = agent.String()
		existing.Status = string(status)
		existing.UpdatedAt = now
		s.broadcastLocked()
		return
	}

	s.sessions[taskID] = &biz.DelegationSession{
		TaskID:    taskID,
		RootAgent: agent.String(),
		Status:    string(status),
		CreatedAt: now,
		UpdatedAt: now,
		Plan:      make([]biz.PlanStep, 0, 4),
		Events:    make([]biz.DelegationEvent, 0, 8),
	}
	s.order = append([]string{taskID}, s.order...)
	s.trimLocked()
	s.broadcastLocked()
}

func (s *memoryTraceStore) UpdateTask(taskID string, status biz.TaskStatus, result *taskv1.TaskResult, err error) {
	if strings.TrimSpace(taskID) == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.ensureSessionLocked(taskID)
	session.Status = string(status)
	session.UpdatedAt = time.Now()
	if result != nil {
		session.ResultSummary = strings.TrimSpace(result.GetSummary())
		session.ResultOutput = strings.TrimSpace(result.GetOutput())
	}
	if err != nil {
		session.Error = err.Error()
	} else {
		session.Error = ""
	}
	s.broadcastLocked()
}

func (s *memoryTraceStore) AppendEvent(event biz.DelegationEvent) {
	if strings.TrimSpace(event.TaskID) == "" {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.ensureSessionLocked(event.TaskID)
	if strings.TrimSpace(event.Agent) != "" && session.RootAgent == "" {
		session.RootAgent = event.Agent
	}
	session.Events = append(session.Events, event)
	session.Plan = advancePlan(session.Plan, event)
	session.UpdatedAt = event.Time
	s.broadcastLocked()
}

func (s *memoryTraceStore) UpdatePlan(taskID string, steps []biz.PlanStep) {
	if strings.TrimSpace(taskID) == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.ensureSessionLocked(taskID)
	session.Plan = slices.Clone(steps)
	session.UpdatedAt = time.Now()
	s.broadcastLocked()
}

func (s *memoryTraceStore) ListSessions(limit int) []biz.DelegationSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	result := make([]biz.DelegationSession, 0, limit)
	for _, taskID := range s.order[:limit] {
		session, ok := s.sessions[taskID]
		if !ok || session == nil {
			continue
		}
		result = append(result, cloneTraceSession(session))
	}
	return result
}

func (s *memoryTraceStore) ensureSessionLocked(taskID string) *biz.DelegationSession {
	if session, ok := s.sessions[taskID]; ok {
		return session
	}
	session := &biz.DelegationSession{
		TaskID:    taskID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Plan:      make([]biz.PlanStep, 0, 4),
		Events:    make([]biz.DelegationEvent, 0, 8),
	}
	s.sessions[taskID] = session
	s.order = append([]string{taskID}, s.order...)
	s.trimLocked()
	return session
}

func (s *memoryTraceStore) trimLocked() {
	if len(s.order) <= maxTraceSessions {
		return
	}
	for _, taskID := range s.order[maxTraceSessions:] {
		delete(s.sessions, taskID)
	}
	s.order = slices.Clone(s.order[:maxTraceSessions])
}

func cloneTraceSession(session *biz.DelegationSession) biz.DelegationSession {
	if session == nil {
		return biz.DelegationSession{}
	}
	cloned := *session
	cloned.Plan = slices.Clone(session.Plan)
	cloned.Events = slices.Clone(session.Events)
	return cloned
}

func (s *memoryTraceStore) Subscribe() (<-chan []biz.DelegationSession, func()) {
	ch := make(chan []biz.DelegationSession, 1)

	s.mu.Lock()
	s.subs[ch] = struct{}{}
	snapshot := s.listSessionsLocked(12)
	s.mu.Unlock()

	ch <- snapshot

	cancel := func() {
		s.mu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.mu.Unlock()
	}

	return ch, cancel
}

func (s *memoryTraceStore) listSessionsLocked(limit int) []biz.DelegationSession {
	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	result := make([]biz.DelegationSession, 0, limit)
	for _, taskID := range s.order[:limit] {
		session, ok := s.sessions[taskID]
		if !ok || session == nil {
			continue
		}
		result = append(result, cloneTraceSession(session))
	}
	return result
}

func (s *memoryTraceStore) broadcastLocked() {
	if len(s.subs) == 0 {
		return
	}
	snapshot := s.listSessionsLocked(12)
	for ch := range s.subs {
		select {
		case ch <- snapshot:
		default:
		}
	}
}
