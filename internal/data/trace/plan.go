package trace

func advancePlan(steps []PlanStep, event DelegationEvent) []PlanStep {
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

func setPlanStepStatus(steps []PlanStep, id, status string) {
	for i := range steps {
		if steps[i].ID == id {
			steps[i].Status = status
			return
		}
	}
}

func setFirstPending(steps []PlanStep, status string) {
	for i := range steps {
		if steps[i].Status == "pending" {
			steps[i].Status = status
			return
		}
	}
}

func clonePlanSteps(steps []PlanStep) []PlanStep {
	if len(steps) == 0 {
		return nil
	}
	cloned := make([]PlanStep, len(steps))
	copy(cloned, steps)
	return cloned
}
