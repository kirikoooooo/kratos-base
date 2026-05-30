package service

import (
	"testing"

	agentctx "kratos-demo/internal/data/agent/ctx"
)

func TestMatchShiftTab(t *testing.T) {
	cases := []struct {
		in       []byte
		wantOK   bool
		wantCons int
	}{
		{[]byte("\x1bZ"), true, 2},
		{[]byte("\x1b[Z"), true, 3},
		{[]byte("\x1b[1;2Z"), true, 6},
		{[]byte("\x1b[A"), false, 0},
		{[]byte("a"), false, 0},
	}
	for _, tc := range cases {
		gotCons, gotOK := matchShiftTab(tc.in)
		if gotOK != tc.wantOK || gotCons != tc.wantCons {
			t.Fatalf("matchShiftTab(%q) = (%d, %v), want (%d, %v)", tc.in, gotCons, gotOK, tc.wantCons, tc.wantOK)
		}
	}
}

func TestPermissionModeCycle(t *testing.T) {
	c := &CLIService{permMode: PermAsk}
	if got := c.cyclePermissionMode(); got != PermAgent {
		t.Fatalf("cycle 1 = %v, want agent", got)
	}
	if got := c.cyclePermissionMode(); got != PermAuto {
		t.Fatalf("cycle 2 = %v, want auto", got)
	}
	if got := c.cyclePermissionMode(); got != PermAsk {
		t.Fatalf("cycle 3 = %v, want ask", got)
	}
}

func TestResolveRiskApproval(t *testing.T) {
	c := &CLIService{permMode: PermAgent}
	del := agentctx.RiskAction{Tool: "delete_file"}
	exec := agentctx.RiskAction{Tool: "exec_command"}

	if decided, allowed := c.resolveRiskApproval(del); !decided || !allowed {
		t.Fatalf("agent delete_file: want auto approve, got (%v,%v)", decided, allowed)
	}
	if decided, _ := c.resolveRiskApproval(exec); decided {
		t.Fatal("agent exec_command should require UI")
	}

	c.setPermissionMode(PermAuto)
	if decided, allowed := c.resolveRiskApproval(exec); !decided || !allowed {
		t.Fatalf("auto exec: want auto approve, got (%v,%v)", decided, allowed)
	}
}
