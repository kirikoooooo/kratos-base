package ctx

import (
	"context"

	bizauthz "kratos-demo/internal/biz/authz"
)

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal bizauthz.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFrom(ctx context.Context) (bizauthz.Principal, bool) {
	if ctx == nil {
		return bizauthz.Principal{}, false
	}
	principal, ok := ctx.Value(principalContextKey{}).(bizauthz.Principal)
	return principal, ok
}
