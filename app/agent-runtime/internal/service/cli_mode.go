package service

import (
	"kratos-demo/internal/consts/public"
	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
)

// PermissionMode controls how risky tool operations are approved in CLI.
type PermissionMode = public.PermissionMode

const (
	PermAsk   = public.PermAsk
	PermAgent = public.PermAgent
	PermAuto  = public.PermAuto
)

func (c *CLIService) permissionMode() PermissionMode {
	if c == nil {
		return PermAsk
	}
	c.permMu.Lock()
	defer c.permMu.Unlock()
	return c.permMode
}

func (c *CLIService) cyclePermissionMode() PermissionMode {
	if c == nil {
		return PermAsk
	}
	c.permMu.Lock()
	c.permMode = c.permMode.Next()
	mode := c.permMode
	c.permMu.Unlock()
	return mode
}

func (c *CLIService) setPermissionMode(mode PermissionMode) {
	if c == nil {
		return
	}
	c.permMu.Lock()
	c.permMode = mode
	c.permMu.Unlock()
}

// resolveRiskApproval returns decided=true when mode makes the choice without UI.
func (c *CLIService) resolveRiskApproval(action agentctx.RiskAction) (decided bool, allowed bool) {
	switch c.permissionMode() {
	case PermAuto:
		return true, true
	case PermAgent:
		if action.Tool == "exec_command" {
			return false, false
		}
		return true, true
	default:
		return false, false
	}
}
