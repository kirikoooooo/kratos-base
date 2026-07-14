package context

import (
	"fmt"
	"math"
	"time"

	datatrace "kratos-demo/internal/data/trace"
)

func NewUsageSnapshot(stats CompressStats) datatrace.ContextUsageSnapshot {
	threshold := stats.Threshold
	if threshold < 0 {
		threshold = 0
	}
	percent := 0.0
	if threshold > 0 {
		percent = math.Min(100, float64(stats.EstimatedChars)/float64(threshold)*100)
	}
	return datatrace.ContextUsageSnapshot{
		EstimatedChars: stats.EstimatedChars,
		Threshold:      threshold,
		UsagePercent:   percent,
		NeedsCompress:  stats.NeedsCompress,
		UpdatedAt:      time.Now(),
	}
}

func FormatCompressEventSummary(result datatrace.ContextCompressResult) string {
	return fmt.Sprintf(
		"context compressed: %d -> %d chars; omitted_turns=%d truncated_tools=%d",
		result.OriginalChars,
		result.CompressedChars,
		result.OmittedTurns,
		result.TruncatedTools,
	)
}

func FormatCompressEventOutput(result datatrace.ContextCompressResult, threshold int) string {
	return fmt.Sprintf(
		"original_chars: %d\ncompressed_chars: %d\nthreshold: %d\nomitted_turns: %d\ntruncated_tools: %d",
		result.OriginalChars,
		result.CompressedChars,
		threshold,
		result.OmittedTurns,
		result.TruncatedTools,
	)
}
