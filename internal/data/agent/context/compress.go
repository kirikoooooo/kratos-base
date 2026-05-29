package context
import (
	"fmt"
	"strings"

	datatrace "kratos-demo/internal/data/trace"
)

func estimateConversationContextSize(turns []ConversationTurn) int {
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

func AnalyzeConversationContext(turns []ConversationTurn, cfg CompressConfig) CompressStats {
	if contextCompressDisabled(cfg) {
		size := estimateConversationContextSize(turns)
		return CompressStats{
			EstimatedChars: size,
			Threshold:      -1,
			NeedsCompress:  false,
		}
	}
	cfg = normalizeCompressConfig(cfg)
	size := estimateConversationContextSize(turns)
	return CompressStats{
		EstimatedChars: size,
		Threshold:      cfg.Threshold,
		NeedsCompress:  size > cfg.Threshold,
	}
}

func CompressConversationTurns(turns []ConversationTurn, cfg CompressConfig) ([]ConversationTurn, datatrace.ContextCompressResult) {
	if contextCompressDisabled(cfg) {
		original := estimateConversationContextSize(turns)
		return cloneConversationTurns(turns), datatrace.ContextCompressResult{OriginalChars: original, CompressedChars: original}
	}
	cfg = normalizeCompressConfig(cfg)
	original := estimateConversationContextSize(turns)
	result := datatrace.ContextCompressResult{OriginalChars: original}
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
		if estimateConversationContextSize(working) <= cfg.Threshold {
			result.Compressed = true
			result.CompressedChars = estimateConversationContextSize(working)
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
	result.CompressedChars = estimateConversationContextSize(compressed)
	return compressed, result
}

func contextCompressDisabled(cfg CompressConfig) bool {
	return cfg.Threshold < 0
}

func normalizeCompressConfig(cfg CompressConfig) CompressConfig {
	out := cfg
	if out.Threshold == 0 {
		out.Threshold = DefaultCompressThreshold
	}
	if out.KeepRecentTurns <= 0 {
		out.KeepRecentTurns = DefaultKeepRecentTurns
	}
	if out.ToolOutputMaxChars <= 0 {
		out.ToolOutputMaxChars = DefaultToolOutputMaxChars
	}
	return out
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

func compressBySlidingWindow(turns []ConversationTurn, cfg CompressConfig) ([]ConversationTurn, int) {
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
				estimateConversationContextSize(turns), cfg.Threshold, omitted, keepRecent,
			),
		})
	}

	merged := append(head, tail...)
	if estimateConversationContextSize(merged) > cfg.Threshold {
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

func EstimateConversationContextSize(turns []ConversationTurn) int {
	return estimateConversationContextSize(turns)
}
