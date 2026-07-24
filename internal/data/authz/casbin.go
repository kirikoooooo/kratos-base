package authz

import (
	"context"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"

	bizauthz "kratos-demo/internal/biz/authz"
	"kratos-demo/internal/conf"
)

const rbacModel = `[request_definition]
r = sub, obj, path, cmd

[policy_definition]
p = sub, obj, path, cmd

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
	m = g(r.sub, p.sub) && (p.obj == "*" || r.obj == p.obj) && (r.path == "" || p.path == "*" || keyMatch2(r.path, p.path)) && (r.cmd == "" || p.cmd == "*" || r.cmd == p.cmd)
`

// CasbinAuthorizer evaluates the configured principal-to-role and role-to-tool
// policies. Policies are in memory; configuration remains the source of truth.
type CasbinAuthorizer struct {
	enabled    bool
	enforcer   *casbin.Enforcer
	principals map[string]bizauthz.Principal
}

func NewCasbinFileAuthorizer(config *conf.Security) (*CasbinAuthorizer, error) {
	if config == nil {
		return nil, fmt.Errorf("security configuration is nil")
	}
	modelFile := strings.TrimSpace(config.GetCasbinModelFile())
	policyFile := strings.TrimSpace(config.GetCasbinPolicyFile())
	if modelFile == "" || policyFile == "" {
		return nil, fmt.Errorf("security.casbin_model_file and security.casbin_policy_file are required")
	}
	enforcer, err := casbin.NewEnforcer(modelFile, policyFile)
	if err != nil {
		return nil, fmt.Errorf("load Casbin model or policy: %w", err)
	}
	return newCasbinAuthorizer(config, enforcer)
}

func NewCasbinAuthorizer(config *conf.Security) (*CasbinAuthorizer, error) {
	if config == nil {
		return nil, fmt.Errorf("security configuration is nil")
	}
	policyModel, err := model.NewModelFromString(rbacModel)
	if err != nil {
		return nil, fmt.Errorf("build Casbin RBAC model: %w", err)
	}
	enforcer, err := casbin.NewEnforcer(policyModel)
	if err != nil {
		return nil, fmt.Errorf("create Casbin enforcer: %w", err)
	}
	return newCasbinAuthorizer(config, enforcer)
}

func newCasbinAuthorizer(config *conf.Security, enforcer *casbin.Enforcer) (*CasbinAuthorizer, error) {
	a := &CasbinAuthorizer{enabled: config.GetEnabled(), enforcer: enforcer, principals: map[string]bizauthz.Principal{}}
	if hasFilePolicy(config) {
		return a.loadPrincipalsFromPolicy(config)
	}
	roles := make(map[string]struct{}, len(config.GetRoles()))
	for _, configured := range config.GetRoles() {
		role := strings.TrimSpace(configured.GetName())
		if role == "" {
			return nil, fmt.Errorf("security role name is empty")
		}
		if _, exists := roles[role]; exists {
			return nil, fmt.Errorf("duplicate security role %q", role)
		}
		roles[role] = struct{}{}
		permissions := normalizedOrWildcard(configured.GetPermissions())
		paths := casbinPaths(configured.GetPaths())
		commands := normalizedOrWildcard(configured.GetCommandAllowlist())
		for _, permission := range permissions {
			for _, path := range paths {
				for _, command := range commands {
					if _, err := enforcer.AddPolicy(role, permission, path, command); err != nil {
						return nil, fmt.Errorf("add Casbin policy for role %q: %w", role, err)
					}
				}
			}
		}
	}
	for _, configured := range config.GetPrincipals() {
		id := strings.TrimSpace(configured.GetId())
		if id == "" {
			return nil, fmt.Errorf("security principal id is empty")
		}
		if _, exists := a.principals[id]; exists {
			return nil, fmt.Errorf("duplicate security principal %q", id)
		}
		roles := normalized(configured.GetRoles())
		a.principals[id] = bizauthz.Principal{ID: id, Roles: roles}
		for _, role := range roles {
			if _, err := enforcer.AddGroupingPolicy(id, role); err != nil {
				return nil, fmt.Errorf("add Casbin role binding for principal %q: %w", id, err)
			}
		}
	}
	if a.enabled && strings.TrimSpace(config.GetLocalPrincipal()) != "" {
		if _, ok := a.principals[config.GetLocalPrincipal()]; !ok {
			return nil, fmt.Errorf("security.local_principal %q is not configured", config.GetLocalPrincipal())
		}
	}
	return a, nil
}

