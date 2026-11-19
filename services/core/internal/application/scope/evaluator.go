// Package scope is the evaluator use case (P-04). It is the only caller of the
// decision function scope.Decide, and the only place that records its verdict as
// a policy decision. The claim at E2 and the launch at E3 of the worker use it,
// and so will the job creation of M21 and the promotion of an asset of M20. No
// second evaluator exists.
//
// A missing source, an error of a source and an input that disagrees with the
// job are denials. The evaluator never guesses (S1.6).
package scope

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// The reasons that the evaluator adds to those of the decision function.
const (
	ReasonInputsUnavailable = "inputs_unavailable"
	ReasonInputsMismatch    = "inputs_mismatch"
)

type Deps struct {
	Inputs    outbound.ScopeInputs
	Decisions outbound.PolicyDecisions
	Audit     outbound.AuditSink
	NewID     func() string
	Now       func() time.Time
	Logger    *slog.Logger
}

type Evaluator struct{ Deps }

func New(d Deps) *Evaluator {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Evaluator{d}
}

// Request, Result and the evaluator interface are ports (ports/inbound), so the
// job worker uses them without importing this package.
type (
	Request = inbound.ScopeRequest
	Result  = inbound.ScopeResult
)

// Evaluate loads the inputs, calls the decision function and records the
// verdict in the transaction of ctx, so the decision and the change that
// depends on it commit together. The error is for a failure to record: the
// caller must not act on a decision that was not recorded. A refusal is a
// Result that does not allow.
func (e *Evaluator) Evaluate(ctx context.Context, r Request) (Result, error) {
	if r.Point != scope.PointE2 && r.Point != scope.PointE3 {
		return Result{}, errors.New("scope: the evaluator of the worker decides at E2 and E3")
	}
	id := e.NewID()
	in, loadErr := e.Inputs.Load(ctx, r.Job)
	var verdict scope.Verdict
	switch {
	case loadErr != nil:
		verdict = deny(ReasonInputsUnavailable)
	case !consistent(r.Job, in):
		verdict = deny(ReasonInputsMismatch)
	default:
		verdict = scope.Decide(scope.DecisionInput{
			Point: r.Point, DecisionID: id, Now: e.Now(), Operation: in.Operation, Rules: in.Rules,
			Validation: in.Validation, Policy: in.Policy, Approval: in.Approval, Job: approval.JobID(r.Job.ID),
			RunningJobs: r.RunningJobs, PreviousTreeConfirmed: r.PreviousTreeConfirmed,
		})
	}
	d := outbound.PolicyDecision{
		ID: id, ProgramID: r.Job.ProgramID, AssessmentID: r.Job.AssessmentID, OperationID: r.Job.OperationID, JobID: r.Job.ID,
		Point: r.Point, Outcome: verdict.Outcome, Reasons: verdict.Reasons,
		ScopeVersionID: firstNonEmpty(in.Policy.ScopeVersionID, r.Job.ScopeVersionID), ValidationID: firstNonEmpty(in.Validation.ID, r.Job.ValidationID),
		ApprovalID: r.Job.ApprovalID, PolicySnapshot: in.Policy.PolicySnapshot,
	}
	if loadErr != nil || !consistent(r.Job, in) {
		d.ScopeVersionID, d.ValidationID, d.PolicySnapshot = r.Job.ScopeVersionID, r.Job.ValidationID, [32]byte{}
	}
	if err := e.Decisions.Insert(ctx, d); err != nil {
		return Result{}, fmt.Errorf("scope: record the decision: %w", err)
	}
	return Result{DecisionID: id, Verdict: verdict, Input: in}, nil
}

func deny(reason string) scope.Verdict {
	return scope.Verdict{Outcome: scope.OutcomeDeny, Reasons: []string{reason}}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// consistent says that what the source returned is about this job. A source that
// answers for another assessment or another class is a defect, and it denies.
func consistent(j outbound.JobRecord, in outbound.ScopeInput) bool {
	op := in.Operation
	if op.IsZero() || op.AssessmentID() != j.AssessmentID || op.Class().String() != string(j.Risk) {
		return false
	}
	if j.Risk.NeedsApproval() && (in.Approval == nil || in.Approval.ID() != j.ApprovalID) {
		return false
	}
	return true
}

// AuditDenial records a refusal after the transaction that wrote its decision
// has committed (the event names the decision, which must exist). The refusal
// stands whether or not the record is written (SEC 13).
func (e *Evaluator) AuditDenial(ctx context.Context, who audit.Actor, r Request, res Result, requestID string) {
	reasons := res.Verdict.Reasons
	reason := "denied"
	if len(reasons) > 0 {
		reason = reasons[0]
	}
	detail, err := audit.ScopeEvaluation{Allowed: false, Reasons: reasons}.Detail()
	if err == nil {
		detail["point"] = r.Point.String()
		var ev audit.Event
		ev, err = audit.New(e.NewID(), audit.Input{
			Actor: who, Action: "policy.decision_denied", Outcome: audit.Denied, Reason: reason, RequestID: requestID,
			Subject: audit.Subject{ProgramID: r.Job.ProgramID, AssessmentID: r.Job.AssessmentID, JobID: r.Job.ID, EntityType: "policy_decision", EntityID: res.DecisionID, DecisionID: res.DecisionID},
			Detail:  detail,
		})
		if err == nil {
			err = e.Audit.Record(ctx, ev)
		}
	}
	if err != nil {
		e.Logger.Error("scope: a denial was not audited", "point", r.Point.String(), "decision", res.DecisionID)
	}
}
