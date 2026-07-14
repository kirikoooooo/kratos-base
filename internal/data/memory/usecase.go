package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	bizconversation "kratos-demo/internal/biz/conversation"
	"kratos-demo/internal/consts/public"
	agentcontext "kratos-demo/internal/data/agent_runtime/context"
	datatrace "kratos-demo/internal/data/trace"
)

type agentMemoryUsecase struct {
	store  AgentMemoryStore
	config AgentMemoryConfig
}

func NewFileAgentMemoryUsecase(store AgentMemoryStore, config AgentMemoryConfig) AgentMemory {
	return &agentMemoryUsecase{
		store:  store,
		config: config,
	}
}

func (uc *agentMemoryUsecase) UserID() string {
	if uc == nil {
		return "default"
	}
	return normalizeMemoryUserID(uc.config.UserID)
}

func (uc *agentMemoryUsecase) StartConversation(ctx context.Context, sessionID string, agent public.AgentKind, initialPrompt string) error {
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
	conv.Agent = string(agent)
	initialPrompt = strings.TrimSpace(initialPrompt)
	conv.Turns = agentcontext.AppendHumanTurnIfNeeded(conv.Turns, initialPrompt)
	conv.UpdatedAt = time.Now()
	if err := uc.store.SaveConversation(ctx, conv); err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	return uc.PrepareForTask(ctx, sessionID, agent)
}

func (uc *agentMemoryUsecase) LoadConversation(ctx context.Context, sessionID string) (*SessionConversation, error) {
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

func (uc *agentMemoryUsecase) SaveConversation(ctx context.Context, conv *SessionConversation) error {
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

func (uc *agentMemoryUsecase) ConversationContextStats(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) agentcontext.CompressStats {
	if uc == nil {
		return agentcontext.CompressStats{}
	}
	_ = ctx
	_ = sessionID
	return agentcontext.AnalyzeConversationContext(turns, compressConfig(uc.config))
}

func (uc *agentMemoryUsecase) PrepareTurnsForLLM(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) []agentcontext.ConversationTurn {
	prepared, _ := uc.PrepareTurnsForLLMWithMeta(ctx, sessionID, turns)
	return prepared
}

func (uc *agentMemoryUsecase) PrepareTurnsForLLMWithMeta(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) ([]agentcontext.ConversationTurn, agentcontext.PrepareMeta) {
	meta := agentcontext.PrepareMeta{}
	if uc == nil || len(turns) == 0 {
		if len(turns) > 0 {
			meta.Stats = agentcontext.CompressStats{EstimatedChars: agentcontext.EstimateConversationContextSize(turns)}
		}
		return turns, meta
	}
	cfg := compressConfig(uc.config)
	meta.Stats = agentcontext.AnalyzeConversationContext(turns, cfg)
	if cfg.Threshold < 0 || !meta.Stats.NeedsCompress {
		return turns, meta
	}
	compressed, compressResult := agentcontext.CompressConversationTurns(turns, cfg)
	convResult := traceToConversationCompressResult(compressResult)
	meta.Compress = &convResult
	return compressed, meta
}

func (uc *agentMemoryUsecase) ConversationContextUsage(ctx context.Context, sessionID string, turns []agentcontext.ConversationTurn) bizconversation.UsageSnapshot {
	if uc == nil {
		return bizconversation.UsageSnapshot{}
	}
	if len(turns) == 0 {
		conv, err := uc.LoadConversation(ctx, sessionID)
		if err != nil || conv == nil {
			return bizconversation.UsageSnapshot{}
		}
		turns = conv.Turns
	}
	snap := agentcontext.NewUsageSnapshot(uc.ConversationContextStats(ctx, sessionID, turns))
	return traceToConversationUsageSnapshot(snap)
}

func (uc *agentMemoryUsecase) ConversationPreview(ctx context.Context, sessionID string) string {
	conv, err := uc.LoadConversation(ctx, sessionID)
	if err != nil || conv == nil {
		return ""
	}
	for _, turn := range conv.Turns {
		if turn.Role != agentcontext.ConversationRoleHuman {
			continue
		}
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			continue
		}
		runes := []rune(content)
		if len(runes) <= 120 {
			return content
		}
		return string(runes[:120]) + "..."
	}
	return ""
}

func (uc *agentMemoryUsecase) PrepareForTask(ctx context.Context, sessionID string, agent public.AgentKind) error {
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
	session, err := uc.store.LoadSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session memory: %w", err)
	}
	if session == nil {
		session = &SessionAgentMemory{SessionID: sessionID}
	}
	session.SessionID = sessionID
	session.Agent = string(agent)
	session.UpdatedAt = time.Now()
	if err := uc.store.SaveSession(ctx, session); err != nil {
		return fmt.Errorf("save session memory: %w", err)
	}
	_ = user
	return nil
}