func hasFilePolicy(config *conf.Security) bool {
	return strings.TrimSpace(config.GetCasbinModelFile()) != "" || strings.TrimSpace(config.GetCasbinPolicyFile()) != ""
}

func (a *CasbinAuthorizer) loadPrincipalsFromPolicy(config *conf.Security) (*CasbinAuthorizer, error) {
	if strings.TrimSpace(config.GetCasbinModelFile()) == "" || strings.TrimSpace(config.GetCasbinPolicyFile()) == "" {
		return nil, fmt.Errorf("security.casbin_model_file and security.casbin_policy_file must be set together")
	}
	rules, err := a.enforcer.GetGroupingPolicy()
	if err != nil {
		return nil, fmt.Errorf("read Casbin grouping policies: %w", err)
	}
	for _, rule := range rules {
		if len(rule) < 2 {
			continue
		}
		id, role := strings.TrimSpace(rule[0]), strings.TrimSpace(rule[1])
		if id == "" || role == "" {
			continue
		}
		principal := a.principals[id]
		principal.ID = id
		principal.Roles = append(principal.Roles, role)
		a.principals[id] = principal
	}
	if a.enabled && strings.TrimSpace(config.GetLocalPrincipal()) != "" {
		if _, ok := a.principals[config.GetLocalPrincipal()]; !ok {
			return nil, fmt.Errorf("security.local_principal %q is not configured in Casbin policy", config.GetLocalPrincipal())
		}
	}
	return a, nil
}

func (a *CasbinAuthorizer) LocalPrincipal(id string) (bizauthz.Principal, error) {
	if !a.enabled {
		return bizauthz.Principal{ID: "local-development"}, nil
	}
	principal, ok := a.principals[strings.TrimSpace(id)]
	if !ok {
		return bizauthz.Principal{}, fmt.Errorf("RBAC principal %q is not configured", id)
	}
	return principal, nil
}

func (a *CasbinAuthorizer) Authorize(_ context.Context, principal bizauthz.Principal, request bizauthz.Request) error {
	if !a.enabled {
		return nil
	}
	if strings.TrimSpace(principal.ID) == "" {
		return fmt.Errorf("RBAC denied %s: missing principal", request.Permission)
	}
	allowed, err := a.enforcer.Enforce(principal.ID, strings.TrimSpace(request.Permission), strings.TrimSpace(request.Path), strings.TrimSpace(request.Command))
	if err != nil {
		return fmt.Errorf("evaluate Casbin RBAC policy: %w", err)
	}
	if !allowed {
		return fmt.Errorf("RBAC denied %s for principal %q", request.Permission, principal.ID)
	}
	return nil
}

func normalized(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func normalizedOrWildcard(values []string) []string {
	if result := normalized(values); len(result) > 0 {
		return result
	}
	return []string{"*"}
}

// keyMatch2 accepts a trailing * but not the glob-style ** used by the YAML
// configuration. Convert only that suffix, preserving the public config form.
func casbinPaths(values []string) []string {
	paths := normalizedOrWildcard(values)
	for i, value := range paths {
		switch {
		case value == "**":
			paths[i] = "*"
		case strings.HasSuffix(value, "/**"):
			paths[i] = strings.TrimSuffix(value, "**") + "*"
		}
	}
	return paths
}
