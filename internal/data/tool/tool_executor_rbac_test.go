package tool

import (
	"context"
	"testing"

	bizauthz "kratos-demo/internal/biz/authz"
	"kratos-demo/internal/conf"
	agentctx "kratos-demo/internal/data/agent_runtime/ctx"
	dataauthz "kratos-demo/internal/data/authz"
)

func TestToolExecutorRBACDeniesBeforeFileRead(t *testing.T) {
	tools, err := NewToolExecutor(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	authorizer, err := dataauthz.NewCasbinAuthorizer(&conf.Security{Enabled: true,
		Principals: []*conf.Security_Principal{{Id: "viewer", Roles: []string{"viewer"}}},
		Roles:      []*conf.Security_Role{{Name: "viewer", Permissions: []string{"tool.read_file"}, Paths: []string{"docs/**"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tools.SetAuthorizer(authorizer)
	ctx := agentctx.WithPrincipal(context.Background(), bizauthz.Principal{ID: "viewer", Roles: []string{"viewer"}})
	if _, err := tools.readFile(ctx, "go.mod"); err == nil {
		t.Fatal("expected RBAC denial")
	}
}
