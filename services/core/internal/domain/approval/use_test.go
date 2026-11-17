package approval_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

func reasonOf(t testing.TB, err error) approval.Reason {
	t.Helper()
	if !errors.Is(err, approval.ErrDenied) {
		t.Fatalf("error %v is not a denial", err)
	}
	r, ok := approval.ReasonOf(err)
	if !ok {
		t.Fatalf("error %v carries no reason", err)
	}
	return r
}

func wantDenied(t testing.TB, what string, err error, want approval.Reason) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: allowed, want denial %q", what, want)
		return
	}
	if got := reasonOf(t, err); got != want {
		t.Errorf("%s: denied with %q, want %q", what, got, want)
	}
}

func claim(t testing.TB, a approval.Approval, job approval.JobID) approval.Approval {
	t.Helper()
	out, _, err := a.Claim(job, 1, a.Operation(), binding(), t0)
	if err != nil {
		t.Fatalf("Claim(%s): %v", job, err)
	}
	return out
}

func TestADenialCarriesAStableReason(t *testing.T) {
	err := error(&approval.DenialError{Reason: approval.ReasonExpired})
	if !errors.Is(err, approval.ErrDenied) {
		t.Error("a DenialError must match ErrDenied")
	}
	var de *approval.DenialError
	if !errors.As(err, &de) || de.Reason != "approval_expired" {
		t.Error("errors.As did not find the reason")
	}
	if strings.Contains(err.Error(), t0.String()) || !strings.Contains(err.Error(), "approval_expired") {
		t.Errorf("error text = %q", err.Error())
	}
	if _, ok := approval.ReasonOf(errors.New("other")); ok {
		t.Error("ReasonOf accepted an error that is not a denial")
	}
	// The stable codes are part of the API contract (error-model.md).
	for r, s := range map[approval.Reason]string{
		approval.ReasonNotApproved: "approval_not_approved", approval.ReasonRevoked: "approval_revoked",
		approval.ReasonExpired: "approval_expired", approval.ReasonExhausted: "approval_exhausted",
		approval.ReasonOperationMismatch: "operation_mismatch", approval.ReasonUseNotClaimed: "use_not_claimed",
		approval.ReasonIntrusiveNoRetry: "intrusive_no_retry", approval.ReasonAttemptsExhausted: "attempts_exhausted",
		approval.ReasonPreviousAttemptRunning: "previous_attempt_running", approval.ReasonConcurrencyExceeded: "concurrency_exceeded",
		approval.ReasonLimitsNotEstablished: "limits_not_established", approval.ReasonInvalidInput: "invalid_input",
	} {
		if string(r) != s {
			t.Errorf("reason %q, want %q", r, s)
		}
	}
}

