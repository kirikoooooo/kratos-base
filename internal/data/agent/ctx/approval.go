package ctx

import "context"

// RiskAction describes a tool operation that may need human approval.
type RiskAction struct {
	Tool    string
	Summary string
	Detail  string
}

// RiskApprover returns true when the user allows the operation.
type RiskApprover func(ctx context.Context, action RiskAction) (bool, error)

type riskApproverContextKey struct{}

func WithRiskApprover(ctx context.Context, approver RiskApprover) context.Context {
	if approver == nil {
		return ctx
	}
	return context.WithValue(ctx, riskApproverContextKey{}, approver)
}

func RiskApproverFrom(ctx context.Context) RiskApprover {
	if ctx == nil {
		return nil
	}
	approver, _ := ctx.Value(riskApproverContextKey{}).(RiskApprover)
	return approver
}
