package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
	errconst "kratos-demo/internal/consts/error"
	"kratos-demo/internal/consts/public"
	dataagent "kratos-demo/internal/data/agent_runtime"
	datasession "kratos-demo/internal/data/session"
	datatasking "kratos-demo/internal/data/tasking"
	datatrace "kratos-demo/internal/data/trace"
)

type DashboardService struct {
	trace        datatrace.DelegationTraceStore
	uc           *biz.AgentRuntimeUsecase
	memory       dataagent.AgentMemory
	sessionStore datasession.SessionStore
	config       *conf.Runtime
	page         *template.Template
	mu           sync.RWMutex
	agents       map[public.AgentKind]*dashboardAgentState
}

type dashboardAgentState struct {
	Agent      string    `json:"agent"`
	Started    bool      `json:"started"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	LastTaskID string    `json:"last_task_id,omitempty"`
	LastPrompt string    `json:"last_prompt,omitempty"`
}

type dashboardMessageRequest struct {
	Agent  string `json:"agent"`
	Prompt string `json:"prompt"`
}

func NewDashboardService(trace datatrace.DelegationTraceStore, uc *biz.AgentRuntimeUsecase, memory dataagent.AgentMemory, sessionStore datasession.SessionStore, config *conf.Runtime) *DashboardService {
	return &DashboardService{
		trace:        trace,
		uc:           uc,
		memory:       memory,
		sessionStore: sessionStore,
		config:       config,
		page:         template.Must(template.New("dashboard").Parse(dashboardTemplate)),
		agents: map[public.AgentKind]*dashboardAgentState{
			public.AgentKindDefault: {
				Agent: string(public.AgentKindDefault),
			},
			public.AgentKindRouter: {
				Agent: string(public.AgentKindRouter),
			},
			public.AgentKindCoder: {
				Agent: string(public.AgentKindCoder),
			},
			public.AgentKindReviewer: {
				Agent: string(public.AgentKindReviewer),
			},
		},
	}
}

func (s *DashboardService) Register(mux interface {
	HandleFunc(string, http.HandlerFunc)
}) {
	mux.HandleFunc("/debug/a2a", s.handleDashboard)
	mux.HandleFunc("/debug/a2a/state", s.handleState)
	mux.HandleFunc("/debug/a2a/stream", s.handleStream)
	mux.HandleFunc("/debug/a2a/events", s.handleTraceEvents)
	mux.HandleFunc("/debug/a2a/agent/start", s.handleStartAgent)
	mux.HandleFunc("/debug/a2a/message", s.handleMessage)
	mux.HandleFunc("/debug/a2a/verify", s.handleVerify)
}

func (s *DashboardService) handleTraceEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := prepareSSE(w)
	if !ok {
		return
	}
	subscriber, ok := s.trace.(datatrace.DelegationEventSubscriber)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "trace store does not support event subscriptions"})
		return
	}
	taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
	ch, cancel := subscriber.SubscribeEvents()
	defer cancel()
	if !writeSSE(w, flusher, "ready", map[string]string{"task_id": taskID}) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-ch:
			if !open {
				return
			}
			if taskID != "" && event.TaskID != taskID {
				continue
			}
			if !writeSSE(w, flusher, event.Type, event) {
				return
			}
		}
	}
}

func (s *DashboardService) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.page.Execute(w, nil)
}

func (s *DashboardService) handleState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"server_time": time.Now().Format(time.RFC3339),
		"agents":      s.agentStates(),
		"remotes":     s.remoteStates(),
		"sessions":    s.delegationSessions(),
	})
}

func (s *DashboardService) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming is not supported"})
		return
	}

	subscriber, ok := s.trace.(datatrace.DelegationTraceSubscriber)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "trace store does not support subscriptions"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := subscriber.Subscribe()
	defer cancel()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case sessions, ok := <-ch:
			if !ok {
				return
			}
			sessions = s.enrichSessions(sessions)
			payload := map[string]any{
				"server_time": time.Now().Format(time.RFC3339),
				"agents":      s.agentStates(),
				"remotes":     s.remoteStates(),
				"sessions":    sessions,
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: state\ndata: %s\n\n", raw)
			flusher.Flush()
		}
	}
}

func (s *DashboardService) handleStartAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	agent, err := normalizeDashboardAgent(r.URL.Query().Get("agent"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	state := s.markAgentStarted(agent)
	writeJSON(w, http.StatusOK, map[string]any{
		"agent":  state,
		"agents": s.agentStates(),
	})
}

func (s *DashboardService) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req dashboardMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	agent, err := normalizeDashboardAgent(req.Agent)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "prompt is required"})
		return
	}
	if !s.isAgentStarted(agent) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": fmt.Sprintf("agent %s has not been started", agent)})
		return
	}

	taskID := dashboardSessionID(agent)
	s.rememberAgentTask(agent, taskID, prompt)

	result, err := s.runConversation(context.Background(), taskID, agent, prompt)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"task_id":  taskID,
			"error":    err.Error(),
			"agents":   s.agentStates(),
			"sessions": s.delegationSessions(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"task_id":  taskID,
		"result":   result,
		"agents":   s.agentStates(),
		"sessions": s.delegationSessions(),
	})
}

func (s *DashboardService) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	agent := public.AgentKind(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("agent"))))
	if agent == "" {
		agent = public.AgentKindCoder
	}
	prompt := strings.TrimSpace(r.URL.Query().Get("prompt"))
	taskID := fmt.Sprintf("verify-%d", time.Now().UnixNano())

	result, err := s.uc.VerifyDelegation(context.Background(), taskID, agent, prompt)
	if errors.Is(err, errconst.ErrDelegationNotSupported) {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "runtime does not support verification"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"task_id":  taskID,
			"error":    err.Error(),
			"sessions": s.delegationSessions(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id":  taskID,
		"result":   result,
		"sessions": s.delegationSessions(),
	})
}

func (s *DashboardService) delegationSessions() []datatrace.DelegationSession {
	if s.trace == nil {
		return nil
	}
	return s.enrichSessions(s.trace.ListSessions(12))
}

func mergeContextUsage(traceUsage, liveUsage datatrace.ContextUsageSnapshot) datatrace.ContextUsageSnapshot {
	if liveUsage.Threshold <= 0 && traceUsage.Threshold > 0 {
		liveUsage.Threshold = traceUsage.Threshold
	}
	if liveUsage.Threshold > 0 && liveUsage.UsagePercent == 0 && liveUsage.EstimatedChars > 0 {
		liveUsage.UsagePercent = float64(liveUsage.EstimatedChars) / float64(liveUsage.Threshold) * 100
		if liveUsage.UsagePercent > 100 {
			liveUsage.UsagePercent = 100
		}
	}
	if traceUsage.CompressCount > liveUsage.CompressCount {
		liveUsage.CompressCount = traceUsage.CompressCount
	}
	if traceUsage.LastOriginalChars > 0 {
		liveUsage.LastOriginalChars = traceUsage.LastOriginalChars
		liveUsage.LastCompressedChars = traceUsage.LastCompressedChars
		liveUsage.LastOmittedTurns = traceUsage.LastOmittedTurns
		liveUsage.LastTruncatedTools = traceUsage.LastTruncatedTools
	}
	if traceUsage.UpdatedAt.After(liveUsage.UpdatedAt) {
		liveUsage.UpdatedAt = traceUsage.UpdatedAt
	}
	liveUsage.NeedsCompress = liveUsage.Threshold > 0 && liveUsage.EstimatedChars > liveUsage.Threshold
	return liveUsage
}

func (s *DashboardService) enrichSessions(sessions []datatrace.DelegationSession) []datatrace.DelegationSession {
	if len(sessions) == 0 {
		return sessions
	}
	ctx := context.Background()
	for i := range sessions {
		if s.memory != nil {
			sessions[i].ConversationPreview = s.memory.ConversationPreview(ctx, sessions[i].TaskID)
			liveUsage := s.memory.ConversationContextUsage(ctx, sessions[i].TaskID, nil)
			liveTrace := datatrace.ContextUsageSnapshot{
				EstimatedChars:      liveUsage.EstimatedChars,
				Threshold:           liveUsage.Threshold,
				UsagePercent:        liveUsage.UsagePercent,
				NeedsCompress:       liveUsage.NeedsCompress,
				CompressCount:       liveUsage.CompressCount,
				LastOriginalChars:   liveUsage.LastOriginalChars,
				LastCompressedChars: liveUsage.LastCompressedChars,
				LastOmittedTurns:    liveUsage.LastOmittedTurns,
				LastTruncatedTools:  liveUsage.LastTruncatedTools,
			}
			sessions[i].ContextUsage = mergeContextUsage(sessions[i].ContextUsage, liveTrace)
		}
		if s.sessionStore != nil {
			if sess := s.sessionStore.Open(sessions[i].TaskID); sess != nil {
				sessions[i].FileChanges = sess.FileChanges()
			}
		}
	}
	return sessions
}

func (s *DashboardService) agentStates() []dashboardAgentState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := []public.AgentKind{public.AgentKindDefault, public.AgentKindRouter, public.AgentKindCoder, public.AgentKindReviewer}
	result := make([]dashboardAgentState, 0, len(agents))
	for _, agent := range agents {
		state, ok := s.agents[agent]
		if !ok || state == nil {
			result = append(result, dashboardAgentState{Agent: string(agent)})
			continue
		}
		result = append(result, *state)
	}
	return result
}

func (s *DashboardService) StartAllAgents() {
	for _, agent := range []public.AgentKind{public.AgentKindDefault, public.AgentKindRouter, public.AgentKindCoder, public.AgentKindReviewer} {
		s.markAgentStarted(agent)
	}
}

func (s *DashboardService) markAgentStarted(agent public.AgentKind) dashboardAgentState {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.agents[agent]
	if !ok || state == nil {
		state = &dashboardAgentState{Agent: string(agent)}
		s.agents[agent] = state
	}
	if !state.Started {
		state.Started = true
		state.StartedAt = time.Now()
	}
	return *state
}

func (s *DashboardService) isAgentStarted(agent public.AgentKind) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.agents[agent]
	return ok && state != nil && state.Started
}

func (s *DashboardService) rememberAgentTask(agent public.AgentKind, taskID, prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.agents[agent]
	if !ok || state == nil {
		state = &dashboardAgentState{Agent: string(agent)}
		s.agents[agent] = state
	}
	state.LastTaskID = taskID
	state.LastPrompt = previewDashboardPrompt(prompt)
}

func (s *DashboardService) runConversation(ctx context.Context, taskID string, agent public.AgentKind, prompt string) (*taskv1.TaskResult, error) {
	if s.memory != nil {
		if err := s.memory.StartConversation(ctx, taskID, agent, prompt); err != nil {
			return nil, fmt.Errorf("start conversation memory: %w", err)
		}
	}
	if s.trace != nil {
		s.trace.StartTask(taskID, agent, string(public.TaskStatusRunning))
		s.trace.UpdatePlan(taskID, datatasking.InitialPlan(agent, prompt))
		s.trace.AppendEvent(datatrace.DelegationEvent{
			Time:          time.Now(),
			TaskID:        taskID,
			Agent:         string(agent),
			Stage:         "message_received",
			PromptPreview: previewDashboardPrompt(prompt),
		})
	}

	var (
		result *taskv1.TaskResult
		err    error
	)

	switch agent {
	case public.AgentKindDefault:
		result, err = s.executeAgent(ctx, taskID, public.AgentKindDefault, prompt)
	case public.AgentKindRouter:
		result, err = s.runRouterConversation(ctx, taskID, prompt, true)
	case public.AgentKindCoder:
		result, err = s.runCoderConversation(ctx, taskID, prompt)
	case public.AgentKindReviewer:
		result, err = s.executeAgent(ctx, taskID, public.AgentKindReviewer, prompt)
	default:
		err = fmt.Errorf("agent %s is not supported by dashboard", agent)
	}

	if err != nil && s.memory != nil {
		_ = s.memory.RecordSessionError(ctx, dataagent.SessionErrorRecord{
			SessionID: taskID,
			Agent:     string(agent),
			Stage:     "message_failed",
			Message:   err.Error(),
		})
	}
	if s.trace != nil {
		if err != nil {
			s.trace.AppendEvent(datatrace.DelegationEvent{
				Time:   time.Now(),
				TaskID: taskID,
				Agent:  string(agent),
				Stage:  "message_failed",
				Error:  err.Error(),
			})
			s.trace.UpdateTask(taskID, string(public.TaskStatusFailed), nil, err)
		} else {
			s.trace.AppendEvent(datatrace.DelegationEvent{
				Time:       time.Now(),
				TaskID:     taskID,
				Agent:      string(agent),
				Stage:      "message_done",
				Summary:    result.GetSummary(),
				DurationMS: 0,
			})
			s.trace.AppendEvent(datatrace.DelegationEvent{
				Time:       time.Now(),
				TaskID:     taskID,
				Agent:      string(agent),
				Stage:      "final_answer",
				Summary:    result.GetSummary(),
				ToolOutput: strings.TrimSpace(result.GetOutput()),
				DurationMS: 0,
			})
			s.trace.UpdateTask(taskID, string(public.TaskStatusDone), result, nil)
		}
	}

	return result, err
}

func (s *DashboardService) runRouterConversation(ctx context.Context, taskID, prompt string, allowDelegate bool) (*taskv1.TaskResult, error) {
	if allowDelegate && shouldDelegateToCoder(prompt) {
		if !s.isAgentStarted(public.AgentKindCoder) {
			return nil, errors.New("coder agent is not started")
		}
		if s.trace != nil {
			s.trace.AppendEvent(datatrace.DelegationEvent{
				Time:          time.Now(),
				TaskID:        taskID,
				Agent:         string(public.AgentKindRouter),
				Stage:         "delegate_local",
				Mode:          string(public.AgentKindCoder),
				PromptPreview: previewDashboardPrompt(prompt),
			})
		}
		result, err := s.executeAgent(ctx, taskID, public.AgentKindCoder, prompt)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	if s.trace != nil {
		s.trace.AppendEvent(datatrace.DelegationEvent{
			Time:          time.Now(),
			TaskID:        taskID,
			Agent:         string(public.AgentKindRouter),
			Stage:         "handle_direct",
			PromptPreview: previewDashboardPrompt(prompt),
		})
	}
	return s.executeAgent(ctx, taskID, public.AgentKindRouter, prompt)
}

func (s *DashboardService) runCoderConversation(ctx context.Context, taskID, prompt string) (*taskv1.TaskResult, error) {
	if shouldDelegateToRouter(prompt) {
		if !s.isAgentStarted(public.AgentKindRouter) {
			return nil, errors.New("router agent is not started")
		}
		if s.trace != nil {
			s.trace.AppendEvent(datatrace.DelegationEvent{
				Time:          time.Now(),
				TaskID:        taskID,
				Agent:         string(public.AgentKindCoder),
				Stage:         "delegate_local",
				Mode:          string(public.AgentKindRouter),
				PromptPreview: previewDashboardPrompt(prompt),
			})
		}
		result, err := s.runRouterConversation(ctx, taskID, prompt, false)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	if s.trace != nil {
		s.trace.AppendEvent(datatrace.DelegationEvent{
			Time:          time.Now(),
			TaskID:        taskID,
			Agent:         string(public.AgentKindCoder),
			Stage:         "handle_direct",
			PromptPreview: previewDashboardPrompt(prompt),
		})
	}
	return s.executeAgent(ctx, taskID, public.AgentKindCoder, prompt)
}

func (s *DashboardService) executeAgent(ctx context.Context, taskID string, agent public.AgentKind, prompt string) (*taskv1.TaskResult, error) {
	if s == nil || s.uc == nil {
		return nil, errconst.ErrAgentRuntimeUnavailable
	}
	return s.uc.ExecuteTask(ctx, &taskv1.TaskCommand{
		TaskID: taskID,
		Agent:  string(agent),
		Prompt: prompt,
	})
}

func dashboardSessionID(agent public.AgentKind) string {
	return fmt.Sprintf("dashboard-%s", strings.TrimSpace(string(agent)))
}

func normalizeDashboardAgent(raw string) (public.AgentKind, error) {
	agent := public.AgentKind(strings.ToLower(strings.TrimSpace(raw)))
	switch agent {
	case public.AgentKindDefault, public.AgentKindRouter, public.AgentKindCoder, public.AgentKindReviewer:
		return agent, nil
	default:
		return "", fmt.Errorf("unsupported dashboard agent: %s", raw)
	}
}

func shouldDelegateToCoder(prompt string) bool {
	text := strings.ToLower(strings.TrimSpace(prompt))
	keywords := []string{"实现", "编码", "开发", "写一个", "修复", "bug", "接口", "函数", "重构", "代码", "golang", "go ", "api"}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func shouldDelegateToRouter(prompt string) bool {
	text := strings.ToLower(strings.TrimSpace(prompt))
	keywords := []string{"router", "路由", "委派", "拆解", "拆分", "规划", "协调", "分配", "转给"}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func previewDashboardPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if len([]rune(prompt)) <= 120 {
		return prompt
	}
	runes := []rune(prompt)
	return string(runes[:120]) + "..."
}

func (s *DashboardService) remoteStates() []map[string]any {
	if s == nil || s.config == nil {
		return nil
	}
	remotes := s.config.GetRemotes()
	result := make([]map[string]any, 0, len(remotes))
	for _, remote := range remotes {
		if remote == nil {
			continue
		}
		result = append(result, map[string]any{
			"agent":   strings.TrimSpace(remote.GetAgent()),
			"target":  strings.TrimSpace(remote.GetTarget()),
			"timeout": remote.GetTimeout(),
		})
	}
	return result
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

const dashboardTemplate = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Agent Runtime Dashboard</title>
  <style>
    :root { --ink:#1e2430; --muted:#6b7280; --line:rgba(30,36,48,.12); --accent:#0f766e; --accent-soft:#d9f3ef; --warn:#c2410c; --ok:#166534; --shadow:0 18px 48px rgba(30,36,48,.12); }
    * { box-sizing: border-box; }
    body { margin:0; font-family:"Segoe UI","PingFang SC",sans-serif; color:var(--ink); background: radial-gradient(circle at top left, #fde68a 0, transparent 24%), radial-gradient(circle at top right, #bfdbfe 0, transparent 28%), linear-gradient(135deg, #f8f4ec, #edf5f3 58%, #eef2ff); min-height:100vh; }
    .wrap { width:min(1180px, calc(100vw - 32px)); margin:0 auto; padding:28px 0 40px; }
    .hero,.panel { background:rgba(255,255,255,.78); backdrop-filter:blur(14px); border:1px solid var(--line); border-radius:24px; box-shadow:var(--shadow); }
    .hero { padding:28px; display:grid; gap:16px; }
    .eyebrow { letter-spacing:.18em; text-transform:uppercase; font-size:12px; color:var(--muted); }
    h1 { margin:0; font-size:clamp(28px,6vw,56px); line-height:.95; max-width:9em; }
    .sub { color:var(--muted); max-width:62ch; line-height:1.6; }
    .formline { display:flex; flex-wrap:wrap; gap:10px; margin-top:8px; align-items:center; }
    button,input,select,textarea { font:inherit; }
    button { border:0; border-radius:999px; padding:12px 18px; background:var(--ink); color:#fff; cursor:pointer; }
    button.secondary { background:#fff; color:var(--ink); border:1px solid var(--line); }
    .grid { margin-top:20px; display:grid; grid-template-columns:320px 1fr; gap:18px; }
    .panel { padding:18px; }
    .panel h2 { margin:0 0 12px; font-size:16px; }
    .meta,.session-list,.events,.agent-grid { display:grid; gap:10px; }
    .agent-grid { grid-template-columns:repeat(auto-fit, minmax(220px, 1fr)); }
    .chip { display:inline-flex; align-items:center; padding:6px 10px; border-radius:999px; background:var(--accent-soft); color:var(--accent); font-size:12px; font-weight:600; }
    .legend { display:flex; flex-wrap:wrap; gap:8px; margin-top:8px; }
    .legend-item { display:inline-flex; align-items:center; gap:8px; font-size:12px; color:var(--muted); }
    .legend-dot { width:10px; height:10px; border-radius:999px; display:inline-block; }
    .legend-dot.user { background:#0f766e; }
    .legend-dot.internal { background:#94a3b8; }
    .remote,.session,.event,.agent-card,.result-card { border:1px solid var(--line); border-radius:16px; padding:12px 14px; background:rgba(255,255,255,.72); }
    .session { cursor:pointer; }
    .session.active { border-color:rgba(15,118,110,.36); transform:translateX(4px); }
    .k,.small { color:var(--muted); font-size:12px; }
    .v { margin-top:4px; font-weight:600; word-break:break-word; }
    .status-ok { color:var(--ok); } .status-failed { color:var(--warn); }
    .status-idle { color:var(--muted); }
    .event-top { display:flex; justify-content:space-between; gap:12px; align-items:baseline; margin-bottom:6px; }
    .event-stage { font-weight:700; letter-spacing:.04em; font-size:13px; }
    .event-raw { color:var(--muted); font-size:11px; margin-top:2px; font-family:ui-monospace,SFMono-Regular,Consolas,monospace; }
    .prompt { white-space:pre-wrap; line-height:1.5; color:var(--muted); margin-top:8px; }
    .event.user-visible { border-color:rgba(15,118,110,.28); background:rgba(217,243,239,.42); }
    .event.internal-flow { border-color:rgba(148,163,184,.28); background:rgba(241,245,249,.78); }
    .event.final-answer { border-color:rgba(15,118,110,.42); background:rgba(217,243,239,.62); }
    .event-kind { display:inline-flex; align-items:center; padding:3px 8px; border-radius:999px; font-size:11px; font-weight:700; }
    .event-kind.user { background:rgba(15,118,110,.12); color:#0f766e; }
    .event-kind.internal { background:rgba(148,163,184,.18); color:#475569; }
    .preview-block { margin-top:8px; border:1px solid var(--line); border-radius:12px; background:rgba(255,255,255,.72); overflow:hidden; }
    .preview-head { padding:8px 10px; border-bottom:1px solid var(--line); font-size:12px; color:var(--muted); }
    details.preview { margin-top:8px; }
    details.preview summary { cursor:pointer; list-style:none; color:var(--accent); font-size:12px; font-weight:600; }
    details.preview summary::-webkit-details-marker { display:none; }
    details.preview summary::before { content:"展开"; margin-right:6px; }
    details.preview[open] summary::before { content:"收起"; }
    .preview-body { white-space:pre-wrap; line-height:1.5; color:var(--muted); margin-top:8px; }
    .empty { padding:20px; border:1px dashed var(--line); border-radius:16px; color:var(--muted); text-align:center; }
    .file-change { border:1px solid var(--line); border-radius:14px; padding:12px 14px; background:rgba(255,255,255,.72); margin-top:10px; }
    .file-change-head { display:flex; justify-content:space-between; gap:10px; align-items:baseline; margin-bottom:8px; }
    .file-change-status { font-size:11px; font-weight:700; padding:3px 8px; border-radius:999px; background:rgba(15,118,110,.12); color:#0f766e; }
    .file-change-status.created { background:rgba(15,118,110,.12); color:#0f766e; }
    .file-change-status.modified { background:rgba(37,99,235,.12); color:#1d4ed8; }
    .file-change-status.deleted { background:rgba(194,65,12,.12); color:#c2410c; }
    .diff-view { margin-top:8px; border:1px solid var(--line); border-radius:12px; background:#0f172a; color:#e2e8f0; overflow:auto; max-height:360px; }
    .diff-line { display:block; padding:1px 10px; font-family:ui-monospace,SFMono-Regular,Consolas,monospace; font-size:12px; line-height:1.45; white-space:pre; }
    .diff-line.add { background:rgba(22,163,74,.22); color:#86efac; }
    .diff-line.del { background:rgba(220,38,38,.22); color:#fca5a5; }
    .diff-line.ctx { color:#94a3b8; }
    .diff-line.meta { color:#67e8f9; background:rgba(8,145,178,.18); }
    .context-usage { border:1px solid var(--line); border-radius:14px; padding:12px 14px; background:rgba(255,255,255,.78); margin-top:10px; }
    .context-usage-bar { height:10px; border-radius:999px; background:rgba(148,163,184,.25); overflow:hidden; margin-top:8px; }
    .context-usage-fill { height:100%; border-radius:999px; background:linear-gradient(90deg,#0f766e,#14b8a6); transition:width .25s ease; }
    .context-usage-fill.warn { background:linear-gradient(90deg,#d97706,#f59e0b); }
    .context-usage-fill.critical { background:linear-gradient(90deg,#dc2626,#f87171); }
    .event.context-compress { border-color:rgba(217,119,6,.35); background:rgba(255,251,235,.82); }
    input,select,textarea { min-width:180px; padding:11px 14px; border-radius:12px; border:1px solid var(--line); background:rgba(255,255,255,.92); }
    textarea { width:100%; min-height:110px; resize:vertical; }
    .agent-actions { display:flex; gap:10px; flex-wrap:wrap; }
    @media (max-width:900px) { .grid { grid-template-columns:1fr; } }
  </style>
</head>
<body>
  <div class="wrap">
    <section class="hero">
      <div class="eyebrow">Runtime Inspection</div>
      <h1>观察任务执行与工具闭环</h1>
      <div class="sub">这个页面用于观察第一阶段通用 agent runtime 的任务执行过程。你可以直接给 default profile 发送任务，也可以继续验证 router / coder / reviewer 兼容链路。右侧时间线会展示输入、工具调用、委派、结果与失败原因。</div>
      <div class="formline">
        <span class="chip" id="serverTime">loading</span>
        <button id="startDefaultBtn">启动 Default Agent</button>
        <button id="startRouterBtn">启动 Router Agent</button>
        <button id="startCoderBtn">启动 Coder Agent</button>
        <button id="startReviewerBtn">启动 Reviewer Agent</button>
        <button class="secondary" id="refreshBtn">刷新状态</button>
      </div>
      <div class="legend">
        <div class="legend-item"><span class="legend-dot user"></span><span>给用户展示</span></div>
        <div class="legend-item"><span class="legend-dot internal"></span><span>内部数据流转</span></div>
      </div>
      <div class="agent-grid" id="agents"></div>
    </section>
    <section class="grid">
      <aside class="panel">
        <h2>Send Message</h2>
        <div class="meta">
          <select id="agent"><option value="default">default</option><option value="router">router</option><option value="coder">coder</option><option value="reviewer">reviewer</option></select>
          <textarea id="prompt">请先读取 README.md 的前 20 行，再总结第一阶段目标。</textarea>
          <div class="agent-actions"><button id="sendBtn">发送消息</button></div>
        </div>
        <h2 style="margin-top:18px;">Remote Targets</h2><div class="meta" id="remotes"></div>
        <h2 style="margin-top:18px;">Recent Sessions</h2><div class="session-list" id="sessions"></div>
      </aside>
      <main class="panel"><h2>Delegation Timeline</h2><div id="timeline"></div><h2 style="margin-top:18px;">Session File Changes</h2><div id="fileChanges"></div><h2 style="margin-top:18px;">Execution Plan</h2><div id="plan"></div></main>
    </section>
  </div>
  <script>
    let state = { sessions: [], remotes: [], agents: [] };
    let selectedTaskId = "";
    const expandedBlocks = new Set();
    const agentsEl = document.getElementById("agents");
    const remotesEl = document.getElementById("remotes");
    const sessionsEl = document.getElementById("sessions");
    const timelineEl = document.getElementById("timeline");
    const fileChangesEl = document.getElementById("fileChanges");
    const planEl = document.getElementById("plan");
    const serverTimeEl = document.getElementById("serverTime");
    async function loadState() {
      const res = await fetch("/debug/a2a/state");
      state = await res.json();
      serverTimeEl.textContent = state.server_time || "n/a";
      if (!selectedTaskId && state.sessions && state.sessions.length) { selectedTaskId = state.sessions[0].task_id; }
      render();
    }
    async function startAgent(agent) {
      const res = await fetch("/debug/a2a/agent/start?agent=" + encodeURIComponent(agent), { method: "POST" });
      await res.json();
      await loadState();
    }
    async function sendMessage() {
      const agent = document.getElementById("agent").value;
      const prompt = document.getElementById("prompt").value;
      const btn = document.getElementById("sendBtn");
      btn.disabled = true; btn.textContent = "发送中...";
      try {
        const res = await fetch("/debug/a2a/message", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ agent, prompt })
        });
        const payload = await res.json();
        if (payload.error) { alert(payload.error); }
        if (payload.task_id) { selectedTaskId = payload.task_id; }
        await loadState();
      } finally { btn.disabled = false; btn.textContent = "发送消息"; }
    }
    function render() { renderAgents(); renderRemotes(); renderSessions(); renderTimeline(); renderFileChanges(); renderPlan(); }
    function summarizeMultiline(text, maxLines) {
      const value = String(text || "").trim();
      if (!value) { return ""; }
      const lines = value.split("\n");
      if (lines.length <= maxLines) { return value; }
      return lines.slice(0, maxLines).join("\n") + "\n...";
    }
    function detailsOpenAttr(expandId) {
      return expandId && expandedBlocks.has(expandId) ? " open" : "";
    }
    function renderExpandableBlock(label, content, maxLines, expandId) {
      const value = String(content || "").trim();
      if (!value) { return ""; }
      const preview = summarizeMultiline(value, maxLines || 4);
      const escapedLabel = escapeHtml(label);
      const escapedPreview = escapeHtml(preview);
      const escapedValue = escapeHtml(value);
      const escapedExpandId = expandId ? escapeHtml(expandId) : "";
      const expandAttr = expandId ? ' data-expand-id="'+escapedExpandId+'"' : "";
      const openAttr = detailsOpenAttr(expandId);
      if (preview === value) {
        return '<div class="preview-block"><div class="preview-head">'+escapedLabel+'</div><div class="preview-body">'+escapedValue+'</div></div>';
      }
      return '<div class="preview-block"><div class="preview-head">'+escapedLabel+'</div><div class="preview-body">'+escapedPreview+'</div><details class="preview"'+expandAttr+openAttr+'><summary>查看完整内容</summary><div class="preview-body">'+escapedValue+'</div></details></div>';
    }
    function renderCollapsedBlock(label, preview, content, expandId) {
      const value = String(content || "").trim();
      if (!value) { return ""; }
      const previewValue = String(preview || "").trim() || summarizeMultiline(value, 4);
      const escapedExpandId = expandId ? escapeHtml(expandId) : "";
      const expandAttr = expandId ? ' data-expand-id="'+escapedExpandId+'"' : "";
      const openAttr = detailsOpenAttr(expandId);
      return '<div class="preview-block"><div class="preview-head">'+escapeHtml(label)+'</div><div class="preview-body">'+escapeHtml(previewValue)+'</div><details class="preview"'+expandAttr+openAttr+'><summary>查看完整内容</summary><div class="preview-body">'+escapeHtml(value)+'</div></details></div>';
    }
    function stageDisplayLabel(stage) {
      const labels = {
        message_received: "User Message",
        handle_direct: "Direct Handling",
        delegate_local: "Delegated",
        delegate_remote: "Delegated Remote",
        remote_execute_done: "Remote Task Finished",
        remote_execute_failed: "Remote Task Failed",
        tool_read_file: "Tool Call: Read File",
        tool_edit_file: "Tool Call: Edit File",
        tool_write_file: "Tool Call: Write File",
        tool_exec_command: "Tool Call: Run Command",
        message_done: "Message Done",
        message_failed: "Message Failed",
        final_answer: "Final Answer",
        verification_start: "Verification Started",
        verification_done: "Verification Done",
        verification_failed: "Verification Failed",
        task_running: "Task Running",
        task_done: "Task Done",
        task_failed: "Task Failed",
        context_compress: "Context Compressed"
      };
      return labels[stage] || String(stage || "-");
    }
    function stageExtraMeta(event) {
      const stage = String(event.stage || "");
      if (stage === "context_compress") { return "session memory compression"; }
      if (stage === "final_answer") { return "assistant response"; }
      if (stage.startsWith("tool_")) { return event.tool_name ? "tool: " + event.tool_name : "tool event"; }
      if (stage === "delegate_local" || stage === "delegate_remote") { return event.mode ? "to " + event.mode : "delegation"; }
      if (stage === "message_received") { return "task accepted"; }
      if (stage === "message_done") { return "runtime completed"; }
      return "";
    }
    function eventAudience(stage) {
      const userVisible = new Set(["message_received", "message_failed", "message_done", "final_answer"]);
      return userVisible.has(String(stage || "")) ? "user" : "internal";
    }
    function renderEventCard(event, taskId, eventIndex) {
      const stage = String(event.stage || "-");
      const isFinalAnswer = stage === "final_answer";
      const isContextCompress = stage === "context_compress";
      const isToolEvent = stage.startsWith("tool_");
      const audience = eventAudience(stage);
      const blockPrefix = String(taskId || "") + "::" + String(eventIndex);
      const body = []
      body.push('<div class="event '+(audience === "user" ? 'user-visible' : 'internal-flow')+(isFinalAnswer ? ' final-answer' : '')+(isContextCompress ? ' context-compress' : '')+'">');
      body.push('<div class="event-top"><div><div class="event-stage">'+escapeHtml(stageDisplayLabel(stage))+'</div>' + (stage ? '<div class="event-raw">'+escapeHtml(stage)+'</div>' : '') + '</div><div class="small">'+escapeHtml(event.time || "")+'</div></div>');
      body.push('<div class="event-kind '+(audience === "user" ? 'user' : 'internal')+'">'+(audience === "user" ? 'USER' : 'INTERNAL')+'</div>');
      const meta = stageExtraMeta(event);
      if (meta) { body.push('<div class="small">'+escapeHtml(meta)+'</div>'); }
      body.push('<div class="small">agent: '+escapeHtml(event.agent || "-")+(event.mode ? " / mode: " + escapeHtml(event.mode) : "")+'</div>');
      if (event.target) { body.push('<div class="small">target: '+escapeHtml(event.target)+'</div>'); }
      if (event.tool_name) { body.push('<div class="small">tool: '+escapeHtml(event.tool_name)+'</div>'); }
      if (event.exit_code !== undefined && event.exit_code !== 0) { body.push('<div class="small">exit_code: '+escapeHtml(String(event.exit_code))+'</div>'); }
      if (event.summary) { body.push('<div class="prompt">'+escapeHtml(event.summary)+'</div>'); }
      if (event.tool_input) { body.push(renderExpandableBlock("input", event.tool_input, 3, blockPrefix + "::input")); }
      if (event.tool_output) {
        if (isFinalAnswer) {
          body.push(renderExpandableBlock("final answer", event.tool_output, 8, blockPrefix + "::final_answer"));
        } else if (isToolEvent) {
          body.push(renderCollapsedBlock("output", event.summary, event.tool_output, blockPrefix + "::output"));
        } else {
          body.push(renderExpandableBlock("output", event.tool_output, 4, blockPrefix + "::output"));
        }
      }
      if (event.error) { body.push('<div class="prompt" style="color:#c2410c;">'+escapeHtml(event.error)+'</div>'); }
      if (event.prompt_preview) { body.push('<div class="prompt">'+escapeHtml(event.prompt_preview)+'</div>'); }
      body.push('</div>');
      return body.join("");
    }
    function renderAgents() {
      const agents = state.agents || [];
      if (!agents.length) { agentsEl.innerHTML = '<div class="empty">当前没有可展示的 agent 状态</div>'; return; }
      agentsEl.innerHTML = agents.map(agent => {
        const statusClass = agent.started ? 'status-ok' : 'status-idle';
        const statusText = agent.started ? 'running' : 'idle';
        return '<div class="agent-card"><div class="event-top"><div class="v">'+escapeHtml(agent.agent || '-')+'</div><div class="small '+statusClass+'">'+statusText+'</div></div><div class="k">started at</div><div class="v">'+escapeHtml(agent.started_at || '-')+'</div><div class="k" style="margin-top:8px;">last task</div><div class="v">'+escapeHtml(agent.last_task_id || '-')+'</div><div class="k" style="margin-top:8px;">last prompt</div><div class="prompt">'+escapeHtml(agent.last_prompt || '-')+'</div></div>';
      }).join('');
    }
    function renderRemotes() {
      const remotes = state.remotes || [];
      if (!remotes.length) { remotesEl.innerHTML = '<div class="empty">当前没有配置 remote agent</div>'; return; }
      remotesEl.innerHTML = remotes.map(remote => '<div class="remote"><div class="k">agent</div><div class="v">'+escapeHtml(remote.agent || "-")+'</div><div class="k" style="margin-top:10px;">target</div><div class="v">'+escapeHtml(remote.target || "-")+'</div><div class="k" style="margin-top:10px;">timeout</div><div class="v">'+escapeHtml(String(remote.timeout || 0))+'s</div></div>').join("");
    }
    function renderSessions() {
      const sessions = state.sessions || [];
      if (!sessions.length) { sessionsEl.innerHTML = '<div class="empty">还没有任务或验证记录</div>'; return; }
      sessionsEl.innerHTML = sessions.map(session => {
        const usage = session.context_usage || {};
        const pct = usage.threshold ? Math.min(100, Number(usage.usage_percent || 0)).toFixed(0) : "-";
        return '<div class="session '+(session.task_id === selectedTaskId ? "active" : "")+'" data-task-id="'+escapeHtml(session.task_id)+'"><div class="k">task</div><div class="v">'+escapeHtml(session.task_id)+'</div><div class="k" style="margin-top:8px;">root agent</div><div class="v">'+escapeHtml(session.root_agent || "-")+'</div><div class="k" style="margin-top:8px;">context</div><div class="v">'+escapeHtml(formatChars(usage.estimated_chars))+' / '+escapeHtml(formatChars(usage.threshold))+' ('+escapeHtml(pct)+'%)</div><div class="k" style="margin-top:8px;">status</div><div class="v '+(session.status === "failed" ? "status-failed" : "status-ok")+'">'+escapeHtml(session.status || "-")+'</div></div>';
      }).join("");
      document.querySelectorAll(".session").forEach(el => el.addEventListener("click", () => { selectedTaskId = el.dataset.taskId; renderSessions(); renderTimeline(); renderFileChanges(); renderPlan(); }));
    }
    function formatChars(n) {
      const value = Number(n || 0);
      if (value >= 1000) { return (value / 1000).toFixed(1) + "k"; }
      return String(value);
    }
    function renderContextUsageBlock(usage) {
      if (!usage || !usage.threshold) {
        return '<div class="context-usage"><div class="small">上下文统计未启用（context_compress_threshold: -1）</div></div>';
      }
      const percent = Math.max(0, Math.min(100, Number(usage.usage_percent || 0)));
      const fillClass = percent >= 95 ? "critical" : (percent >= 80 ? "warn" : "");
      const compressNote = usage.compress_count
        ? '<div class="small" style="margin-top:8px;">已压缩 '+escapeHtml(String(usage.compress_count))+' 次'
          + (usage.last_original_chars ? '；最近 '+escapeHtml(formatChars(usage.last_original_chars))+' → '+escapeHtml(formatChars(usage.last_compressed_chars)) : '')
          + '</div>'
        : '';
      return '<div class="context-usage"><div class="event-top"><div class="v">Context Usage</div><div class="small">'+escapeHtml(formatChars(usage.estimated_chars))+' / '+escapeHtml(formatChars(usage.threshold))+' chars</div></div>'
        + '<div class="context-usage-bar"><div class="context-usage-fill '+fillClass+'" style="width:'+percent+'%;"></div></div>'
        + '<div class="small" style="margin-top:6px;">'+percent.toFixed(1)+'%'
        + (usage.needs_compress ? ' · <span class="status-failed">超过阈值，发送 LLM 前将压缩</span>' : ' · <span class="status-ok">未超阈值</span>')
        + '</div>' + compressNote + '</div>';
    }
    function renderDiffLines(unifiedDiff) {
      const lines = String(unifiedDiff || "").split("\n");
      return lines.map(line => {
        let cls = "ctx";
        if (line.startsWith("+++ ") || line.startsWith("--- ") || line.startsWith("@@")) { cls = "meta"; }
        else if (line.startsWith("+")) { cls = "add"; }
        else if (line.startsWith("-")) { cls = "del"; }
        return '<span class="diff-line '+cls+'">'+escapeHtml(line)+'</span>';
      }).join("");
    }
    function renderFileChanges() {
      const sessions = state.sessions || [];
      const current = sessions.find(s => s.task_id === selectedTaskId) || sessions[0];
      if (!current) { fileChangesEl.innerHTML = '<div class="empty">选择一个 session 查看本次对话的文件变更</div>'; return; }
      const changes = current.file_changes || [];
      if (!changes.length) {
        fileChangesEl.innerHTML = '<div class="empty">这个 session 还没有通过 write_file / edit_file 修改文件</div>';
        return;
      }
      fileChangesEl.innerHTML = changes.map((change, index) => {
        const ops = (change.operations || []).map(op => escapeHtml((op.tool || "-") + (op.operation ? " / " + op.operation : "") + (op.summary ? " — " + op.summary : ""))).join("<br>");
        const expandId = String(current.task_id || "") + "::file-change::" + String(index);
        const diffBlock = change.unified_diff
          ? '<details class="preview" data-expand-id="'+escapeHtml(expandId)+'"'+detailsOpenAttr(expandId)+'><summary>查看 diff（相对会话开始）</summary><div class="diff-view">'+renderDiffLines(change.unified_diff)+'</div></details>'
          : '<div class="small" style="margin-top:8px;">无文本 diff（可能为二进制或空变更）</div>';
        return '<div class="file-change"><div class="file-change-head"><div class="v">'+escapeHtml(change.path || "-")+'</div><span class="file-change-status '+escapeHtml(change.status || "modified")+'">'+escapeHtml(change.status || "modified")+'</span></div>'
          + (ops ? '<div class="small">'+ops+'</div>' : '')
          + diffBlock
          + '</div>';
      }).join("");
    }
    if (!fileChangesEl.dataset.expandBound) {
      fileChangesEl.dataset.expandBound = "1";
      fileChangesEl.addEventListener("toggle", (evt) => {
        const el = evt.target;
        if (!el || !el.matches || !el.matches("details.preview")) { return; }
        const id = el.dataset.expandId;
        if (!id) { return; }
        if (el.open) { expandedBlocks.add(id); } else { expandedBlocks.delete(id); }
      }, true);
    }
    function renderTimeline() {
      const sessions = state.sessions || [];
      const current = sessions.find(s => s.task_id === selectedTaskId) || sessions[0];
      if (!current) { timelineEl.innerHTML = '<div class="empty">选择一个 session 查看事件链路</div>'; return; }
      selectedTaskId = current.task_id;
      const finalResult = '<div class="result-card" style="margin-top:12px;"><div class="event-top"><div class="v">最终返回消息</div><div class="small">summary / output</div></div>'
        + '<div class="k">summary</div><div class="v">'+escapeHtml(current.result_summary || '-')+'</div>'
        + '<div class="k" style="margin-top:10px;">output</div><div class="prompt">'+escapeHtml(current.result_output || '-')+'</div>'
        + (current.error ? '<div class="k" style="margin-top:10px;">error</div><div class="prompt" style="color:#c2410c;">'+escapeHtml(current.error)+'</div>' : '')
        + '</div>';
      const header = '<div class="event"><div class="event-top"><div><div class="small">task</div><div class="v">'+escapeHtml(current.task_id)+'</div></div><div class="chip">'+escapeHtml(current.status || "-")+'</div></div><div class="prompt">'+escapeHtml(current.conversation_preview || "")+'</div></div>'
        + renderContextUsageBlock(current.context_usage || {})
        + finalResult;
      const events = current.events || [];
      if (!events.length) { timelineEl.innerHTML = header + '<div class="empty" style="margin-top:12px;">这个 session 还没有采集到事件</div>'; return; }
      timelineEl.innerHTML = header + '<div class="events" style="margin-top:12px;">' + events.map((event, index) => renderEventCard(event, current.task_id, index)).join("") + '</div>';
    }
    if (!timelineEl.dataset.expandBound) {
      timelineEl.dataset.expandBound = "1";
      timelineEl.addEventListener("toggle", (evt) => {
        const el = evt.target;
        if (!el || !el.matches || !el.matches("details.preview")) { return; }
        const id = el.dataset.expandId;
        if (!id) { return; }
        if (el.open) { expandedBlocks.add(id); } else { expandedBlocks.delete(id); }
      }, true);
    }
    function renderPlan() {
      const sessions = state.sessions || [];
      const current = sessions.find(s => s.task_id === selectedTaskId) || sessions[0];
      if (!current || !current.plan || !current.plan.length) { planEl.innerHTML = '<div class="empty">这个 session 还没有计划数据</div>'; return; }
      planEl.innerHTML = current.plan.map(step => '<div class="event"><div class="event-top"><div class="v">'+escapeHtml(step.title || "-")+'</div><div class="small '+(step.status === "completed" ? "status-ok" : step.status === "failed" ? "status-failed" : "status-idle")+'">'+escapeHtml(step.status || "-")+'</div></div>' + (step.description ? '<div class="prompt">'+escapeHtml(step.description)+'</div>' : '') + '</div>').join("");
    }
    function escapeHtml(str) { return String(str).replaceAll("&","&amp;").replaceAll("<","&lt;").replaceAll(">","&gt;").replaceAll("\"","&quot;").replaceAll("'","&#39;"); }
    document.getElementById("refreshBtn").addEventListener("click", loadState);
    document.getElementById("startDefaultBtn").addEventListener("click", () => startAgent("default"));
    document.getElementById("startRouterBtn").addEventListener("click", () => startAgent("router"));
    document.getElementById("startCoderBtn").addEventListener("click", () => startAgent("coder"));
    document.getElementById("startReviewerBtn").addEventListener("click", () => startAgent("reviewer"));
    document.getElementById("sendBtn").addEventListener("click", sendMessage);
    function connectStream() {
      const es = new EventSource("/debug/a2a/stream");
      es.addEventListener("state", (evt) => {
        state = JSON.parse(evt.data);
        serverTimeEl.textContent = state.server_time || "n/a";
        if (!selectedTaskId && state.sessions && state.sessions.length) { selectedTaskId = state.sessions[0].task_id; }
        render();
      });
      es.onerror = () => {
        es.close();
        setTimeout(connectStream, 2000);
      };
    }
    loadState();
    connectStream();
    setInterval(loadState, 15000);
  </script>
</body>
</html>`
