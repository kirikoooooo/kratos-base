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
}

func NewDelegationTraceStore() biz.DelegationTraceStore {
	return &memoryTraceStore{
		sessions: make(map[string]*biz.DelegationSession),
		order:    make([]string, 0, maxTraceSessions),
	}
}

func (s *memoryTraceStore) StartTask(taskID string, agent biz.TaskAgent, prompt string, status biz.TaskStatus) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.sessions[taskID]; ok {
		existing.RootAgent = agent.String()
		existing.Prompt = prompt
		existing.Status = string(status)
		existing.UpdatedAt = now
		return
	}

	s.sessions[taskID] = &biz.DelegationSession{
		TaskID:    taskID,
		RootAgent: agent.String(),
		Prompt:    prompt,
		Status:    string(status),
		CreatedAt: now,
		UpdatedAt: now,
		Events:    make([]biz.DelegationEvent, 0, 8),
	}
	s.order = append([]string{taskID}, s.order...)
	s.trimLocked()
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
	if strings.TrimSpace(event.PromptPreview) != "" && session.Prompt == "" {
		session.Prompt = event.PromptPreview
	}
	session.Events = append(session.Events, event)
	session.UpdatedAt = event.Time
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
	cloned.Events = slices.Clone(session.Events)
	return cloned
}
