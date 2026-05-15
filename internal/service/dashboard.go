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
)

type DashboardService struct {
	trace   biz.DelegationTraceStore
	runtime biz.AgentRuntime
	config  *conf.Runtime
	page    *template.Template
	mu      sync.RWMutex
	agents  map[biz.TaskAgent]*dashboardAgentState
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

func NewDashboardService(trace biz.DelegationTraceStore, runtime biz.AgentRuntime, config *conf.Runtime) *DashboardService {
	return &DashboardService{
		trace:   trace,
		runtime: runtime,
		config:  config,
		page:    template.Must(template.New("dashboard").Parse(dashboardTemplate)),
		agents: map[biz.TaskAgent]*dashboardAgentState{
			biz.TaskAgentRouter: {
				Agent: biz.TaskAgentRouter.String(),
			},
			biz.TaskAgentCoder: {
				Agent: biz.TaskAgentCoder.String(),
			},
		},
	}
}

func (s *DashboardService) Register(mux interface {
	HandleFunc(string, http.HandlerFunc)
}) {
	mux.HandleFunc("/debug/a2a", s.handleDashboard)
	mux.HandleFunc("/debug/a2a/state", s.handleState)
	mux.HandleFunc("/debug/a2a/agent/start", s.handleStartAgent)
	mux.HandleFunc("/debug/a2a/message", s.handleMessage)
	mux.HandleFunc("/debug/a2a/verify", s.handleVerify)
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
		"sessions":    s.sessions(),
	})
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

	taskID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	s.rememberAgentTask(agent, taskID, prompt)

	result, err := s.runConversation(context.Background(), taskID, agent, prompt)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"task_id":  taskID,
			"error":    err.Error(),
			"agents":   s.agentStates(),
			"sessions": s.sessions(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"task_id":  taskID,
		"result":   result,
		"agents":   s.agentStates(),
		"sessions": s.sessions(),
	})
}

func (s *DashboardService) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	agent := biz.TaskAgent(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("agent"))))
	if agent == "" {
		agent = biz.TaskAgentCoder
	}
	prompt := strings.TrimSpace(r.URL.Query().Get("prompt"))
	taskID := fmt.Sprintf("verify-%d", time.Now().UnixNano())

	verifier, ok := s.runtime.(biz.DelegationVerifier)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "runtime does not support verification"})
		return
	}
	result, err := verifier.VerifyDelegation(context.Background(), taskID, agent, prompt)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"task_id":  taskID,
			"error":    err.Error(),
			"sessions": s.sessions(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id":  taskID,
		"result":   result,
		"sessions": s.sessions(),
	})
}

func (s *DashboardService) sessions() []biz.DelegationSession {
	if s.trace == nil {
		return nil
	}
	return s.trace.ListSessions(12)
}

func (s *DashboardService) agentStates() []dashboardAgentState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := []biz.TaskAgent{biz.TaskAgentRouter, biz.TaskAgentCoder}
	result := make([]dashboardAgentState, 0, len(agents))
	for _, agent := range agents {
		state, ok := s.agents[agent]
		if !ok || state == nil {
			result = append(result, dashboardAgentState{Agent: agent.String()})
			continue
		}
		result = append(result, *state)
	}
	return result
}

func (s *DashboardService) markAgentStarted(agent biz.TaskAgent) dashboardAgentState {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.agents[agent]
	if !ok || state == nil {
		state = &dashboardAgentState{Agent: agent.String()}
		s.agents[agent] = state
	}
	if !state.Started {
		state.Started = true
		state.StartedAt = time.Now()
	}
	return *state
}

func (s *DashboardService) isAgentStarted(agent biz.TaskAgent) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.agents[agent]
	return ok && state != nil && state.Started
}

func (s *DashboardService) rememberAgentTask(agent biz.TaskAgent, taskID, prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.agents[agent]
	if !ok || state == nil {
		state = &dashboardAgentState{Agent: agent.String()}
		s.agents[agent] = state
	}
	state.LastTaskID = taskID
	state.LastPrompt = previewDashboardPrompt(prompt)
}

