package biz

import (
	"fmt"
	"math"
	"time"
)

// ContextUsageSnapshot 会话上下文用量快照（Dashboard / SSE 实时展示）。
type ContextUsageSnapshot struct {
	EstimatedChars      int       `json:"estimated_chars"`
	Threshold           int       `json:"threshold"`
	UsagePercent        float64   `json:"usage_percent"`
	NeedsCompress       bool      `json:"needs_compress"`
	CompressCount       int       `json:"compress_count"`
	LastOriginalChars   int       `json:"last_original_chars,omitempty"`
	LastCompressedChars int       `json:"last_compressed_chars,omitempty"`
	LastOmittedTurns    int       `json:"last_omitted_turns,omitempty"`
	LastTruncatedTools  int       `json:"last_truncated_tools,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// ContextPrepareMeta PrepareTurnsForLLM 的统计与压缩结果。
type ContextPrepareMeta struct {
	Stats    ContextCompressStats
	Compress *ContextCompressResult
}

// NewContextUsageSnapshot 由统计结果生成展示快照。
func NewContextUsageSnapshot(stats ContextCompressStats) ContextUsageSnapshot {
	threshold := stats.Threshold
	if threshold < 0 {
		threshold = 0
	}
	percent := 0.0
	if threshold > 0 {
		percent = math.Min(100, float64(stats.EstimatedChars)/float64(threshold)*100)
	}
	return ContextUsageSnapshot{
		EstimatedChars: stats.EstimatedChars,
		Threshold:      threshold,
		UsagePercent:   percent,
		NeedsCompress:  stats.NeedsCompress,
		UpdatedAt:      time.Now(),
	}
}

// FormatContextCompressEventSummary 生成压缩事件摘要。
func FormatContextCompressEventSummary(result ContextCompressResult) string {
	return fmt.Sprintf(
		"context compressed: %d -> %d chars; omitted_turns=%d truncated_tools=%d",
		result.OriginalChars,
		result.CompressedChars,
		result.OmittedTurns,
		result.TruncatedTools,
	)
}

// FormatContextCompressEventOutput 生成压缩事件详情。
func FormatContextCompressEventOutput(result ContextCompressResult, threshold int) string {
	return fmt.Sprintf(
		"original_chars: %d\ncompressed_chars: %d\nthreshold: %d\nomitted_turns: %d\ntruncated_tools: %d",
		result.OriginalChars,
		result.CompressedChars,
		threshold,
		result.OmittedTurns,
		result.TruncatedTools,
	)
}
