package inbound

import (
	"context"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// ScopeRequest is one decision about a job. RunningJobs and
// PreviousTreeConfirmed matter at E3: the number of jobs that run under the
// approval now, and whether the process tree of the last attempt is confirmed
// gone.
type ScopeRequest struct {
	Point                 scope.Point
	Job                   outbound.JobRecord
	RunningJobs           int
	PreviousTreeConfirmed bool
}

// ScopeResult is the verdict and the id of the policy decision that records it.
type ScopeResult struct {
	DecisionID string
	Verdict    scope.Verdict
	Input      outbound.ScopeInput
}

func (r ScopeResult) Allowed() bool { return r.Verdict.Outcome == scope.OutcomeAllow }

// ScopeEvaluator is the evaluator use case. It is the only caller of the
// decision function scope.Decide.
type ScopeEvaluator interface {
	Evaluate(ctx context.Context, r ScopeRequest) (ScopeResult, error)
	AuditDenial(ctx context.Context, who audit.Actor, r ScopeRequest, res ScopeResult, requestID string)
}
