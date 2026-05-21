package biz

import (
	"fmt"
	"strings"
)

const (
	DefaultContextCompressThreshold = 200_000
	DefaultKeepRecentTurns          = 24
	DefaultToolOutputMaxChars       = 8_000
)

// ContextCompressConfig 会话级对话上下文压缩策略（按 session 生效）。
type ContextCompressConfig struct {
	Threshold          int
	KeepRecentTurns    int // 保留的最近对话轮次组数（human 轮或 ai+tool 轮），不是单条 turn 数
	ToolOutputMaxChars int
}

func (c ContextCompressConfig) disabled() bool {
	return c.Threshold < 0
}

func (c ContextCompressConfig) normalized() ContextCompressConfig {
	out := c
	if out.Threshold == 0 {
		out.Threshold = DefaultContextCompressThreshold
	}
	if out.KeepRecentTurns <= 0 {
		out.KeepRecentTurns = DefaultKeepRecentTurns
	}
	if out.ToolOutputMaxChars <= 0 {
		out.ToolOutputMaxChars = DefaultToolOutputMaxChars
	}
	return out
}

// ContextCompressStats 会话上下文体积统计。
type ContextCompressStats struct {
	EstimatedChars int
	Threshold      int
	NeedsCompress  bool
}

// ContextCompressResult 压缩执行结果（仅影响发给 LLM 的视图，不改变持久化全量 turns 的约定）。
type ContextCompressResult struct {
	OriginalChars   int
	CompressedChars int
	Compressed      bool
	OmittedTurns    int
	TruncatedTools  int
}

// EstimateConversationContextSize 统计会话 turns 的近似上下文字符数（用于与阈值比较）。
func EstimateConversationContextSize(turns []ConversationTurn) int {
	total := 0
	for _, turn := range turns {
		total += len(strings.TrimSpace(turn.Content))
		total += len(strings.TrimSpace(turn.ToolCallID))
		total += len(strings.TrimSpace(turn.ToolName))
		for _, call := range turn.ToolCalls {
			total += len(strings.TrimSpace(call.ID))
			total += len(strings.TrimSpace(call.Name))
			total += len(strings.TrimSpace(call.Arguments))
		}
	}
	return total
}

// AnalyzeConversationContext 判断当前会话上下文是否超过配置阈值。
func AnalyzeConversationContext(turns []ConversationTurn, cfg ContextCompressConfig) ContextCompressStats {
	if cfg.disabled() {
		size := EstimateConversationContextSize(turns)
		return ContextCompressStats{
			EstimatedChars: size,
			Threshold:      -1,
			NeedsCompress:  false,
		}
	}
	cfg = cfg.normalized()
	size := EstimateConversationContextSize(turns)
	return ContextCompressStats{
		EstimatedChars: size,
		Threshold:      cfg.Threshold,
		NeedsCompress:  size > cfg.Threshold,
	}
}

// CompressConversationTurns 在超过阈值时压缩 turns（保留首条用户消息 + 最近若干轮，并截断 tool 输出）。
func CompressConversationTurns(turns []ConversationTurn, cfg ContextCompressConfig) ([]ConversationTurn, ContextCompressResult) {
	if cfg.disabled() {
		original := EstimateConversationContextSize(turns)
		return cloneConversationTurns(turns), ContextCompressResult{OriginalChars: original, CompressedChars: original}
	}
	cfg = cfg.normalized()
	original := EstimateConversationContextSize(turns)
	result := ContextCompressResult{OriginalChars: original}
	if len(turns) == 0 || original <= cfg.Threshold {
		result.CompressedChars = original
		return cloneConversationTurns(turns), result
	}

	working := cloneConversationTurns(turns)
	toolMax := cfg.ToolOutputMaxChars
	for toolMax > 0 {
		var truncatedTools int
		working, truncatedTools = truncateToolTurnContents(cloneConversationTurns(working), toolMax)
		result.TruncatedTools += truncatedTools
		if EstimateConversationContextSize(working) <= cfg.Threshold {
			result.Compressed = true
			result.CompressedChars = EstimateConversationContextSize(working)
			return working, result
		}
		if toolMax <= 256 {
			break
		}
		toolMax /= 2
	}

	compressed, omitted := compressBySlidingWindow(working, cfg)
	result.Compressed = true
	result.OmittedTurns = omitted
	result.CompressedChars = EstimateConversationContextSize(compressed)
	return compressed, result
}