func (s *DashboardService) runConversation(ctx context.Context, taskID string, agent biz.TaskAgent, prompt string) (*taskv1.TaskResult, error) {
	if s.trace != nil {
		s.trace.StartTask(taskID, agent, prompt, biz.TaskStatusRunning)
		s.trace.AppendEvent(biz.DelegationEvent{
			Time:          time.Now(),
			TaskID:        taskID,
			Agent:         agent.String(),
			Stage:         "message_received",
			PromptPreview: previewDashboardPrompt(prompt),
		})
	}

	var (
		result *taskv1.TaskResult
		err    error
	)

	switch agent {
	case biz.TaskAgentRouter:
		result, err = s.runRouterConversation(ctx, taskID, prompt, true)
	case biz.TaskAgentCoder:
		result, err = s.runCoderConversation(ctx, taskID, prompt)
	default:
		err = fmt.Errorf("agent %s is not supported by dashboard", agent)
	}

	if s.trace != nil {
		if err != nil {
			s.trace.AppendEvent(biz.DelegationEvent{
				Time:   time.Now(),
				TaskID: taskID,
				Agent:  agent.String(),
				Stage:  "message_failed",
				Error:  err.Error(),
			})
			s.trace.UpdateTask(taskID, biz.TaskStatusFailed, nil, err)
		} else {
			s.trace.AppendEvent(biz.DelegationEvent{
				Time:       time.Now(),
				TaskID:     taskID,
				Agent:      agent.String(),
				Stage:      "message_done",
				Summary:    result.GetSummary(),
				DurationMS: 0,
			})
			s.trace.UpdateTask(taskID, biz.TaskStatusDone, result, nil)
		}
	}

	return result, err
}

