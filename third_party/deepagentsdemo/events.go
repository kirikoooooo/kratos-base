package deepagentsdemo

import (
	"fmt"
	"io"
	"strings"

	deepagent "github.com/denizumutdereli/go-deepagent/pkg/agent"
)

// StreamEvents 将 ReAct 事件流格式输出到 w，便于观察 orchestrator → sub-agent 调用链。
func StreamEvents(w io.Writer, events <-chan deepagent.ReactEvent) {
	for evt := range events {
		switch evt.Type {
		case deepagent.EventIterationStart:
			fmt.Fprintf(w, "\n⟳ [%s] iteration %d\n", evt.Agent, evt.Iteration)
		case deepagent.EventLLMResponse:
			if text := trimPreview(evt.Content, 240); text != "" {
				fmt.Fprintf(w, "💭 [%s] %s\n", evt.Agent, text)
			}
		case deepagent.EventToolStart:
			fmt.Fprintf(w, "🔧 [%s] %s(%s)\n", evt.Agent, evt.ToolName, trimPreview(evt.ToolInput, 120))
		case deepagent.EventToolEnd:
			fmt.Fprintf(w, "✓  [%s] %s → %s\n", evt.Agent, evt.ToolName, trimPreview(evt.Content, 160))
		case deepagent.EventFinalAnswer:
			fmt.Fprintf(w, "\n✅ [%s] final\n", evt.Agent)
		}
	}
}

func trimPreview(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
