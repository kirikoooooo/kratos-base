package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AgentMemoryStore 持久化用户级提示词记忆、会话级提示词记忆与会话对话历史。
type AgentMemoryStore interface {
	LoadUser(ctx context.Context, userID string) (*UserAgentMemory, error)
	SaveUser(ctx context.Context, memory *UserAgentMemory) error
	LoadSession(ctx context.Context, sessionID string) (*SessionAgentMemory, error)
	SaveSession(ctx context.Context, memory *SessionAgentMemory) error
	LoadConversation(ctx context.Context, sessionID string) (*SessionConversation, error)
	SaveConversation(ctx context.Context, memory *SessionConversation) error
	AppendSessionError(ctx context.Context, record SessionErrorRecord) error
	ListSessionErrors(ctx context.Context, sessionID string, limit int) ([]SessionErrorRecord, error)
}

// ConversationRole 对话轮次角色（与 LLM 消息角色对应，不含完整 trace 事件）。
type ConversationRole string

const (
	ConversationRoleHuman ConversationRole = "human"
	ConversationRoleAI    ConversationRole = "ai"
	ConversationRoleTool  ConversationRole = "tool"
)

// ConversationToolCall 助手发起的工具调用快照。
type ConversationToolCall struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// ConversationTurn 单轮对话记录（持久化到 memory，不写入 trace）。
type ConversationTurn struct {
	Role       ConversationRole       `json:"role"`
	Content    string                 `json:"content,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolCalls  []ConversationToolCall `json:"tool_calls,omitempty"`
}

// SessionConversation 会话级对话历史（与 task_id / session_id 对齐）。
type SessionConversation struct {
	SessionID string             `json:"session_id"`
	Agent     string             `json:"agent,omitempty"`
	Turns     []ConversationTurn `json:"turns,omitempty"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// CommandPolicy 描述在何种情况下应优先执行哪些命令/工具策略。
type CommandPolicy struct {
	Situation string   `json:"situation"`
	Commands  []string `json:"commands,omitempty"`
	Notes     string   `json:"notes,omitempty"`
}

// ToolHint 项目可用工具及使用时机。
type ToolHint struct {
	Name        string `json:"name"`
	WhenToUse   string `json:"when_to_use"`
	Constraints string `json:"constraints,omitempty"`
}

// SkillHint 项目 skill 及触发场景（用于提示词，不是对话记录）。
type SkillHint struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	WhenToUse   string `json:"when_to_use"`
	Description string `json:"description,omitempty"`
}

// PromptAdjustment 面向 LLM 的额外提示片段。
type PromptAdjustment struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// UserAgentMemory 用户级记忆：跨 session 的项目约定、工具与 skill 认知。
type UserAgentMemory struct {
	UserID          string             `json:"user_id"`
	WorkspaceRoot   string             `json:"workspace_root,omitempty"`
	ToolHints       []ToolHint         `json:"tool_hints,omitempty"`
	SkillHints      []SkillHint        `json:"skill_hints,omitempty"`
	CommandPolicies []CommandPolicy    `json:"command_policies,omitempty"`
	PromptNotes     []PromptAdjustment `json:"prompt_notes,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// SessionAgentMemory 会话级记忆：当前任务上下文下的提示词微调（不含对话 turns）。
type SessionAgentMemory struct {
	SessionID       string             `json:"session_id"`
	Agent           string             `json:"agent,omitempty"`
	ToolHints       []ToolHint         `json:"tool_hints,omitempty"`
	CommandPolicies []CommandPolicy    `json:"command_policies,omitempty"`
	PromptNotes     []PromptAdjustment `json:"prompt_notes,omitempty"`
	ToolsUsed       []string           `json:"tools_used,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// AgentMemoryConfig 记忆存储配置（由 data 层从 conf 映射）。
type AgentMemoryConfig struct {
	Dir    string
	UserID string
	// 会话对话上下文压缩（按 session_id 对 conversation turns 生效）。
	ContextCompressThreshold int
	KeepRecentTurns          int
	ToolOutputMaxChars       int
}

func (c AgentMemoryConfig) contextCompressConfig() ContextCompressConfig {
	return ContextCompressConfig{
		Threshold:          c.ContextCompressThreshold,
		KeepRecentTurns:    c.KeepRecentTurns,
		ToolOutputMaxChars: c.ToolOutputMaxChars,
	}
}

func (c AgentMemoryConfig) normalizedUserID() string {
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		return "default"
	}
	return userID
}

// AgentMemoryUsecase 自顶向下编排记忆加载、引导与提示词拼装。
type AgentMemoryUsecase struct {
	store  AgentMemoryStore
	config AgentMemoryConfig
}