func truncateToolTurnContents(turns []ConversationTurn, maxChars int) ([]ConversationTurn, int) {
	if maxChars <= 0 {
		return turns, 0
	}
	truncated := 0
	for i := range turns {
		if turns[i].Role != ConversationRoleTool {
			continue
		}
		content := turns[i].Content
		if len(content) <= maxChars {
			continue
		}
		turns[i].Content = content[:maxChars] + fmt.Sprintf("\n...[tool output truncated, %d chars omitted]", len(content)-maxChars)
		truncated++
	}
	return turns, truncated
}

// splitConversationRounds 按对话轮次分组，保证 ai+tool 不会被拆开。
func splitConversationRounds(turns []ConversationTurn) [][]ConversationTurn {
	if len(turns) == 0 {
		return nil
	}
	rounds := make([][]ConversationTurn, 0, len(turns))
	for i := 0; i < len(turns); {
		switch turns[i].Role {
		case ConversationRoleHuman:
			start := i
			for i < len(turns) && turns[i].Role == ConversationRoleHuman {
				i++
			}
			rounds = append(rounds, append([]ConversationTurn(nil), turns[start:i]...))
		case ConversationRoleAI:
			start := i
			i++
			for i < len(turns) && turns[i].Role == ConversationRoleTool {
				i++
			}
			rounds = append(rounds, append([]ConversationTurn(nil), turns[start:i]...))
		case ConversationRoleTool:
			start := i
			for i < len(turns) && turns[i].Role == ConversationRoleTool {
				i++
			}
			rounds = append(rounds, append([]ConversationTurn(nil), turns[start:i]...))
		default:
			i++
		}
	}
	return rounds
}

func flattenConversationRounds(rounds [][]ConversationTurn) []ConversationTurn {
	if len(rounds) == 0 {
		return nil
	}
	total := 0
	for _, round := range rounds {
		total += len(round)
	}
	flat := make([]ConversationTurn, 0, total)
	for _, round := range rounds {
		flat = append(flat, round...)
	}
	return flat
}

func countTurnsInRounds(rounds [][]ConversationTurn) int {
	total := 0
	for _, round := range rounds {
		total += len(round)
	}
	return total
}

func firstHumanTurn(turns []ConversationTurn) (ConversationTurn, bool) {
	for _, turn := range turns {
		if turn.Role == ConversationRoleHuman && strings.TrimSpace(turn.Content) != "" {
			return turn, true
		}
	}
	return ConversationTurn{}, false
}

func compressBySlidingWindow(turns []ConversationTurn, cfg ContextCompressConfig) ([]ConversationTurn, int) {
	rounds := splitConversationRounds(turns)
	if len(rounds) == 0 {
		return turns, 0
	}

	keepRecent := cfg.KeepRecentTurns
	if keepRecent > len(rounds) {
		keepRecent = len(rounds)
	}
	omittedRounds := rounds[:len(rounds)-keepRecent]
	tail := flattenConversationRounds(rounds[len(rounds)-keepRecent:])
	omitted := countTurnsInRounds(omittedRounds)

	var head []ConversationTurn
	if first, ok := firstHumanTurn(turns); ok {
		head = append(head, first)
	}
	if omitted > 0 {
		head = append(head, ConversationTurn{
			Role: ConversationRoleHuman,
			Content: fmt.Sprintf(
				"[session context compressed] estimated_chars=%d threshold=%d omitted_turns=%d; kept first user message and last %d round groups.",
				EstimateConversationContextSize(turns), cfg.Threshold, omitted, keepRecent,
			),
		})
	}

	merged := append(head, tail...)
	if EstimateConversationContextSize(merged) > cfg.Threshold {
		merged = tail
		if first, ok := firstHumanTurn(turns); ok {
			merged = append([]ConversationTurn{first, {
				Role:    ConversationRoleHuman,
				Content: "[session context compressed] only recent round groups kept due to size limit.",
			}}, tail...)
		}
	}
	return merged, omitted
}

func cloneConversationTurns(turns []ConversationTurn) []ConversationTurn {
	if len(turns) == 0 {
		return nil
	}
	cloned := make([]ConversationTurn, len(turns))
	for i, turn := range turns {
		cloned[i] = turn
		if len(turn.ToolCalls) > 0 {
			cloned[i].ToolCalls = append([]ConversationToolCall(nil), turn.ToolCalls...)
		}
	}
	return cloned
}
