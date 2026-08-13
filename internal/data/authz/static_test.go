package authz

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bizauthz "kratos-demo/internal/biz/authz"
	"kratos-demo/internal/conf"
)

func TestCasbinAuthorizerLoadsModelAndPolicyFiles(t *testing.T) {
	dir := t.TempDir()
	modelFile := filepath.Join(dir, "model.conf")
	policyFile := filepath.Join(dir, "policy.csv")
	if err := os.WriteFile(modelFile, []byte("[request_definition]\nr = sub, obj, path, cmd\n\n[policy_definition]\np = sub, obj, path, cmd\n\n[role_definition]\ng = _, _\n\n[policy_effect]\ne = some(where (p.eft == allow))\n\n[matchers]\nm = g(r.sub, p.sub) && r.obj == p.obj\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyFile, []byte("p, developer, tool.read_file, *, *\ng, dev, developer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authorizer, err := NewCasbinFileAuthorizer(&conf.Security{Enabled: true, CasbinModelFile: modelFile, CasbinPolicyFile: policyFile})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authorizer.LocalPrincipal("dev")
	if err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Authorize(context.Background(), principal, bizauthz.Request{Permission: "tool.read_file"}); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimePolicyAllowsDeveloperToReadMonorepoLayout(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	policyFile := filepath.Join(root, "app", "agent-runtime", "configs", "casbin", "policy.csv")
	modelFile := filepath.Join(root, "app", "agent-runtime", "configs", "casbin", "model.conf")
	authorizer, err := NewCasbinFileAuthorizer(&conf.Security{
		Enabled:          true,
		LocalPrincipal:   "local-dev",
		CasbinModelFile:  modelFile,
		CasbinPolicyFile: policyFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authorizer.LocalPrincipal("local-dev")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".", "go.mod", "Makefile", "app/agent-runtime/cmd/main.go", "pkg/kernel/task.go"} {
		if err := authorizer.Authorize(context.Background(), principal, bizauthz.Request{Permission: "tool.read_file", Path: path}); err != nil {
			t.Fatalf("expected developer to read %q: %v", path, err)
		}
	}
	if err := authorizer.Authorize(context.Background(), principal, bizauthz.Request{Permission: "tool.write_file", Path: "app/agent-runtime/cmd/main.go"}); err == nil {
		t.Fatal("read policy must not grant write access")
	}
}

func TestCasbinAuthorizerDeniesUnlistedToolAndPath(t *testing.T) {
	authorizer, err := NewCasbinAuthorizer(&conf.Security{Enabled: true,
		Principals: []*conf.Security_Principal{{Id: "dev", Roles: []string{"developer"}}},
		Roles:      []*conf.Security_Role{{Name: "developer", Permissions: []string{"tool.read_file"}, Paths: []string{"docs/**"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authorizer.LocalPrincipal("dev")
	if err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Authorize(context.Background(), principal, bizauthz.Request{Permission: "tool.read_file", Path: "docs/design.md"}); err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Authorize(context.Background(), principal, bizauthz.Request{Permission: "tool.read_file", Path: "internal/data.go"}); err == nil {
		t.Fatal("expected path denial")
	}
	if err := authorizer.Authorize(context.Background(), principal, bizauthz.Request{Permission: "tool.write_file", Path: "docs/new.md"}); err == nil {
		t.Fatal("expected permission denial")
	}
}

func TestCasbinAuthorizerAllowsConfiguredPrincipalRole(t *testing.T) {
	authorizer, err := NewCasbinAuthorizer(&conf.Security{Enabled: true,
		Principals: []*conf.Security_Principal{{Id: "dev", Roles: []string{"developer"}}},
		Roles:      []*conf.Security_Role{{Name: "developer", Permissions: []string{"tool.exec_command"}, CommandAllowlist: []string{"go test ./..."}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authorizer.LocalPrincipal("dev")
	if err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Authorize(context.Background(), principal, bizauthz.Request{Permission: "tool.exec_command", Command: "go test ./..."}); err != nil {
		t.Fatalf("expected configured command to be allowed: %v", err)
	}
	if err := authorizer.Authorize(context.Background(), principal, bizauthz.Request{Permission: "tool.exec_command", Command: "go test ./internal/..."}); err == nil {
		t.Fatal("expected unlisted command to be denied")
	}
}

func TestCasbinAuthorizerAllowsDaytonaAnalystRole(t *testing.T) {
	authorizer, err := NewCasbinAuthorizer(&conf.Security{Enabled: true,
		Principals: []*conf.Security_Principal{{Id: "local-dev", Roles: []string{"developer", "analyst"}}},
		Roles:      []*conf.Security_Role{{Name: "analyst", Permissions: []string{"tool.daytona_data_analysis"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authorizer.LocalPrincipal("local-dev")
	if err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Authorize(context.Background(), principal, bizauthz.Request{Permission: "tool.daytona_data_analysis"}); err != nil {
		t.Fatalf("expected Daytona analyst permission: %v", err)
	}
}
