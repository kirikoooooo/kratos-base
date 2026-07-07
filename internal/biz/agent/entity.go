package agent

// Agent identifies a specific agent type/capability.
type Agent string

const (
	AgentDefault  Agent = "default"
	AgentGeneric  Agent = "generic"
	AgentRouter   Agent = "router"
	AgentCoder    Agent = "coder"
	AgentReviewer Agent = "reviewer"
)
