package tasking

import (
	"testing"

	"kratos-demo/internal/consts/public"
	datatrace "kratos-demo/internal/data/trace"
)

func TestInitialPlanAndAdvance(t *testing.T) {
	plan := initialPlan(public.AgentKindDefault, "请读 README 然后总结")
	if len(plan) < 4 {
		t.Fatalf("initialPlan() steps = %d, want >= 4", len(plan))
	}
	if plan[0].Status != "in_progress" {
		t.Fatalf("first step status = %q, want in_progress", plan[0].Status)
	}

	advanced := AdvancePlan(plan, datatrace.DelegationEvent{Stage: "tool_read_file"})
	foundCompleted := false
	for _, step := range advanced {
		if step.ID == "inspect" && step.Status == "completed" {
			foundCompleted = true
		}
	}
	if !foundCompleted {
		t.Fatalf("expected inspect step to complete after tool_read_file: %+v", advanced)
	}
}
