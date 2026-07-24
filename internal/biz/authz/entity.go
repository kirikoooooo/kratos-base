package authz

import "context"

type Principal struct {
	ID    string
	Roles []string
}

type Request struct {
	Permission string
	Path       string
	Command    string
}

type Authorizer interface {
	Authorize(ctx context.Context, principal Principal, request Request) error
}
