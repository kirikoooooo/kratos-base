package agent

// AgentKind identifies a specific agent role/capability (e.g. coder, reviewer).
type AgentKind string

const (
	AgentKindDefault  AgentKind = "default"
	AgentKindGeneric  AgentKind = "generic"
	AgentKindRouter   AgentKind = "router"
	AgentKindCoder    AgentKind = "coder"
	AgentKindReviewer AgentKind = "reviewer"
)