func NewAgentMemoryUsecase(store AgentMemoryStore, config AgentMemoryConfig) *AgentMemoryUsecase {
	return &AgentMemoryUsecase{
		store:  store,
		config: config,
	}
}

func (uc *AgentMemoryUsecase) UserID() string {
	if uc == nil {
		return "default"
	}
	return uc.config.normalizedUserID()
}

// StartConversation 初始化或加载会话对话；首轮用户输入写入 conversation 文件。
func (uc *AgentMemoryUsecase) StartConversation(ctx context.Context, sessionID string, agent TaskAgent, initialPrompt string) error {
	if uc == nil || uc.store == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}

	conv, err := uc.store.LoadConversation(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load conversation: %w", err)
	}
	if conv == nil {
		conv = &SessionConversation{SessionID: sessionID}
	}
	conv.SessionID = sessionID
	conv.Agent = agent.String()
	initialPrompt = strings.TrimSpace(initialPrompt)
	conv.Turns = AppendHumanTurnIfNeeded(conv.Turns, initialPrompt)
	conv.UpdatedAt = time.Now()
	if err := uc.store.SaveConversation(ctx, conv); err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	return uc.PrepareForTask(ctx, sessionID, agent)
}

// AppendHumanTurnIfNeeded 在会话末尾追加用户消息（与上一条 human 内容不同才追加，支持连续对话）。
func AppendHumanTurnIfNeeded(turns []ConversationTurn, prompt string) []ConversationTurn {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return turns
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role != ConversationRoleHuman {
			continue
		}
		if strings.TrimSpace(turns[i].Content) == prompt {
			return turns
		}
		break
	}
	return append(turns, ConversationTurn{
		Role:    ConversationRoleHuman,
		Content: prompt,
	})
}

// LoadConversation 读取会话对话历史。
func (uc *AgentMemoryUsecase) LoadConversation(ctx context.Context, sessionID string) (*SessionConversation, error) {
	if uc == nil || uc.store == nil {
		return nil, errors.New("agent memory usecase is not available")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	conv, err := uc.store.LoadConversation(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return &SessionConversation{SessionID: sessionID}, nil
	}
	return conv, nil
}

// SaveConversation 持久化完整会话对话。
func (uc *AgentMemoryUsecase) SaveConversation(ctx context.Context, conv *SessionConversation) error {
	if uc == nil || uc.store == nil {
		return nil
	}
	if conv == nil {
		return errors.New("conversation is nil")
	}
	conv.SessionID = strings.TrimSpace(conv.SessionID)
	if conv.SessionID == "" {
		return errors.New("session id is required")
	}
	conv.UpdatedAt = time.Now()
	return uc.store.SaveConversation(ctx, conv)
}

// ConversationContextStats 统计指定 session 的对话上下文体积。
func (uc *AgentMemoryUsecase) ConversationContextStats(ctx context.Context, sessionID string, turns []ConversationTurn) ContextCompressStats {
	if uc == nil {
		return ContextCompressStats{}
	}
	_ = ctx
	_ = sessionID
	return AnalyzeConversationContext(turns, uc.config.contextCompressConfig())
}

// PrepareTurnsForLLM 按 session 配置统计并压缩对话 turns，供 LLM 使用（不修改持久化全量历史）。
func (uc *AgentMemoryUsecase) PrepareTurnsForLLM(ctx context.Context, sessionID string, turns []ConversationTurn) []ConversationTurn {
	prepared, _ := uc.PrepareTurnsForLLMWithMeta(ctx, sessionID, turns)
	return prepared
}

// PrepareTurnsForLLMWithMeta 与 PrepareTurnsForLLM 相同，并返回统计/压缩元数据供 trace 展示。
func (uc *AgentMemoryUsecase) PrepareTurnsForLLMWithMeta(ctx context.Context, sessionID string, turns []ConversationTurn) ([]ConversationTurn, ContextPrepareMeta) {
	meta := ContextPrepareMeta{}
	if uc == nil || len(turns) == 0 {
		if len(turns) > 0 {
			meta.Stats = ContextCompressStats{EstimatedChars: EstimateConversationContextSize(turns)}
		}
		return turns, meta
	}
	cfg := uc.config.contextCompressConfig()
	meta.Stats = AnalyzeConversationContext(turns, cfg)
	if cfg.disabled() || !meta.Stats.NeedsCompress {
		return turns, meta
	}
	compressed, compressResult := CompressConversationTurns(turns, cfg)
	meta.Compress = &compressResult
	return compressed, meta
}

// ConversationContextUsage 基于持久化对话或传入 turns 统计 session 上下文用量。
func (uc *AgentMemoryUsecase) ConversationContextUsage(ctx context.Context, sessionID string, turns []ConversationTurn) ContextUsageSnapshot {
	if uc == nil {
		return ContextUsageSnapshot{}
	}
	if len(turns) == 0 {
		conv, err := uc.LoadConversation(ctx, sessionID)
		if err != nil || conv == nil {
			return ContextUsageSnapshot{}
		}
		turns = conv.Turns
	}
	return NewContextUsageSnapshot(uc.ConversationContextStats(ctx, sessionID, turns))
}

// ConversationPreview 返回会话首条用户输入摘要，供 trace/dashboard 展示。
func (uc *AgentMemoryUsecase) ConversationPreview(ctx context.Context, sessionID string) string {
	conv, err := uc.LoadConversation(ctx, sessionID)
	if err != nil || conv == nil {
		return ""
	}
	for _, turn := range conv.Turns {
		if turn.Role != ConversationRoleHuman {
			continue
		}
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			continue
		}
		return previewConversationText(content)
	}
	return ""
}