func TestClaimConsumesOneUsePerJob(t *testing.T) {
	base := approvedLow(t)
	got, use, err := base.Claim("job-1", 7, base.Operation(), binding(), t0.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got.UsesConsumed() != 1 || got.Version() != base.Version()+1 {
		t.Errorf("after the claim: consumed %d, version %d", got.UsesConsumed(), got.Version())
	}
	if use.JobID() != "job-1" || use.ClaimEpoch() != 7 || use.Attempts() != 0 || !use.ConsumedAt().Equal(t0.Add(time.Second)) {
		t.Errorf("use = job %q epoch %d attempts %d at %v", use.JobID(), use.ClaimEpoch(), use.Attempts(), use.ConsumedAt())
	}
	if u, ok := got.UseFor("job-1"); !ok || u != use {
		t.Error("UseFor does not return the consumed use")
	}
	if _, ok := got.UseFor("job-2"); ok {
		t.Error("UseFor found a use that was never consumed")
	}
	// The receiver was not changed (value semantics).
	if base.UsesConsumed() != 0 || base.Version() != 2 {
		t.Error("Claim changed the receiver")
	}
}

// M2 acceptance: a retry claim reuses the record (S1.6 item 3, D-55).
func TestARetryClaimReusesTheOriginalUse(t *testing.T) {
	first := claim(t, approvedLow(t), "job-1")
	again, use, err := first.Claim("job-1", 2, first.Operation(), binding(), t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if use.ClaimEpoch() != 1 || !use.ConsumedAt().Equal(t0) {
		t.Errorf("the retry changed the original use: epoch %d, consumed %v", use.ClaimEpoch(), use.ConsumedAt())
	}
	if !reflect.DeepEqual(again, first) || again.UsesConsumed() != 1 || again.Version() != first.Version() {
		t.Error("a retry claim changed the approval or consumed another use")
	}
}

func TestClaimsStopAtMaxUsesButARetryStillWorks(t *testing.T) {
	a := approvedLow(t)
	for _, j := range []approval.JobID{"job-1", "job-2", "job-3", "job-4", "job-5"} {
		a = claim(t, a, j)
	}
	if a.UsesConsumed() != 5 {
		t.Fatalf("consumed %d, want 5", a.UsesConsumed())
	}
	_, _, err := a.Claim("job-6", 1, a.Operation(), binding(), t0)
	wantDenied(t, "sixth job", err, approval.ReasonExhausted)
	wantDenied(t, "CheckClaim of a sixth job", a.CheckClaim("job-6", a.Operation(), binding(), t0), approval.ReasonExhausted)
	// Five uses are five jobs. A job that already holds a use may retry.
	if _, _, err := a.Claim("job-3", 2, a.Operation(), binding(), t0); err != nil {
		t.Errorf("retry claim at exhaustion: %v", err)
	}
	// E1 refuses to attach an exhausted approval to a new job.
	wantDenied(t, "CheckUsable when exhausted", a.CheckUsable(a.Operation(), binding(), t0), approval.ReasonExhausted)
}

// M2 acceptance: expiry stops new launches only (D-48).
func TestExpiryStopsNewLaunchesOnly(t *testing.T) {
	a := claim(t, approvedLow(t), "job-1")
	exp := a.ExpiresAt()
	op := a.Operation()
	maxDur := op.Limits().MaxDuration

	if err := a.CheckLaunch("job-1", op, binding(), 0, true, exp.Add(-time.Nanosecond)); err != nil {
		t.Errorf("one nanosecond before expiry: %v", err)
	}
	for _, at := range []time.Time{exp, exp.Add(time.Nanosecond), exp.Add(time.Hour)} {
		wantDenied(t, "launch at "+at.Sub(exp).String(), a.CheckLaunch("job-1", op, binding(), 0, true, at), approval.ReasonExpired)
		_, _, err := a.Claim("job-9", 1, op, binding(), at)
		wantDenied(t, "claim at "+at.Sub(exp).String(), err, approval.ReasonExpired)
		wantDenied(t, "retry claim after expiry", a.CheckClaim("job-1", op, binding(), at), approval.ReasonExpired)
		wantDenied(t, "CheckUsable after expiry", a.CheckUsable(op, binding(), at), approval.ReasonExpired)
	}

	// A process launched before expiry keeps running to its own maximum
	// duration. Expiry neither stops it nor extends it. The launch time is the
	// one that AuthorizeLaunch recorded, so a caller cannot move it.
	launchedAt := exp.Add(-time.Minute)
	l, err := a.AuthorizeLaunch("job-1", op, binding(), 0, true, launchedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !l.RunningAllowed("job-1", binding(), exp.Add(time.Minute)) {
		t.Error("expiry stopped a launched operation")
	}
	if !l.RunningAllowed("job-1", binding(), launchedAt.Add(maxDur).Add(-time.Nanosecond)) {
		t.Error("the operation was stopped before its maximum duration")
	}
	if l.RunningAllowed("job-1", binding(), launchedAt.Add(maxDur)) {
		t.Error("the operation may run past its maximum duration")
	}
	if l.RunningAllowed("job-1", binding(), exp.Add(24*time.Hour)) {
		t.Error("expiry extended the maximum duration")
	}
}

// M2 acceptance: revocation.
func TestRevocationStopsClaimLaunchAndRunningWork(t *testing.T) {
	a := claim(t, approvedLow(t), "job-1")
	op := a.Operation()
	running, err := a.AuthorizeLaunch("job-1", op, binding(), 0, true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !running.RunningAllowed("job-1", binding(), t0.Add(time.Minute)) {
		t.Fatal("the launched operation is not allowed to run before the revocation")
	}
	revoked, err := running.Revoke(human(t), "scope changed", t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	later := t0.Add(2 * time.Minute)
	_, _, cerr := revoked.Claim("job-2", 1, op, binding(), later)
	wantDenied(t, "claim after revocation", cerr, approval.ReasonRevoked)
	wantDenied(t, "retry claim after revocation", revoked.CheckClaim("job-1", op, binding(), later), approval.ReasonRevoked)
	wantDenied(t, "launch after revocation", revoked.CheckLaunch("job-1", op, binding(), 0, true, later), approval.ReasonRevoked)
	wantDenied(t, "CheckUsable after revocation", revoked.CheckUsable(op, binding(), later), approval.ReasonRevoked)
	if revoked.RunningAllowed("job-1", binding(), later) {
		t.Error("a running operation is still authorized after revocation")
	}
	// M2 acceptance: a consumed use is not restored, not even by revocation.
	if revoked.UsesConsumed() != 1 {
		t.Errorf("revocation changed the consumed uses to %d", revoked.UsesConsumed())
	}
	// The approval before the revocation is unchanged. Revocation is a new value.
	if err := a.CheckLaunch("job-1", op, binding(), 0, true, later); err != nil {
		t.Errorf("the earlier value was changed: %v", err)
	}
}

func TestAnApprovalThatIsNotApprovedAuthorizesNothing(t *testing.T) {
	op := lowOp(t)
	pending := requested(t, op)
	rejected, err := pending.Reject(human(t), t0)
	if err != nil {
		t.Fatal(err)
	}
	for name, a := range map[string]approval.Approval{"pending": pending, "rejected": rejected, "zero": {}} {
		wantDenied(t, name+" CheckUsable", a.CheckUsable(op, binding(), t0), approval.ReasonNotApproved)
		wantDenied(t, name+" CheckClaim", a.CheckClaim("job-1", op, binding(), t0), approval.ReasonNotApproved)
		wantDenied(t, name+" CheckLaunch", a.CheckLaunch("job-1", op, binding(), 0, true, t0), approval.ReasonNotApproved)
		if _, _, err := a.Claim("job-1", 1, op, binding(), t0); err == nil {
			t.Errorf("%s: Claim succeeded", name)
		}
		if a.RunningAllowed("job-1", binding(), t0) {
			t.Errorf("%s: RunningAllowed", name)
		}
	}
}

// M2 acceptance: an intrusive operation has exactly one launch (D-49).
func TestAnIntrusiveOperationHasOneLaunch(t *testing.T) {
	a := approvedIntrusive(t)
	op := a.Operation()
	a = claim(t, a, "job-1")
	launched, err := a.AuthorizeLaunch("job-1", op, binding(), 0, true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := launched.UseFor("job-1"); u.Attempts() != 1 {
		t.Errorf("attempts = %d, want 1", u.Attempts())
	}
	_, err = launched.AuthorizeLaunch("job-1", op, binding(), 0, true, t0)
	wantDenied(t, "second launch", err, approval.ReasonIntrusiveNoRetry)
	// Neither a redelivered claim nor a lease expiry brings a second launch (D-49).
	_, _, err = launched.Claim("job-1", 2, op, binding(), t0)
	wantDenied(t, "retry claim of an intrusive job", err, approval.ReasonIntrusiveNoRetry)
	_, _, err = a.Claim("job-1", 2, op, binding(), t0) // even if the first launch never happened
	wantDenied(t, "retry claim before any launch", err, approval.ReasonIntrusiveNoRetry)
	// A rerun needs a new job and a new approval: the one use is spent.
	_, _, err = launched.Claim("job-2", 1, op, binding(), t0)
	wantDenied(t, "a second job under the same approval", err, approval.ReasonExhausted)
}

// M2 acceptance: a pre-launch denial is not an attempt and does not restore
// the use (S1.6 item 3, D-55).
func TestADenialBeforeLaunchIsNotAnAttemptAndRestoresNothing(t *testing.T) {
	a := claim(t, approvedLow(t), "job-1")
	op := a.Operation()
	late := a.ExpiresAt().Add(time.Second)

	// Many denials: expired, mismatched operation, too many running jobs.
	other := spec(operation.CategoryHTTPProbe)
	other.Params = map[string]string{"path_depth": "2"}
	for i := 0; i < 10; i++ {
		got, err := a.AuthorizeLaunch("job-1", op, binding(), 0, true, late)
		wantDenied(t, "expired launch", err, approval.ReasonExpired)
		if !reflect.DeepEqual(got, approval.Approval{}) {
			t.Error("a denied launch returned a usable approval")
		}
		_, err = a.AuthorizeLaunch("job-1", mustOp(t, other), binding(), 0, true, t0)
		wantDenied(t, "mismatched launch", err, approval.ReasonOperationMismatch)
		_, err = a.AuthorizeLaunch("job-1", op, binding(), op.Limits().MaxConcurrency, true, t0)
		wantDenied(t, "launch with every slot taken", err, approval.ReasonConcurrencyExceeded)
	}
	u, _ := a.UseFor("job-1")
	if u.Attempts() != 0 || a.UsesConsumed() != 1 {
		t.Fatalf("denials changed the state: attempts %d, consumed %d", u.Attempts(), a.UsesConsumed())
	}
	// Three real launches are still possible: the denials were not attempts.
	prev := false
	for i := 1; i <= 3; i++ {
		next, err := a.AuthorizeLaunch("job-1", op, binding(), 0, prev, t0)
		if err != nil {
			t.Fatalf("launch %d: %v", i, err)
		}
		a, prev = next, true
	}
	_, err := a.AuthorizeLaunch("job-1", op, binding(), 0, true, t0)
	wantDenied(t, "fourth launch", err, approval.ReasonAttemptsExhausted)
	wantDenied(t, "retry claim after three attempts", a.CheckClaim("job-1", op, binding(), t0), approval.ReasonAttemptsExhausted)
	if a.UsesConsumed() != 1 {
		t.Errorf("retries consumed more uses: %d", a.UsesConsumed())
	}
}

// S1.6 item 3 and D-64 choice 12: a retry launch waits for the previous
// attempt's process tree to be confirmed gone.
func TestARetryLaunchNeedsTheTreeOfTheLastAttemptGone(t *testing.T) {
	a := claim(t, approvedLow(t), "job-1")
	op := a.Operation()
	first, err := a.AuthorizeLaunch("job-1", op, binding(), 0, false, t0) // the first launch has no previous attempt
	if err != nil {
		t.Fatalf("first launch: %v", err)
	}
	_, err = first.AuthorizeLaunch("job-1", op, binding(), 0, false, t0)
	wantDenied(t, "retry with the tree not confirmed", err, approval.ReasonPreviousAttemptRunning)
	if _, err := first.AuthorizeLaunch("job-1", op, binding(), 0, true, t0); err != nil {
		t.Errorf("retry with the tree confirmed: %v", err)
	}
}

func TestALaunchNeedsAClaimedUse(t *testing.T) {
	a := approvedLow(t)
	_, err := a.AuthorizeLaunch("job-1", a.Operation(), binding(), 0, true, t0)
	wantDenied(t, "launch without a claim", err, approval.ReasonUseNotClaimed)
	// A use belongs to one job. Another job's use does not carry over.
	a = claim(t, a, "job-1")
	_, err = a.AuthorizeLaunch("job-2", a.Operation(), binding(), 0, true, t0)
	wantDenied(t, "launch of another job", err, approval.ReasonUseNotClaimed)
}

// FR-APPROVAL-001 AC1 and AC2 at claim and launch.
func TestAChangedOperationIsDeniedAtClaimAndLaunch(t *testing.T) {
	a := approvedLow(t)
	for name, edit := range map[string]func(*operation.Spec){
		"target":    func(s *operation.Spec) { s.Target = "other.example.com" },
		"port":      func(s *operation.Spec) { s.Port = 8443 },
		"parameter": func(s *operation.Spec) { s.Params = map[string]string{"path_depth": "2"} },
		"limit":     func(s *operation.Spec) { s.Limits.RequestsPerMinute = 600 },
		"category":  func(s *operation.Spec) { s.Category = operation.CategorySafeReplay },
		"method": func(s *operation.Spec) {
			s.Params = map[string]string{"path_depth": "1", operation.ParamHTTPMethod: "POST"}
		},
	} {
		s := spec(operation.CategoryHTTPProbe)
		edit(&s)
		other := mustOp(t, s)
		wantDenied(t, name+" CheckUsable", a.CheckUsable(other, binding(), t0), approval.ReasonOperationMismatch)
		_, _, err := a.Claim("job-1", 1, other, binding(), t0)
		wantDenied(t, name+" Claim", err, approval.ReasonOperationMismatch)
		claimed := claim(t, a, "job-1")
		wantDenied(t, name+" CheckLaunch", claimed.CheckLaunch("job-1", other, binding(), 0, true, t0), approval.ReasonOperationMismatch)
	}
	wantDenied(t, "zero operation", a.CheckUsable(operation.Operation{}, binding(), t0), approval.ReasonInvalidInput)
}

// FR-APPROVAL-006: jobs under one approval stay inside the approved concurrency.
func TestALaunchStaysInsideTheApprovedConcurrency(t *testing.T) {
	a := claim(t, approvedLow(t), "job-1")
	op := a.Operation() // MaxConcurrency 2
	if err := a.CheckLaunch("job-1", op, binding(), 1, true, t0); err != nil {
		t.Errorf("one other job running: %v", err)
	}
	wantDenied(t, "two other jobs running", a.CheckLaunch("job-1", op, binding(), 2, true, t0), approval.ReasonConcurrencyExceeded)
	wantDenied(t, "more running than approved", a.CheckLaunch("job-1", op, binding(), 9, true, t0), approval.ReasonConcurrencyExceeded)
	wantDenied(t, "negative running count", a.CheckLaunch("job-1", op, binding(), -1, true, t0), approval.ReasonInvalidInput)
}

func TestInvalidInputIsDenied(t *testing.T) {
	a := approvedLow(t)
	op := a.Operation()
	for _, j := range []approval.JobID{"", "job 1", approval.JobID(strings.Repeat("a", 65)), "-job"} {
		_, _, err := a.Claim(j, 1, op, binding(), t0)
		wantDenied(t, "claim with job id "+string(j), err, approval.ReasonInvalidInput)
		wantDenied(t, "CheckLaunch with job id "+string(j), a.CheckLaunch(j, op, binding(), 0, true, t0), approval.ReasonInvalidInput)
	}
	for _, e := range []int{0, -1} {
		_, _, err := a.Claim("job-1", e, op, binding(), t0)
		wantDenied(t, "claim with epoch", err, approval.ReasonInvalidInput)
	}
	_, _, err := a.Claim("job-1", 1, op, binding(), time.Time{})
	wantDenied(t, "claim with zero time", err, approval.ReasonInvalidInput)
	wantDenied(t, "CheckUsable with zero time", a.CheckUsable(op, binding(), time.Time{}), approval.ReasonInvalidInput)
}

// The checks hold the rules, and they change nothing.
func TestTheChecksDoNotChangeTheApproval(t *testing.T) {
	a := claim(t, approvedLow(t), "job-1")
	untouched := claim(t, approvedLow(t), "job-1") // built separately: nothing is shared with a
	op := a.Operation()
	_ = a.CheckUsable(op, binding(), t0)
	_ = a.CheckClaim("job-1", op, binding(), t0)
	_ = a.CheckClaim("job-9", op, binding(), t0)
	_ = a.CheckLaunch("job-1", op, binding(), 0, true, t0)
	_ = a.RunningAllowed("job-1", binding(), t0)
	if !reflect.DeepEqual(a, untouched) {
		t.Error("a check changed the approval")
	}
	u, _ := a.UseFor("job-1")
	if u.Attempts() != 0 {
		t.Error("a check recorded an attempt")
	}
}

// Two values derived from one approval never share a use list. Three uses make
// the backing array larger than the list, which is the case where a plain
// append would let two derived values write the same slot.
func TestApprovalsHaveValueSemantics(t *testing.T) {
	base := approvedLow(t)
	for _, j := range []approval.JobID{"job-1", "job-2", "job-3"} {
		base = claim(t, base, j)
	}
	x := claim(t, base, "job-4")
	y := claim(t, base, "job-5")
	ux, uy := x.Uses(), y.Uses()
	if len(ux) != 4 || len(uy) != 4 || ux[3].JobID() != "job-4" || uy[3].JobID() != "job-5" {
		t.Errorf("derived approvals share state: %v %v", ux, uy)
	}
	if base.UsesConsumed() != 3 || len(base.Uses()) != 3 {
		t.Error("the base approval was changed by a claim on a copy")
	}
	// The slice that Uses returns is a copy.
	ux[0] = approval.Use{}
	if u, ok := x.UseFor("job-1"); !ok || u.JobID() != "job-1" {
		t.Error("changing the slice from Uses changed the approval")
	}
	// An attempt on a derived value does not show on the original.
	launched, err := x.AuthorizeLaunch("job-1", x.Operation(), binding(), 0, true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := x.UseFor("job-1"); u.Attempts() != 0 {
		t.Error("an attempt on a derived value shows on the original")
	}
	if u, _ := base.UseFor("job-1"); u.Attempts() != 0 {
		t.Error("an attempt shows on the base approval")
	}
	if u, _ := launched.UseFor("job-1"); u.Attempts() != 1 {
		t.Error("the attempt was not recorded on the new value")
	}
}

func TestLaunchedAttemptsDoNotChangeTheVersion(t *testing.T) {
	// Attempts are stored on the job (job_attempts), not on the approval row,
	// so a launch does not touch the approval's optimistic-lock counter.
	a := claim(t, approvedLow(t), "job-1")
	launched, err := a.AuthorizeLaunch("job-1", a.Operation(), binding(), 0, true, t0)
	if err != nil || launched.Version() != a.Version() || launched.UsesConsumed() != a.UsesConsumed() {
		t.Errorf("launch: %v, version %d -> %d", err, a.Version(), launched.Version())
	}
}
