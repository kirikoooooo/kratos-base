// Package traceexport provides adapters for exporting kratos-base agent traces
// to the JSONL format consumed by the ragas-eval Python evaluation framework.
//
// Usage from the agent runtime or CLI:
//
//	exporter := traceexport.NewExporter(traceStore)
//	// Export a single session
//	exporter.ExportSession(ctx, taskID, os.Stdout)
//	// Export all recent sessions to a file
//	exporter.ExportAll(ctx, ".myagent/rag/evaluations/traces.jsonl")
package traceexport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	datatrace "kratos-demo/internal/data/trace"
)

// ToolCallRecord mirrors the Python ToolCallRecord for JSON serialization.
type toolCallRecord struct {
	ToolName   string  `json:"tool_name"`
	Input      string  `json:"input"`
	Output     *string `json:"output,omitempty"`
	Error      *string `json:"error,omitempty"`
	ExitCode   *int    `json:"exit_code,omitempty"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	StepIndex  int     `json:"step_index"`
}

// agentTrace mirrors the Python AgentTrace for JSON serialization.
type agentTrace struct {
	TaskID        string           `json:"task_id"`
	UserQuery     string           `json:"user_query"`
	ToolCalls     []toolCallRecord `json:"tool_calls"`
	FinalResponse *string          `json:"final_response,omitempty"`
	Metadata      map[string]any   `json:"metadata,omitempty"`
}

// Exporter converts DelegationSession data to the evaluation JSONL format.
type Exporter struct {
	store datatrace.DelegationTraceStore
}

// NewExporter creates a new trace exporter.
func NewExporter(store datatrace.DelegationTraceStore) *Exporter {
	return &Exporter{store: store}
}

// ExportSession writes a single session as one JSONL line.
// The userQuery should be the original user prompt that triggered the task.
func (e *Exporter) ExportSession(taskID string, userQuery string) (*agentTrace, error) {
	sessions := e.store.ListSessions(128)
	var target *datatrace.DelegationSession
	for i := range sessions {
		if sessions[i].TaskID == taskID {
			target = &sessions[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("traceexport: session not found: %s", taskID)
	}

	trace := convertSession(target, userQuery)
	return trace, nil
}

// ExportSessionJSONL writes a single session as one JSONL line to a writer.
func (e *Exporter) ExportSessionJSONL(taskID string, userQuery string, w interface{ Write([]byte) (int, error) }) error {
	trace, err := e.ExportSession(taskID, userQuery)
	if err != nil {
		return err
	}
	data, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("traceexport: marshal trace: %w", err)
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// ExportAll writes all recent sessions to a JSONL file, one per line.
// Only sessions with tool call events are included.
func (e *Exporter) ExportAll(outputPath string, userQueryByTaskID map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("traceexport: create output dir: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("traceexport: create output file: %w", err)
	}
	defer f.Close()

	sessions := e.store.ListSessions(128)
	exported := 0
	for _, session := range sessions {
		if !hasToolEvents(session.Events) {
			continue
		}

		userQuery := ""
		if userQueryByTaskID != nil {
			userQuery = userQueryByTaskID[session.TaskID]
		}

		trace := convertSession(&session, userQuery)
		data, err := json.Marshal(trace)
		if err != nil {
			continue
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("traceexport: write trace: %w", err)
		}
		exported++
	}

	if exported == 0 {
		fmt.Fprintf(os.Stderr, "traceexport: no sessions with tool events found\n")
	}
	return nil
}

// convertSession converts a DelegationSession to an agentTrace for evaluation.
func convertSession(session *datatrace.DelegationSession, userQuery string) *agentTrace {
	toolCalls := extractToolCalls(session.Events)

	finalResponse := ""
	if session.ResultSummary != "" {
		finalResponse = session.ResultSummary
		if session.ResultOutput != "" {
			finalResponse += "\n" + session.ResultOutput
		}
	}

	metadata := map[string]any{
		"root_agent":   session.RootAgent,
		"status":       session.Status,
		"num_events":   len(session.Events),
		"num_tool_calls": len(toolCalls),
		"created_at":   session.CreatedAt,
		"error":        session.Error,
	}

	return &agentTrace{
		TaskID:        session.TaskID,
		UserQuery:     userQuery,
		ToolCalls:     toolCalls,
		FinalResponse: strPtr(finalResponse),
		Metadata:      metadata,
	}
}

// extractToolCalls filters DelegationEvents for tool-related stages
// and converts them to toolCallRecord.
func extractToolCalls(events []datatrace.DelegationEvent) []toolCallRecord {
	var calls []toolCallRecord
	for _, event := range events {
		if !isToolStage(event.Stage) {
			continue
		}

		call := toolCallRecord{
			ToolName:  event.ToolName,
			Input:     strings.TrimSpace(event.ToolInput),
			StepIndex: len(calls),
		}

		if event.ToolOutput != "" {
			call.Output = strPtr(strings.TrimSpace(event.ToolOutput))
		}
		if event.Error != "" {
			call.Error = strPtr(event.Error)
		}
		if event.ExitCode != 0 {
			call.ExitCode = intPtr(event.ExitCode)
		}
		if event.DurationMS > 0 {
			call.DurationMS = int64Ptr(event.DurationMS)
		}

		calls = append(calls, call)
	}
	return calls
}

// isToolStage returns true if the stage represents a tool call execution.
func isToolStage(stage string) bool {
	return strings.HasPrefix(stage, "tool_")
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(v int) *int {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

// hasToolEvents checks if any events in the slice are tool-related.
func hasToolEvents(events []datatrace.DelegationEvent) bool {
	for _, event := range events {
		if isToolStage(event.Stage) {
			return true
		}
	}
	return false
}

// ExportToFile is a convenience function that exports all sessions
// that have tool events to the standard evaluation traces path.
//
// Usage:
//
//	traceexport.ExportToFile(ctx, traceStore, ".myagent/rag/evaluations/traces.jsonl", queryMap)
func ExportToFile(store datatrace.DelegationTraceStore, outputPath string, userQueryByTaskID map[string]string) error {
	exporter := NewExporter(store)
	return exporter.ExportAll(outputPath, userQueryByTaskID)
}
