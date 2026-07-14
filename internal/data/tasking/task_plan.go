package tasking

import (
	"strings"

	"kratos-demo/internal/consts/public"
	datatrace "kratos-demo/internal/data/trace"
)

func InitialPlan(agent public.AgentKind, prompt string) []datatrace.PlanStep {
	return initialPlan(agent, prompt)
}

func initialPlan(agent public.AgentKind, prompt string) []datatrace.PlanStep {
	steps := []datatrace.PlanStep{
		{
			ID:          "understand",
			Title:       "????",
			Status:      "in_progress",
			Description: "?????????",
		},
		{
			ID:          "inspect",
			Title:       "?????",
			Status:      "pending",
			Description: "????????????",
		},
		{
			ID:          "execute",
			Title:       "????",
			Status:      "pending",
			Description: planExecuteDescription(agent, prompt),
		},
		{
			ID:          "summarize",
			Title:       "????",
			Status:      "pending",
			Description: "????????????",
		},
	}

	if needsDelegation(prompt, agent) {
		steps = append(steps[:2], append([]datatrace.PlanStep{{
			ID:          "delegate",
			Title:       "?????",
			Status:      "pending",
			Description: "?????????? agent ??? agent",
		}}, steps[2:]...)...)
	}

	return steps
}

func AdvancePlan(steps []datatrace.PlanStep, event datatrace.DelegationEvent) []datatrace.PlanStep {
	return advancePlan(steps, event)
}

func advancePlan(steps []datatrace.PlanStep, event datatrace.DelegationEvent) []datatrace.PlanStep {
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
	case "tool_edit_file", "tool_write_file", "tool_delete_file", "tool_exec_command", "handle_direct", "remote_execute_done":
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

func planExecuteDescription(agent public.AgentKind, prompt string) string {
	text := strings.TrimSpace(prompt)
	switch agent {
	case public.AgentKindRouter:
		return "?????????????????????"
	case public.AgentKindCoder:
		return "????????????"
	default:
		if strings.Contains(text, "??") || strings.Contains(text, "??") {
			return "????????????"
		}
		return "???????????????????"
	}
}

func needsDelegation(prompt string, agent public.AgentKind) bool {
	if agent == public.AgentKindRouter {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(prompt))
	return strings.Contains(text, "??") || strings.Contains(text, "router") || strings.Contains(text, "review")
}

func setPlanStepStatus(steps []datatrace.PlanStep, id, status string) {
	for i := range steps {
		if steps[i].ID == id {
			steps[i].Status = status
			return
		}
	}
}

func setFirstPending(steps []datatrace.PlanStep, status string) {
	for i := range steps {
		if steps[i].Status == "pending" {
			steps[i].Status = status
			return
		}
	}
}

func clonePlanSteps(steps []datatrace.PlanStep) []datatrace.PlanStep {
	if len(steps) == 0 {
		return nil
	}
	cloned := make([]datatrace.PlanStep, len(steps))
	copy(cloned, steps)
	return cloned
}
