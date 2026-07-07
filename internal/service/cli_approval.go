package service

import (
	"context"
	"fmt"
	"strings"

	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
)

func (c *CLIService) riskApprover(ctx context.Context, action agentctx.RiskAction) (bool, error) {
	if c == nil {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if decided, allowed := c.resolveRiskApproval(action); decided {
		return allowed, nil
	}

	if c.approvalBridge != nil {
		resp := make(chan approvalResponse, 1)
		select {
		case c.approvalBridge <- approvalRequest{action: action, resp: resp}:
		case <-ctx.Done():
			return false, ctx.Err()
		}
		r := <-resp
		return r.allowed, r.err
	}

	return false, fmt.Errorf("高风险操作需要 CLI 交互确认，请使用 kratos-demo -cli 运行")
}

func (c *CLIService) handleApprovalRequest(action agentctx.RiskAction) (bool, error) {
	if decided, allowed := c.resolveRiskApproval(action); decided {
		if allowed && c.ui != nil {
			c.ui.println(c.ui.dim("  ✓  已自动批准（" + c.permissionMode().String() + "）"))
		}
		return allowed, nil
	}
	return c.runApprovalUI(action)
}

func (c *CLIService) runApprovalUI(action agentctx.RiskAction) (bool, error) {
	if c == nil || c.ui == nil {
		return false, nil
	}

	if c.lineReader != nil {
		c.lineReader.PauseForOverlay()
		defer c.lineReader.ResumeAfterOverlay()
	}

	c.ui.println("")
	c.ui.println(c.ui.yellow("  ⚠  需要您的确认"))
	c.ui.println(c.ui.bold("  操作  ") + strings.TrimSpace(action.Tool))
	if summary := strings.TrimSpace(action.Summary); summary != "" {
		c.ui.println(c.ui.dim("  摘要  ") + summary)
	}
	if detail := strings.TrimSpace(action.Detail); detail != "" {
		for _, line := range strings.Split(detail, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			c.ui.println(c.ui.dim("        ") + line)
		}
	}

	options := []selectOption{
		{Label: "允许执行", Value: true},
		{Label: "拒绝", Value: false},
	}
	allowed, err := promptArrowSelect(c.out, c.in, c.ui.paint, options, 1)
	if err != nil {
		return false, err
	}
	if allowed {
		c.ui.println(c.ui.green("  ✓  已批准"))
	} else {
		c.ui.println(c.ui.red("  ✕  已拒绝"))
	}
	return allowed, nil
}

func (c *CLIService) contextWithApproval(ctx context.Context) context.Context {
	if c == nil {
		return ctx
	}
	return agentctx.WithRiskApprover(ctx, c.riskApprover)
}