func previewConversationText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	const maxLen = 240
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// PrepareForTask 在任务开始前确保用户/会话提示词记忆文件存在并可被加载。
func (uc *AgentMemoryUsecase) PrepareForTask(ctx context.Context, sessionID string, agent TaskAgent) error {
	if uc == nil || uc.store == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}

	user, err := uc.store.LoadUser(ctx, uc.UserID())
	if err != nil {
		return fmt.Errorf("load user memory: %w", err)
	}
	if user == nil {
		user = &UserAgentMemory{UserID: uc.UserID()}
	}
	user.UserID = uc.UserID()
	user.UpdatedAt = time.Now()
	if err := uc.store.SaveUser(ctx, user); err != nil {
		return fmt.Errorf("save user memory: %w", err)
	}

	session, err := uc.store.LoadSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session memory: %w", err)
	}
	if session == nil {
		session = &SessionAgentMemory{SessionID: sessionID}
	}
	session.SessionID = sessionID
	session.Agent = agent.String()
	session.UpdatedAt = time.Now()
	if err := uc.store.SaveSession(ctx, session); err != nil {
		return fmt.Errorf("save session memory: %w", err)
	}
	return nil
}

// RenderPromptContext 将用户级 + 会话级记忆格式化为可注入 system prompt 的文本。
func (uc *AgentMemoryUsecase) RenderPromptContext(ctx context.Context, sessionID string) string {
	if uc == nil || uc.store == nil {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	user, err := uc.store.LoadUser(ctx, uc.UserID())
	if err != nil {
		return ""
	}
	var session *SessionAgentMemory
	if sessionID != "" {
		session, _ = uc.store.LoadSession(ctx, sessionID)
	}
	block := FormatAgentMemoryForPrompt(user, session)
	if sessionID != "" {
		if records, err := uc.store.ListSessionErrors(ctx, sessionID, defaultSessionErrorsInPrompt); err == nil {
			if errBlock := FormatSessionErrorsForPrompt(records, uc.config.Dir, sessionID); errBlock != "" {
				if block != "" {
					block += "\n\n"
				}
				block += errBlock
			}
		}
	}
	return block
}

// RecordSessionError 追加一条会话错误到持久化日志（JSONL）。
func (uc *AgentMemoryUsecase) RecordSessionError(ctx context.Context, record SessionErrorRecord) error {
	if uc == nil || uc.store == nil {
		return nil
	}
	record.SessionID = strings.TrimSpace(record.SessionID)
	if record.SessionID == "" {
		return nil
	}
	if record.Time.IsZero() {
		record.Time = time.Now()
	}
	record.Message = truncateSessionErrorText(record.Message, maxSessionErrorMessage)
	record.Detail = truncateSessionErrorText(record.Detail, maxSessionErrorDetail)
	return uc.store.AppendSessionError(ctx, record)
}

// RecordSessionToolUsage 记录本会话已使用的工具名（用于提示词微调，非对话历史）。
func (uc *AgentMemoryUsecase) RecordSessionToolUsage(ctx context.Context, sessionID, toolName string) error {
	if uc == nil || uc.store == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	toolName = strings.TrimSpace(toolName)
	if sessionID == "" || toolName == "" {
		return nil
	}

	session, err := uc.store.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		session = &SessionAgentMemory{SessionID: sessionID}
	}
	for _, existing := range session.ToolsUsed {
		if existing == toolName {
			session.UpdatedAt = time.Now()
			return uc.store.SaveSession(ctx, session)
		}
	}
	session.ToolsUsed = append(session.ToolsUsed, toolName)
	session.UpdatedAt = time.Now()
	return uc.store.SaveSession(ctx, session)
}

