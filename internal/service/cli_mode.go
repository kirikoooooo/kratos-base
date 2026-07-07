package service

import (
	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
)

// PermissionMode controls how risky tool operations are approved in CLI.
type PermissionMode int

const (
	PermAsk PermissionMode = iota
	PermAgent
	PermAuto
)

func (m PermissionMode) String() string {
	switch m {
	case PermAsk:
		return "ask"
	case PermAgent:
		return "agent"
	case PermAuto:
		return "auto"
	default:
		return "ask"
	}
}

func (m PermissionMode) Label() string {
	switch m {
	case PermAsk:
		return "Ask"
	case PermAgent:
		return "Agent"
	case PermAuto:
		return "Auto"
	default:
		return "Ask"
	}
}

func (m PermissionMode) Description() string {
	switch m {
	case PermAsk:
		return "删除文件 / 高风险命令均需确认"
	case PermAgent:
		return "文件操作自动批准，高风险 shell 命令仍需确认"
	case PermAuto:
		return "全部自动批准（无确认弹窗）"
	default:
		return ""
	}
}

func (m PermissionMode) Next() PermissionMode {
	switch m {
	case PermAsk:
		return PermAgent
	case PermAgent:
		return PermAuto
	default:
		return PermAsk
	}
}

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
