package trace

import (
	"fmt"
	"math"
	"time"
)

func newContextUsageSnapshot(estimatedChars, threshold int, needsCompress bool) ContextUsageSnapshot {
	if threshold < 0 {
		threshold = 0
	}
	percent := 0.0
	if threshold > 0 {
		percent = math.Min(100, float64(estimatedChars)/float64(threshold)*100)
	}
	return ContextUsageSnapshot{
		EstimatedChars: estimatedChars,
		Threshold:      threshold,
		UsagePercent:   percent,
		NeedsCompress:  needsCompress,
		UpdatedAt:      time.Now(),
	}
}

func formatContextCompressEventSummary(result ContextCompressResult) string {
	return fmt.Sprintf(
		"context compressed: %d -> %d chars; omitted_turns=%d truncated_tools=%d",
		result.OriginalChars,
		result.CompressedChars,
		result.OmittedTurns,
		result.TruncatedTools,
	)
}
