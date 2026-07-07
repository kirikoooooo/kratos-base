package service

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"kratos-demo/internal/biz"
	"kratos-demo/internal/conf"
)

func TestCLIServiceStartsAllAgentsAndHandlesHelp(t *testing.T) {
	dash := NewDashboardService(nil, nil, nil, nil, &conf.Runtime{})
	cli := &CLIService{
		dash:      dash,
		sessionID: "cli-test",
		in:        strings.NewReader("/agents\n/help\n/exit\n"),
		out:       &bytes.Buffer{},
	}
	cli.BindAIConfig(&conf.AI{Openai: &conf.AI_OpenAI{
		ApiKey:  "sk-test",
		BaseUrl: "https://example.com/v1",
	}})
	cli.ui = newCLIUI(cli.out)

	if err := cli.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, agent := range []biz.AgentKind{biz.AgentKindDefault, biz.AgentKindRouter, biz.AgentKindCoder, biz.AgentKindReviewer} {
		if !dash.isAgentStarted(agent) {
			t.Fatalf("agent %s was not started", agent)
		}
	}

	out := cli.out.(*bytes.Buffer).String()
	if !strings.Contains(out, "default") || !strings.Contains(out, "router") {
		t.Fatalf("unexpected /agents output: %s", out)
	}
	if !strings.Contains(out, "commands") {
		t.Fatalf("missing help in output: %s", out)
	}
}
