package approval_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
)

// Review finding I3: the approval is bound to the scope version, the policy
// snapshot and the validation record it was made in (D-90, FR-SCOPE-006). The
// claim and the launch check hold that rule themselves, so a state change and
// the decision function cannot disagree about it.
func changedBindings() map[string]struct {
	b      approval.Binding
	reason approval.Reason
} {
	scopeChanged, policyChanged, validationChanged := binding(), binding(), binding()
	scopeChanged.ScopeVersionID = "scv-2"
	policyChanged.PolicySnapshot = [32]byte{10}
	validationChanged.ValidationID = "val-2"
	return map[string]struct {
		b      approval.Binding
		reason approval.Reason
	}{
		"scope version":   {scopeChanged, approval.ReasonScopeChanged},
		"policy snapshot": {policyChanged, approval.ReasonPolicyChanged},
		"validation":      {validationChanged, approval.ReasonValidationChanged},
	}
}

func TestACheckDeniesWhenTheStateItWasMadeInHasChanged(t *testing.T) {
	a := claim(t, approvedLow(t), "job-1")
	op := a.Operation()
	for name, c := range changedBindings() {
		wantDenied(t, name+" CheckUsable", a.CheckUsable(op, c.b, t0), c.reason)
		wantDenied(t, name+" CheckClaim", a.CheckClaim("job-2", op, c.b, t0), c.reason)
		wantDenied(t, name+" CheckClaim retry", a.CheckClaim("job-1", op, c.b, t0), c.reason)
		wantDenied(t, name+" CheckLaunch", a.CheckLaunch("job-1", op, c.b, 0, true, t0), c.reason)
		// A claim under a changed state consumes nothing.
		got, _, err := a.Claim("job-2", 1, op, c.b, t0)
		wantDenied(t, name+" Claim", err, c.reason)
		if !reflect.DeepEqual(got, approval.Approval{}) || a.UsesConsumed() != 1 {
			t.Errorf("%s: a refused claim changed something", name)
		}
		if _, err := a.AuthorizeLaunch("job-1", op, c.b, 0, true, t0); err == nil {
			t.Errorf("%s: a launch was recorded under a changed state", name)
		}
	}
	// The state the approval was made in works, and a missing binding denies.
	if err := a.CheckLaunch("job-1", op, binding(), 0, true, t0); err != nil {
		t.Errorf("the original binding: %v", err)
	}
	for name, b := range map[string]approval.Binding{
		"zero":               {},
		"no validation":      {ScopeVersionID: "scv-1", PolicySnapshot: [32]byte{9}},
		"no scope version":   {ValidationID: "val-1", PolicySnapshot: [32]byte{9}},
		"no policy snapshot": {ValidationID: "val-1", ScopeVersionID: "scv-1"},
		"malformed id":       {ValidationID: "val 1", ScopeVersionID: "scv-1", PolicySnapshot: [32]byte{9}},
	} {
		wantDenied(t, name+" binding", a.CheckUsable(op, b, t0), approval.ReasonInvalidInput)
	}
}

func TestBindingReasonsAreStable(t *testing.T) {
	for r, s := range map[approval.Reason]string{
		approval.ReasonScopeChanged:      "approval_scope_changed",
		approval.ReasonPolicyChanged:     "approval_policy_changed",
		approval.ReasonValidationChanged: "approval_validation_changed",
	} {
		if string(r) != s {
			t.Errorf("reason %q, want %q", r, s)
		}
	}
}

