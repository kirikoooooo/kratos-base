package tool

import (
	"context"
	"fmt"
	"strings"

	errconst "kratos-demo/internal/consts/error"
	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
)

var ErrRiskApprovalDenied = errconst.ErrRiskApprovalDenied

func requireRiskApproval(ctx context.Context, action agentctx.RiskAction) error {
	action.Tool = strings.TrimSpace(action.Tool)
	action.Summary = strings.TrimSpace(action.Summary)
	action.Detail = strings.TrimSpace(action.Detail)
	if action.Summary == "" {
		action.Summary = action.Tool
	}

	approver := agentctx.RiskApproverFrom(ctx)
	if approver == nil {
		return fmt.Errorf(
			"高风险操作需要人工确认（%s）。请使用 `kratos-demo -cli` 交互模式，在终端输入 y 批准后再执行",
			action.Summary,
		)
	}

	allowed, err := approver(ctx, action)
	if err != nil {
		return fmt.Errorf("risk approval failed: %w", err)
	}
	if !allowed {
		return fmt.Errorf("%w: %s", ErrRiskApprovalDenied, action.Summary)
	}
	return nil
}

func classifyExecCommandRisk(command string) (bool, string) {
	command = strings.TrimSpace(strings.ToLower(command))
	if command == "" {
		return false, ""
	}

	patterns := []struct {
		needle string
		reason string
	}{
		{"rm ", "删除文件/目录 (rm)"},
		{"rmdir ", "删除目录 (rmdir)"},
		{"del ", "删除文件 (del)"},
		{"erase ", "删除文件 (erase)"},
		{"remove-item", "删除 (Remove-Item)"},
		{"format ", "格式化磁盘 (format)"},
		{"shutdown ", "关机 (shutdown)"},
		{"restart-computer", "重启 (Restart-Computer)"},
		{"stop-computer", "关机 (Stop-Computer)"},
		{"git reset", "Git 重置 (git reset)"},
		{"git checkout --", "Git 丢弃变更 (git checkout --)"},
		{"git clean", "Git 清理未跟踪文件 (git clean)"},
	}

	for _, item := range patterns {
		if strings.Contains(command, item.needle) {
			return true, item.reason
		}
	}
	return false, ""
}
