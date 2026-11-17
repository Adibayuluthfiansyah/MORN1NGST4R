package scope

import (
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

// Outcome is the result of a decision. The zero value is OutcomeDeny, so a
// Verdict that was never filled in denies.
type Outcome int

const (
	OutcomeDeny Outcome = iota
	OutcomeAllow
	// OutcomeRequireApproval says the operation is in scope and needs an
	// approved approval before a job can be created. Nothing may run on it.
	OutcomeRequireApproval
)

// String returns the stable name that is stored and audited.
func (o Outcome) String() string {
	switch o {
	case OutcomeAllow:
		return "allow"
	case OutcomeRequireApproval:
		return "require_approval"
	}
	return "deny"
}

// The reasons of the decision function, next to those of Evaluate and of the
// approval package. They are stable codes, safe to store and to show.
const (
	ReasonAllowed                   = "allowed"
	ReasonInvalidInput              = "invalid_input"
	ReasonLimitsNotEstablished      = "limits_not_established"
	ReasonIntrusiveDisabled         = "intrusive_disabled"
	ReasonValidationNotCurrent      = "validation_not_current"
	ReasonApprovalRequired          = "approval_required"
	ReasonApprovalScopeChanged      = string(approval.ReasonScopeChanged)
	ReasonApprovalPolicyChanged     = string(approval.ReasonPolicyChanged)
	ReasonApprovalValidationChanged = string(approval.ReasonValidationChanged)
)

// ValidationState is the target authorization validation in force for the
// assessment (FR-SCOPE-005). The application layer sets Current to false when
// the record is missing, expired or stale.
type ValidationState struct {
	ID      string
	Current bool
}

// PolicyState is the security policy in force. The zero value disables
// intrusive operations (D-58).
type PolicyState struct {
	ScopeVersionID   string
	PolicySnapshot   [32]byte
	IntrusiveEnabled bool
}

// DecisionInput is everything the decision function reads. It reads no clock
// and no store: the application layer loads the state through ports and passes
// it in (P-03).
type DecisionInput struct {
	Point      Point
	DecisionID string
	Now        time.Time
	Operation  operation.Operation
	Rules      RuleSet
	Validation ValidationState
	Policy     PolicyState
	// Approval is the approval the job relies on, if any.
	Approval *approval.Approval
	// Job, RunningJobs and PreviousTreeConfirmed matter at E2 and E3.
	Job                   approval.JobID
	RunningJobs           int
	PreviousTreeConfirmed bool
}

// Verdict is the result of Decide. The zero value is a deny.
type Verdict struct {
	Outcome Outcome
	// Reasons are stable codes. A deny has exactly one.
	Reasons []string
	// Authorized is set only when Outcome is OutcomeAllow.
	Authorized AuthorizedOperation
}

func verdictDeny(reason string) Verdict {
	return Verdict{Outcome: OutcomeDeny, Reasons: []string{reason}}
}

// Decide is the pure policy function (P-03). It is the only function that
// returns an AuthorizedOperation. Missing data, an unparseable or ambiguous
// target and any failed check return a deny. The first failed check decides
// and its reason is the only one reported.
//
// Order: input, scope, then for active operations the limits, the intrusive
// switch, the policy state, the validation, and the approval (its state at this
// point, then its binding to the scope version, the policy snapshot and the
// validation record, all in the Check functions of the approval package that
// the state change uses too). A passive operation needs no approval (S1.6
// item 6).
//
// Only internal/application/scope may call it (P-04, checked by the
// architecture test). The caller records the verdict as a policy decision and
// audits it.
func Decide(in DecisionInput) Verdict {
	op := in.Operation
	if (in.Point != PointE1 && in.Point != PointE2 && in.Point != PointE3) ||
		!validDecisionID(in.DecisionID) || in.Now.IsZero() || op.IsZero() {
		return verdictDeny(ReasonInvalidInput)
	}
	// A claim and a launch are for a job. E1 comes before the job exists.
	if in.Point != PointE1 && !validDecisionID(string(in.Job)) {
		return verdictDeny(ReasonInvalidInput)
	}
	now := in.Now.Round(0)

	// Scope. The operation's target must be exactly what the scope engine builds.
	tg, err := ParseTarget(op.Target(), op.Port())
	if err != nil || canonicalText(tg) != op.Target() {
		return verdictDeny(ReasonInvalidTarget)
	}
	if d := in.Rules.Evaluate(tg); !d.Allowed {
		return verdictDeny(d.Reason)
	}

	class := op.Class()
	if class == operation.ClassUnknown {
		return verdictDeny(ReasonInvalidInput)
	}
	if !class.IsActive() {
		return allow(in, now.Add(AuthorizationTTL))
	}

	// Active operations.
	if !op.Limits().Established() {
		return verdictDeny(ReasonLimitsNotEstablished)
	}
	if class == operation.ClassIntrusive && !in.Policy.IntrusiveEnabled {
		return verdictDeny(ReasonIntrusiveDisabled)
	}
	if !validDecisionID(in.Policy.ScopeVersionID) || in.Policy.PolicySnapshot == [32]byte{} || !validDecisionID(in.Validation.ID) {
		return verdictDeny(ReasonInvalidInput)
	}
	if !in.Validation.Current {
		return verdictDeny(ReasonValidationNotCurrent)
	}

	// Approval.
	if in.Approval == nil || in.Approval.Status() == approval.StatusInvalid {
		if in.Point == PointE1 {
			return Verdict{Outcome: OutcomeRequireApproval, Reasons: []string{ReasonApprovalRequired}}
		}
		return verdictDeny(ReasonApprovalRequired)
	}
	ap := in.Approval
	// The state in force now. The checks compare it with the state the approval
	// was made in, so a changed scope version, policy snapshot or validation
	// record denies here and in the state change alike.
	current := approval.Binding{ValidationID: in.Validation.ID, ScopeVersionID: in.Policy.ScopeVersionID, PolicySnapshot: in.Policy.PolicySnapshot}
	var aerr error
	switch in.Point {
	case PointE1:
		aerr = ap.CheckUsable(op, current, now)
	case PointE2:
		aerr = ap.CheckClaim(in.Job, op, current, now)
	case PointE3:
		aerr = ap.CheckLaunch(in.Job, op, current, in.RunningJobs, in.PreviousTreeConfirmed, now)
	}
	if aerr != nil {
		reason, ok := approval.ReasonOf(aerr)
		if !ok {
			return verdictDeny(ReasonInvalidInput)
		}
		// An approval that is still pending can be approved: that is a request
		// for approval and not a refusal.
		if in.Point == PointE1 && reason == approval.ReasonNotApproved && ap.Status() == approval.StatusPending {
			return Verdict{Outcome: OutcomeRequireApproval, Reasons: []string{string(reason)}}
		}
		return verdictDeny(string(reason))
	}

	// The authorization never outlives the approval that backs it.
	expires := now.Add(AuthorizationTTL)
	if ap.ExpiresAt().Before(expires) {
		expires = ap.ExpiresAt()
	}
	return allow(in, expires)
}

func allow(in DecisionInput, expires time.Time) Verdict {
	return Verdict{
		Outcome: OutcomeAllow,
		Reasons: []string{ReasonAllowed},
		Authorized: AuthorizedOperation{
			decisionID: in.DecisionID,
			hash:       in.Operation.Hash(),
			job:        in.Job,
			expiresAt:  expires,
			point:      in.Point,
		},
	}
}

// canonicalText is the text form of a Target that ParseTarget builds.
func canonicalText(t Target) string {
	if t.Host != "" {
		return string(t.Host)
	}
	return t.Addr.String()
}

func validDecisionID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if i == 0 && !alnum {
			return false
		}
		if !alnum && c != '.' && c != '_' && c != ':' && c != '-' {
			return false
		}
	}
	return true
}