// Review finding I4: the launch time is recorded by AuthorizeLaunch. A caller
// cannot move it, so a heartbeat cannot extend the approved maximum duration.
func TestTheApprovedDurationRunsFromTheRecordedLaunch(t *testing.T) {
	a := claim(t, approvedLow(t), "job-1")
	op := a.Operation()
	maxDur := op.Limits().MaxDuration

	// Nothing is running before a launch.
	if a.RunningAllowed("job-1", binding(), t0) {
		t.Error("an operation that was never launched is reported as running")
	}
	first, err := a.AuthorizeLaunch("job-1", op, binding(), 0, true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := first.UseFor("job-1"); !u.LaunchedAt().Equal(t0) {
		t.Errorf("LaunchedAt = %v, want %v", u.LaunchedAt(), t0)
	}
	if !first.RunningAllowed("job-1", binding(), t0.Add(maxDur-time.Nanosecond)) || first.RunningAllowed("job-1", binding(), t0.Add(maxDur)) {
		t.Error("the duration does not run from the recorded launch time")
	}
	// A retry is a new launch with its own time and its own duration.
	retry, err := first.AuthorizeLaunch("job-1", op, binding(), 0, true, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if retry.RunningAllowed("job-1", binding(), t0.Add(maxDur)) == false || !retry.RunningAllowed("job-1", binding(), t0.Add(time.Hour+maxDur-time.Nanosecond)) {
		t.Error("the retry did not get its own duration")
	}
	if retry.RunningAllowed("job-1", binding(), t0.Add(time.Hour+maxDur)) {
		t.Error("the retry may run past its maximum duration")
	}
	// Another job under the same approval has no launch of its own.
	both := claim(t, first, "job-2")
	if both.RunningAllowed("job-2", binding(), t0) || both.RunningAllowed("job-3", binding(), t0) {
		t.Error("a job that never launched is reported as running")
	}
	if first.RunningAllowed("job-1", binding(), time.Time{}) {
		t.Error("a zero time was accepted")
	}
}

// S1.6 item 1: a scope change, a policy change or a stale validation
// invalidates the running authorization.
func TestAChangedStateStopsRunningWork(t *testing.T) {
	a := claim(t, approvedLow(t), "job-1")
	running, err := a.AuthorizeLaunch("job-1", a.Operation(), binding(), 0, true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !running.RunningAllowed("job-1", binding(), t0.Add(time.Minute)) {
		t.Fatal("the launched operation is not running")
	}
	for name, c := range changedBindings() {
		if running.RunningAllowed("job-1", c.b, t0.Add(time.Minute)) {
			t.Errorf("%s changed and the running operation is still authorized", name)
		}
	}
	if running.RunningAllowed("job-1", approval.Binding{}, t0.Add(time.Minute)) {
		t.Error("an empty binding keeps the running operation authorized")
	}
}

// Review finding M1: a time from time.Now carries a monotonic clock reading,
// which does not advance while the machine is suspended. Domain values drop it.
func TestStoredTimesCarryNoMonotonicReading(t *testing.T) {
	now := time.Now() // tests may read the clock. Domain code may not.
	op := lowOp(t)
	terms, _ := approval.DefaultTerms(op.Class(), now)
	a, err := approval.Request("apr-1", op, human(t), binding(), now)
	if err != nil {
		t.Fatal(err)
	}
	a, err = a.Approve(human(t), terms, now)
	if err != nil {
		t.Fatal(err)
	}
	a, use, err := a.Claim("job-1", 1, op, binding(), now)
	if err != nil {
		t.Fatal(err)
	}
	a, err = a.AuthorizeLaunch("job-1", op, binding(), 0, true, now)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := a.UseFor("job-1")
	for name, v := range map[string]time.Time{
		"created": a.CreatedAt(), "decided": a.DecidedAt(), "expires": a.ExpiresAt(), "consumed": use.ConsumedAt(), "launched": u.LaunchedAt(),
	} {
		if strings.Contains(v.String(), "m=") {
			t.Errorf("%s time %q carries a monotonic clock reading", name, v.String())
		}
	}
	// The wall-clock instant is what counts at the boundary of expiry.
	if a.Expired(a.ExpiresAt().Add(-time.Nanosecond)) || !a.Expired(a.ExpiresAt()) {
		t.Error("expiry boundary is wrong for times that came from time.Now")
	}
}