func (uc *agentMemoryUsecase) RenderPromptContext(ctx context.Context, sessionID string) string {
	if uc == nil || uc.store == nil {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	user, err := uc.store.LoadUser(ctx, uc.UserID())
	if err != nil {
		return ""
	}
	var session *SessionAgentMemory
	if sessionID != "" {
		session, err = uc.store.LoadSession(ctx, sessionID)
		if err != nil {
			return ""
		}
	}
	block := formatAgentMemoryForPrompt(user, session)
	if block == "" {
		return block
	}
	records, err := uc.store.ListSessionErrors(ctx, sessionID, 6)
	if err == nil {
		if errBlock := formatSessionErrorsForPrompt(records, uc.config.Dir, sessionID); errBlock != "" {
			block += "\n\n" + errBlock
		}
	}
	return block
}

func (uc *agentMemoryUsecase) RecordSessionError(ctx context.Context, record SessionErrorRecord) error {
	if uc == nil || uc.store == nil {
		return nil
	}
	if strings.TrimSpace(record.SessionID) == "" {
		return errors.New("session id is required")
	}
	if record.Time.IsZero() {
		record.Time = time.Now()
	}
	return uc.store.AppendSessionError(ctx, record)
}

func (uc *agentMemoryUsecase) RecordSessionToolUsage(ctx context.Context, sessionID, toolName string) error {
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
	for _, used := range session.ToolsUsed {
		if used == toolName {
			return uc.store.SaveSession(ctx, session)
		}
	}
	session.ToolsUsed = append(session.ToolsUsed, toolName)
	session.UpdatedAt = time.Now()
	return uc.store.SaveSession(ctx, session)
}

func (uc *agentMemoryUsecase) UpsertUserMemory(ctx context.Context, memory *UserAgentMemory) error {
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

func normalizeMemoryUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "default"
	}
	return userID
}

func compressConfig(config AgentMemoryConfig) agentcontext.CompressConfig {
	return agentcontext.ConfigFromMemory(
		config.ContextCompressThreshold,
		config.KeepRecentTurns,
		config.ToolOutputMaxChars,
	)
}

// traceToConversationCompressResult converts a datatrace.ContextCompressResult
// to its biz/conversation equivalent.
func traceToConversationCompressResult(r datatrace.ContextCompressResult) bizconversation.CompressResult {
	return bizconversation.CompressResult{
		OriginalChars:   r.OriginalChars,
		CompressedChars: r.CompressedChars,
		Compressed:      r.Compressed,
		OmittedTurns:    r.OmittedTurns,
		TruncatedTools:  r.TruncatedTools,
	}
}

// traceToConversationUsageSnapshot converts a datatrace.ContextUsageSnapshot
// to its biz/conversation equivalent.
func traceToConversationUsageSnapshot(s datatrace.ContextUsageSnapshot) bizconversation.UsageSnapshot {
	return bizconversation.UsageSnapshot{
		EstimatedChars:      s.EstimatedChars,
		Threshold:           s.Threshold,
		UsagePercent:        s.UsagePercent,
		NeedsCompress:       s.NeedsCompress,
		CompressCount:       s.CompressCount,
		LastOriginalChars:   s.LastOriginalChars,
		LastCompressedChars: s.LastCompressedChars,
		LastOmittedTurns:    s.LastOmittedTurns,
		LastTruncatedTools:  s.LastTruncatedTools,
	}
}
