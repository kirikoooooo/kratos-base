package data

import (
	"strings"

	"kratos-demo/internal/biz"
)

func initialPlan(agent biz.TaskAgent, prompt string) []biz.PlanStep {
	steps := []biz.PlanStep{
		{
			ID:          "understand",
			Title:       "理解任务",
			Status:      "in_progress",
			Description: "解析用户目标和约束",
		},
		{
			ID:          "inspect",
			Title:       "检查上下文",
			Status:      "pending",
			Description: "读取相关代码、文档或配置",
		},
		{
			ID:          "execute",
			Title:       "执行任务",
			Status:      "pending",
			Description: planExecuteDescription(agent, prompt),
		},
		{
			ID:          "summarize",
			Title:       "整理结果",
			Status:      "pending",
			Description: "汇总输出、错误和最终结论",
		},
	}

	if needsDelegation(prompt, agent) {
		steps = append(steps[:2], append([]biz.PlanStep{{
			ID:          "delegate",
			Title:       "派发子任务",
			Status:      "pending",
			Description: "按任务性质委派给兼容 agent 或远端 agent",
		}}, steps[2:]...)...)
	}

	return steps
}

func advancePlan(steps []biz.PlanStep, event biz.DelegationEvent) []biz.PlanStep {
	next := clonePlanSteps(steps)
	switch event.Stage {
	case "task_running", "message_received":
		setPlanStepStatus(next, "understand", "completed")
		setFirstPending(next, "in_progress")
	case "tool_read_file":
		setPlanStepStatus(next, "inspect", "completed")
		setFirstPending(next, "in_progress")
	case "delegate_local", "delegate_remote":
		setPlanStepStatus(next, "delegate", "completed")
		setFirstPending(next, "in_progress")
	case "tool_edit_file", "tool_write_file", "tool_exec_command", "handle_direct", "remote_execute_done":
		setPlanStepStatus(next, "execute", "in_progress")
	case "task_done", "message_done", "verification_done", "final_answer":
		for i := range next {
			next[i].Status = "completed"
		}
	case "task_failed", "message_failed", "verification_failed", "remote_execute_failed":
		setPlanStepStatus(next, "summarize", "failed")
	}
	return next
}

func planExecuteDescription(agent biz.TaskAgent, prompt string) string {
	text := strings.TrimSpace(prompt)
	switch agent {
	case biz.TaskAgentRouter:
		return "协调任务、选择处理路径，并在需要时触发委派"
	case biz.TaskAgentCoder:
		return "完成实现、改动或技术验证"
	default:
		if strings.Contains(text, "总结") || strings.Contains(text, "说明") {
			return "读取上下文并生成结果说明"
		}
		return "按任务需要调用工具、修改文件或执行命令"
	}
}

func needsDelegation(prompt string, agent biz.TaskAgent) bool {
	if agent == biz.TaskAgentRouter {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(prompt))
	return strings.Contains(text, "委派") || strings.Contains(text, "router") || strings.Contains(text, "review")
}

func setPlanStepStatus(steps []biz.PlanStep, id, status string) {
	for i := range steps {
		if steps[i].ID == id {
			steps[i].Status = status
			return
		}
	}
}

func setFirstPending(steps []biz.PlanStep, status string) {
	for i := range steps {
		if steps[i].Status == "pending" {
			steps[i].Status = status
			return
		}
	}
}

func clonePlanSteps(steps []biz.PlanStep) []biz.PlanStep {
	if len(steps) == 0 {
		return nil
	}
	cloned := make([]biz.PlanStep, len(steps))
	copy(cloned, steps)
	return cloned
}
