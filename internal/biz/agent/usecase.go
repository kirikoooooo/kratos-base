package agent

import agentruntime "kratos-demo/internal/biz/agent_runtime"

// AgentRuntimeUsecase is a backward-compatible alias for [Agent].
// It allows service-layer code that references *AgentRuntimeUsecase to compile
// without changes while the underlying implementation is unified into Agent.
//
// Deprecated: use [Agent] directly. New code should call [NewAgent] instead of
// [NewAgentRuntimeUsecase].
type AgentRuntimeUsecase = Agent

// NewAgentRuntimeUsecase is a backward-compatible constructor that creates an
// [Agent] with only a runtime. The Memory and Tools fields are left nil.
//
// Deprecated: use [NewAgent] with all three bounded-context interfaces.
func NewAgentRuntimeUsecase(runtime agentruntime.AgentRuntime) *AgentRuntimeUsecase {
	return &AgentRuntimeUsecase{Runtime: runtime}
}
