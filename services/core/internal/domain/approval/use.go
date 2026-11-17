package approval

import (
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

// The consumption state machine (S1.6 items 1 to 3, D-30, D-49, D-55).
//
// One use is consumed per (approval, job) at the claim, E2. A retry claim for
// the same job reuses that use. A launch, E3, records an attempt on the use.
// The Check functions hold every rule and change nothing. Claim and
// AuthorizeLaunch call them and then change state, so the decision function
// and the state change cannot disagree. A denial returns the zero Approval: no
// state change, no attempt, and the consumed use is never given back.
//
// Atomicity under concurrency is a database property (M3 constraint c1 of
// approval_uses and the conditional statement of M7). This package is the
// model that those must agree with.

// JobID names a job. The job domain package imports approval, so the approval
// package holds the ID type (structure.md 5.6).
type JobID string

// Use is the consumption of one approval by one job. The claim epoch is the
// fencing value of the claim that consumed it and a retry claim does not change
// it. Attempts counts the launches of the job under the approval. LaunchedAt is
// the time of the latest launch. AuthorizeLaunch records it, so a caller cannot
// move it and cannot extend the approved maximum duration.
type Use struct {
	job        JobID
	claimEpoch int
	attempts   int
	consumedAt time.Time
	launchedAt time.Time
}

func (u Use) JobID() JobID          { return u.job }
func (u Use) ClaimEpoch() int       { return u.claimEpoch }
func (u Use) Attempts() int         { return u.attempts }
func (u Use) ConsumedAt() time.Time { return u.consumedAt }
func (u Use) LaunchedAt() time.Time { return u.launchedAt }

// UsesConsumed is the number of uses consumed. It equals the number of jobs that
// hold a use and it never decreases.
func (a Approval) UsesConsumed() int { return len(a.uses) }

// Uses returns a copy of the consumed uses.
func (a Approval) Uses() []Use { return append([]Use(nil), a.uses...) }

// UseFor returns the use that a job holds.
func (a Approval) UseFor(job JobID) (Use, bool) {
	for _, u := range a.uses {
		if u.job == job {
			return u, true
		}
	}
	return Use{}, false
}

func validJob(j JobID) bool { return validID(string(j)) }

// checkLive holds the rules that every point shares: arguments, status, expiry,
// the match between the operation and the approval, and the match between the
// state the approval was made in and the state in force now (b). A change of the
// scope version, the policy snapshot or the validation record makes the
// approval unusable (FR-SCOPE-006, D-90).
func (a Approval) checkLive(op operation.Operation, b Binding, now time.Time) error {
	if now.IsZero() || op.IsZero() || !b.valid() {
		return deny(ReasonInvalidInput)
	}
	switch a.status {
	case StatusApproved:
	case StatusRevoked:
		return deny(ReasonRevoked)
	default:
		return deny(ReasonNotApproved)
	}
	if a.Expired(now) {
		return deny(ReasonExpired)
	}
	if op.Hash() != a.op.Hash() {
		return deny(ReasonOperationMismatch)
	}
	switch {
	case a.binding.ScopeVersionID != b.ScopeVersionID:
		return deny(ReasonScopeChanged)
	case a.binding.PolicySnapshot != b.PolicySnapshot:
		return deny(ReasonPolicyChanged)
	case a.binding.ValidationID != b.ValidationID:
		return deny(ReasonValidationChanged)
	}
	return nil
}

// CheckUsable is the rule of E1: the approval is approved, not expired, not
// revoked, matches the operation and still has a use left for a new job.
func (a Approval) CheckUsable(op operation.Operation, b Binding, now time.Time) error {
	if err := a.checkLive(op, b, now); err != nil {
		return err
	}
	if !a.op.Limits().Established() {
		return deny(ReasonLimitsNotEstablished)
	}
	if len(a.uses) >= a.maxUses {
		return deny(ReasonExhausted)
	}
	return nil
}

// CheckClaim is the rule of E2 for a job. A job that holds a use is a retry
// claim and needs no free use. An intrusive job is never retried (D-49).
func (a Approval) CheckClaim(job JobID, op operation.Operation, b Binding, now time.Time) error {
	if !validJob(job) {
		return deny(ReasonInvalidInput)
	}
	if err := a.checkLive(op, b, now); err != nil {
		return err
	}
	if u, ok := a.UseFor(job); ok {
		if a.op.Class() == operation.ClassIntrusive {
			return deny(ReasonIntrusiveNoRetry)
		}
		if u.attempts >= a.maxAttempts {
			return deny(ReasonAttemptsExhausted)
		}
		return nil
	}
	if len(a.uses) >= a.maxUses {
		return deny(ReasonExhausted)
	}
	return nil
}

// CheckLaunch is the rule of E3. It does not consume a use. running is the
// number of launched attempts of this approval that are running now.
// previousTreeConfirmed says that the process tree of the job's last attempt is
// confirmed gone, and it matters only when there was a last attempt.
func (a Approval) CheckLaunch(job JobID, op operation.Operation, b Binding, running int, previousTreeConfirmed bool, now time.Time) error {
	if !validJob(job) || running < 0 {
		return deny(ReasonInvalidInput)
	}
	if err := a.checkLive(op, b, now); err != nil {
		return err
	}
	u, ok := a.UseFor(job)
	if !ok {
		return deny(ReasonUseNotClaimed)
	}
	if a.op.Class() == operation.ClassIntrusive && u.attempts >= 1 {
		return deny(ReasonIntrusiveNoRetry)
	}
	if u.attempts >= a.maxAttempts {
		return deny(ReasonAttemptsExhausted)
	}
	if u.attempts >= 1 && !previousTreeConfirmed {
		return deny(ReasonPreviousAttemptRunning)
	}
	limits := a.op.Limits()
	if !limits.Established() {
		return deny(ReasonLimitsNotEstablished)
	}
	if running >= limits.MaxConcurrency {
		return deny(ReasonConcurrencyExceeded)
	}
	return nil
}

// Claim consumes one use for a job, or returns the use the job already holds
// (a retry claim). It returns the zero Approval and an error that matches
// ErrDenied when the claim is refused.
func (a Approval) Claim(job JobID, epoch int, op operation.Operation, b Binding, now time.Time) (Approval, Use, error) {
	if epoch < 1 {
		return Approval{}, Use{}, deny(ReasonInvalidInput)
	}
	if err := a.CheckClaim(job, op, b, now); err != nil {
		return Approval{}, Use{}, err
	}
	if u, ok := a.UseFor(job); ok {
		return a, u, nil // a retry claim reuses the original record
	}
	u := Use{job: job, claimEpoch: epoch, consumedAt: now.Round(0)}
	a.uses = append(append([]Use(nil), a.uses...), u)
	a.version++
	return a, u, nil
}

// AuthorizeLaunch records one launched attempt for the job after CheckLaunch
// passes. A denial returns the zero Approval and is not an attempt. The count
// belongs to the job (job_attempts), so the approval's version does not change.
func (a Approval) AuthorizeLaunch(job JobID, op operation.Operation, b Binding, running int, previousTreeConfirmed bool, now time.Time) (Approval, error) {
	if err := a.CheckLaunch(job, op, b, running, previousTreeConfirmed, now); err != nil {
		return Approval{}, err
	}
	uses := append([]Use(nil), a.uses...)
	for i := range uses {
		if uses[i].job == job {
			uses[i].attempts++
			uses[i].launchedAt = now.Round(0)
		}
	}
	a.uses = uses
	return a, nil
}

// RunningAllowed reports whether the operation that a job launched may keep
// running. It says no for an approval that is not approved (revoked included),
// for a job that has not launched, and when the scope version, the policy
// snapshot or the validation record has changed since the approval was made
// (S1.6 item 1). Otherwise it says yes until the recorded launch time plus the
// approved maximum duration. Expiry neither stops the operation nor extends it
// (D-48). The launch time is the one that AuthorizeLaunch recorded for the
// latest attempt, so the caller cannot extend the duration by passing another.
func (a Approval) RunningAllowed(job JobID, b Binding, now time.Time) bool {
	if a.status != StatusApproved || now.IsZero() || !b.valid() || a.binding != b {
		return false
	}
	u, ok := a.UseFor(job)
	if !ok || u.attempts < 1 || u.launchedAt.IsZero() {
		return false
	}
	return now.Round(0).Before(u.launchedAt.Add(a.op.Limits().MaxDuration))
}