// UpsertUserMemory 整体更新用户级记忆（供引导或管理接口使用）。
func (uc *AgentMemoryUsecase) UpsertUserMemory(ctx context.Context, memory *UserAgentMemory) error {
	if uc == nil || uc.store == nil {
		return errors.New("agent memory usecase is not available")
	}
	if memory == nil {
		return errors.New("user memory is nil")
	}
	memory.UserID = uc.UserID()
	memory.UpdatedAt = time.Now()
	return uc.store.SaveUser(ctx, memory)
}

// FormatAgentMemoryForPrompt 将两级记忆渲染为 LLM 可读的结构化提示（纯函数，便于测试）。
func FormatAgentMemoryForPrompt(user *UserAgentMemory, session *SessionAgentMemory) string {
	var sections []string

	if user != nil {
		if block := formatUserMemorySection(user); block != "" {
			sections = append(sections, block)
		}
	}
	if session != nil {
		if block := formatSessionMemorySection(session); block != "" {
			sections = append(sections, block)
		}
	}
	if len(sections) == 0 {
		return ""
	}
	return strings.Join(sections, "\n\n")
}

func formatUserMemorySection(user *UserAgentMemory) string {
	var parts []string
	parts = append(parts, "### 用户级记忆（跨任务）")
	if root := strings.TrimSpace(user.WorkspaceRoot); root != "" {
		parts = append(parts, "- 工作区: "+root)
	}
	parts = append(parts, formatToolHints(user.ToolHints)...)
	parts = append(parts, formatSkillHints(user.SkillHints)...)
	parts = append(parts, formatCommandPolicies(user.CommandPolicies)...)
	parts = append(parts, formatPromptNotes(user.PromptNotes)...)
	return joinNonEmptyLines(parts)
}

func formatSessionMemorySection(session *SessionAgentMemory) string {
	var parts []string
	parts = append(parts, "### 会话级记忆（当前任务）")
	if agent := strings.TrimSpace(session.Agent); agent != "" {
		parts = append(parts, "- 当前 agent: "+agent)
	}
	if len(session.ToolsUsed) > 0 {
		parts = append(parts, "- 本会话已使用工具: "+strings.Join(session.ToolsUsed, ", "))
	}
	parts = append(parts, formatToolHints(session.ToolHints)...)
	parts = append(parts, formatCommandPolicies(session.CommandPolicies)...)
	parts = append(parts, formatPromptNotes(session.PromptNotes)...)
	return joinNonEmptyLines(parts)
}

func formatToolHints(hints []ToolHint) []string {
	if len(hints) == 0 {
		return nil
	}
	lines := []string{"- 工具使用策略:"}
	for _, hint := range hints {
		name := strings.TrimSpace(hint.Name)
		when := strings.TrimSpace(hint.WhenToUse)
		if name == "" && when == "" {
			continue
		}
		line := "  - " + name
		if when != "" {
			line += ": " + when
		}
		if c := strings.TrimSpace(hint.Constraints); c != "" {
			line += "（约束: " + c + "）"
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		return nil
	}
	return lines
}

func formatSkillHints(hints []SkillHint) []string {
	if len(hints) == 0 {
		return nil
	}
	lines := []string{"- 项目 Skills（按需参考，非对话历史）:"}
	for _, hint := range hints {
		name := strings.TrimSpace(hint.Name)
		if name == "" {
			continue
		}
		line := "  - " + name
		if p := strings.TrimSpace(hint.Path); p != "" {
			line += " @ " + p
		}
		if w := strings.TrimSpace(hint.WhenToUse); w != "" {
			line += ": " + w
		}
		if d := strings.TrimSpace(hint.Description); d != "" {
			line += " — " + d
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		return nil
	}
	return lines
}

func formatCommandPolicies(policies []CommandPolicy) []string {
	if len(policies) == 0 {
		return nil
	}
	lines := []string{"- 命令/操作政策:"}
	for _, policy := range policies {
		situation := strings.TrimSpace(policy.Situation)
		if situation == "" {
			continue
		}
		line := "  - 场景「" + situation + "」"
		if len(policy.Commands) > 0 {
			line += " → " + strings.Join(policy.Commands, "；")
		}
		if n := strings.TrimSpace(policy.Notes); n != "" {
			line += "（" + n + "）"
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		return nil
	}
	return lines
}

func formatPromptNotes(notes []PromptAdjustment) []string {
	if len(notes) == 0 {
		return nil
	}
	var lines []string
	for _, note := range notes {
		title := strings.TrimSpace(note.Title)
		content := strings.TrimSpace(note.Content)
		if title == "" && content == "" {
			continue
		}
		if title == "" {
			lines = append(lines, "- "+content)
			continue
		}
		lines = append(lines, "- "+title+": "+content)
	}
	return lines
}

func joinNonEmptyLines(lines []string) string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}