func (s *DashboardService) runRouterConversation(ctx context.Context, taskID, prompt string, allowDelegate bool) (*taskv1.TaskResult, error) {
	if allowDelegate && shouldDelegateToCoder(prompt) {
		if !s.isAgentStarted(biz.TaskAgentCoder) {
			return nil, errors.New("coder agent is not started")
		}
		if s.trace != nil {
			s.trace.AppendEvent(biz.DelegationEvent{
				Time:          time.Now(),
				TaskID:        taskID,
				Agent:         biz.TaskAgentRouter.String(),
				Stage:         "delegate_local",
				Mode:          biz.TaskAgentCoder.String(),
				PromptPreview: previewDashboardPrompt(prompt),
			})
		}
		result, err := s.executeAgent(ctx, taskID, biz.TaskAgentCoder, prompt)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	if s.trace != nil {
		s.trace.AppendEvent(biz.DelegationEvent{
			Time:          time.Now(),
			TaskID:        taskID,
			Agent:         biz.TaskAgentRouter.String(),
			Stage:         "handle_direct",
			PromptPreview: previewDashboardPrompt(prompt),
		})
	}
	return s.executeAgent(ctx, taskID, biz.TaskAgentRouter, prompt)
}

func (s *DashboardService) runCoderConversation(ctx context.Context, taskID, prompt string) (*taskv1.TaskResult, error) {
	if shouldDelegateToRouter(prompt) {
		if !s.isAgentStarted(biz.TaskAgentRouter) {
			return nil, errors.New("router agent is not started")
		}
		if s.trace != nil {
			s.trace.AppendEvent(biz.DelegationEvent{
				Time:          time.Now(),
				TaskID:        taskID,
				Agent:         biz.TaskAgentCoder.String(),
				Stage:         "delegate_local",
				Mode:          biz.TaskAgentRouter.String(),
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
		s.trace.AppendEvent(biz.DelegationEvent{
			Time:          time.Now(),
			TaskID:        taskID,
			Agent:         biz.TaskAgentCoder.String(),
			Stage:         "handle_direct",
			PromptPreview: previewDashboardPrompt(prompt),
		})
	}
	return s.executeAgent(ctx, taskID, biz.TaskAgentCoder, prompt)
}

func (s *DashboardService) executeAgent(ctx context.Context, taskID string, agent biz.TaskAgent, prompt string) (*taskv1.TaskResult, error) {
	if s == nil || s.runtime == nil {
		return nil, errors.New("agent runtime is not available")
	}
	return s.runtime.ReceiveTask(ctx, &taskv1.TaskCommand{
		TaskID: taskID,
		Agent:  agent.String(),
		Prompt: prompt,
	})
}

func normalizeDashboardAgent(raw string) (biz.TaskAgent, error) {
	agent := biz.TaskAgent(strings.ToLower(strings.TrimSpace(raw)))
	switch agent {
	case biz.TaskAgentRouter, biz.TaskAgentCoder:
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
  <title>A2A Agent Dashboard</title>
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
    .remote,.session,.event,.agent-card,.result-card { border:1px solid var(--line); border-radius:16px; padding:12px 14px; background:rgba(255,255,255,.72); }
    .session { cursor:pointer; }
    .session.active { border-color:rgba(15,118,110,.36); transform:translateX(4px); }
    .k,.small { color:var(--muted); font-size:12px; }
    .v { margin-top:4px; font-weight:600; word-break:break-word; }
    .status-ok { color:var(--ok); } .status-failed { color:var(--warn); }
    .status-idle { color:var(--muted); }
    .event-top { display:flex; justify-content:space-between; gap:12px; align-items:baseline; margin-bottom:6px; }
    .event-stage { font-weight:700; text-transform:uppercase; letter-spacing:.08em; font-size:12px; }
    .prompt { white-space:pre-wrap; line-height:1.5; color:var(--muted); margin-top:8px; }
    .empty { padding:20px; border:1px dashed var(--line); border-radius:16px; color:var(--muted); text-align:center; }
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
      <h1>启动 Agent 并观察委派链路</h1>
      <div class="sub">这个页面现在用于启动 router / coder 两个常驻 agent，并向任意一个 agent 发送消息。消息会按内容决定是否委派给另一个 agent，右侧时间线会显示每一步事件。</div>
      <div class="formline">
        <span class="chip" id="serverTime">loading</span>
        <button id="startRouterBtn">启动 Router Agent</button>
        <button id="startCoderBtn">启动 Coder Agent</button>
        <button class="secondary" id="refreshBtn">刷新状态</button>
      </div>
      <div class="agent-grid" id="agents"></div>
    </section>
    <section class="grid">
      <aside class="panel">
        <h2>Send Message</h2>
        <div class="meta">
          <select id="agent"><option value="router">router</option><option value="coder">coder</option></select>
          <textarea id="prompt">请让 router 判断这是不是一个需要交给 coder 的实现任务，并展示委派过程。</textarea>
          <div class="agent-actions"><button id="sendBtn">发送消息</button></div>
        </div>
        <h2 style="margin-top:18px;">Remote Targets</h2><div class="meta" id="remotes"></div>
        <h2 style="margin-top:18px;">Recent Sessions</h2><div class="session-list" id="sessions"></div>
      </aside>
      <main class="panel"><h2>Delegation Timeline</h2><div id="timeline"></div></main>
    </section>
  </div>
  <script>
    let state = { sessions: [], remotes: [], agents: [] };
    let selectedTaskId = "";
    const agentsEl = document.getElementById("agents");
    const remotesEl = document.getElementById("remotes");
    const sessionsEl = document.getElementById("sessions");
    const timelineEl = document.getElementById("timeline");
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
    function render() { renderAgents(); renderRemotes(); renderSessions(); renderTimeline(); }
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
      sessionsEl.innerHTML = sessions.map(session => '<div class="session '+(session.task_id === selectedTaskId ? "active" : "")+'" data-task-id="'+escapeHtml(session.task_id)+'"><div class="k">task</div><div class="v">'+escapeHtml(session.task_id)+'</div><div class="k" style="margin-top:8px;">root agent</div><div class="v">'+escapeHtml(session.root_agent || "-")+'</div><div class="k" style="margin-top:8px;">status</div><div class="v '+(session.status === "failed" ? "status-failed" : "status-ok")+'">'+escapeHtml(session.status || "-")+'</div></div>').join("");
      document.querySelectorAll(".session").forEach(el => el.addEventListener("click", () => { selectedTaskId = el.dataset.taskId; renderSessions(); renderTimeline(); }));
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
      const header = '<div class="event"><div class="event-top"><div><div class="small">task</div><div class="v">'+escapeHtml(current.task_id)+'</div></div><div class="chip">'+escapeHtml(current.status || "-")+'</div></div><div class="prompt">'+escapeHtml(current.prompt || "")+'</div></div>' + finalResult;
      const events = current.events || [];
      if (!events.length) { timelineEl.innerHTML = header + '<div class="empty" style="margin-top:12px;">这个 session 还没有采集到事件</div>'; return; }
      timelineEl.innerHTML = header + '<div class="events" style="margin-top:12px;">' + events.map(event => '<div class="event"><div class="event-top"><div class="event-stage">'+escapeHtml(event.stage || "-")+'</div><div class="small">'+escapeHtml(event.time || "")+'</div></div><div class="small">agent: '+escapeHtml(event.agent || "-")+(event.mode ? " / mode: " + escapeHtml(event.mode) : "")+'</div>' + (event.target ? '<div class="small">target: '+escapeHtml(event.target)+'</div>' : '') + (event.summary ? '<div class="prompt">'+escapeHtml(event.summary)+'</div>' : '') + (event.error ? '<div class="prompt" style="color:#c2410c;">'+escapeHtml(event.error)+'</div>' : '') + (event.prompt_preview ? '<div class="prompt">'+escapeHtml(event.prompt_preview)+'</div>' : '') + '</div>').join("") + '</div>';
    }
    function escapeHtml(str) { return String(str).replaceAll("&","&amp;").replaceAll("<","&lt;").replaceAll(">","&gt;").replaceAll("\"","&quot;").replaceAll("'","&#39;"); }
    document.getElementById("refreshBtn").addEventListener("click", loadState);
    document.getElementById("startRouterBtn").addEventListener("click", () => startAgent("router"));
    document.getElementById("startCoderBtn").addEventListener("click", () => startAgent("coder"));
    document.getElementById("sendBtn").addEventListener("click", sendMessage);
    loadState();
    setInterval(loadState, 5000);
  </script>
</body>
</html>`
