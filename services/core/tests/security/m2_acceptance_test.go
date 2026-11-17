package security_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// TestM2Acceptance has one subtest for each sentence of the M2 acceptance
// criteria in the plan. Each drives the public API from request to verified
// authorization, the way the application layer and the worker will.
func TestM2Acceptance(t *testing.T) {
	const target = "app.example.com"
	w := newWorld(t, target)

	t.Run("expiry stops new launches only", func(t *testing.T) {
		op := mustOp(t, spec(operation.CategoryHTTPProbe, target))
		ap := approved(t, op)
		ap, _, err := ap.Claim("job-1", 1, op, binding(), t0)
		if err != nil {
			t.Fatal(err)
		}
		before := ap.ExpiresAt().Add(-time.Minute)
		if v := scope.Decide(w.input(scope.PointE3, op, &ap, "job-1", before)); v.Outcome != scope.OutcomeAllow {
			t.Fatalf("launch before expiry: %v %v", v.Outcome, v.Reasons)
		}
		launched, err := ap.AuthorizeLaunch("job-1", op, binding(), 0, true, before)
		if err != nil {
			t.Fatal(err)
		}
		// At and after expiry no new launch and no new claim is allowed.
		for _, at := range []time.Time{ap.ExpiresAt(), ap.ExpiresAt().Add(time.Hour)} {
			for _, p := range []scope.Point{scope.PointE1, scope.PointE2, scope.PointE3} {
				if v := scope.Decide(w.input(p, op, &launched, "job-1", at)); v.Outcome != scope.OutcomeDeny || reasons(v) != "approval_expired" {
					t.Errorf("%v at expiry+%v: %v %v", p, at.Sub(ap.ExpiresAt()), v.Outcome, v.Reasons)
				}
			}
		}
		// The launched operation is not stopped by expiry and not extended by it.
		if !launched.RunningAllowed("job-1", binding(), ap.ExpiresAt().Add(time.Minute)) {
			t.Error("expiry stopped a launched operation")
		}
		if launched.RunningAllowed("job-1", binding(), before.Add(op.Limits().MaxDuration)) {
			t.Error("the operation outlived its approved maximum duration")
		}
	})

	t.Run("revocation", func(t *testing.T) {
		op := mustOp(t, spec(operation.CategoryHTTPProbe, target))
		ap, _, err := approved(t, op).Claim("job-1", 1, op, binding(), t0)
		if err != nil {
			t.Fatal(err)
		}
		ap, err = ap.AuthorizeLaunch("job-1", op, binding(), 0, true, t0)
		if err != nil {
			t.Fatal(err)
		}
		revoked, err := ap.Revoke(human(t), "scope was withdrawn", t0.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []scope.Point{scope.PointE1, scope.PointE2, scope.PointE3} {
			if v := scope.Decide(w.input(p, op, &revoked, "job-1", t0.Add(2*time.Minute))); v.Outcome != scope.OutcomeDeny || reasons(v) != "approval_revoked" {
				t.Errorf("%v after revocation: %v %v", p, v.Outcome, v.Reasons)
			}
		}
		if revoked.RunningAllowed("job-1", binding(), t0.Add(2*time.Minute)) {
			t.Error("a running operation stays authorized after revocation")
		}
	})

	t.Run("an intrusive operation has one launch", func(t *testing.T) {
		op := mustOp(t, spec(operation.CategoryStateChangingReplay, target))
		ap := approved(t, op)
		if v := scope.Decide(w.input(scope.PointE2, op, &ap, "job-1", t0)); v.Outcome != scope.OutcomeAllow {
			t.Fatalf("E2: %v %v", v.Outcome, v.Reasons)
		}
		ap, _, err := ap.Claim("job-1", 1, op, binding(), t0)
		if err != nil {
			t.Fatal(err)
		}
		if v := scope.Decide(w.input(scope.PointE3, op, &ap, "job-1", t0)); v.Outcome != scope.OutcomeAllow {
			t.Fatalf("first launch: %v %v", v.Outcome, v.Reasons)
		}
		ap, err = ap.AuthorizeLaunch("job-1", op, binding(), 0, true, t0)
		if err != nil {
			t.Fatal(err)
		}
		if v := scope.Decide(w.input(scope.PointE3, op, &ap, "job-1", t0)); v.Outcome != scope.OutcomeDeny || reasons(v) != "intrusive_no_retry" {
			t.Errorf("second launch: %v %v", v.Outcome, v.Reasons)
		}
		// A redelivered claim after a lease expiry is refused too (D-49).
		if v := scope.Decide(w.input(scope.PointE2, op, &ap, "job-1", t0)); v.Outcome != scope.OutcomeDeny || reasons(v) != "intrusive_no_retry" {
			t.Errorf("retry claim: %v %v", v.Outcome, v.Reasons)
		}
	})

	t.Run("a pre-launch denial is not an attempt", func(t *testing.T) {
		op := mustOp(t, spec(operation.CategoryHTTPProbe, target))
		ap, _, err := approved(t, op).Claim("job-1", 1, op, binding(), t0)
		if err != nil {
			t.Fatal(err)
		}
		busy := w.input(scope.PointE3, op, &ap, "job-1", t0)
		busy.RunningJobs = op.Limits().MaxConcurrency
		for i := 0; i < 5; i++ {
			if v := scope.Decide(busy); v.Outcome != scope.OutcomeDeny || reasons(v) != "concurrency_exceeded" {
				t.Fatalf("denial %d: %v %v", i, v.Outcome, v.Reasons)
			}
		}
		if u, _ := ap.UseFor("job-1"); u.Attempts() != 0 || ap.UsesConsumed() != 1 {
			t.Fatalf("denials changed the state: %d attempts, %d uses", u.Attempts(), ap.UsesConsumed())
		}
		// All three attempts are still there.
		for i := 1; i <= 3; i++ {
			ap, err = ap.AuthorizeLaunch("job-1", op, binding(), 0, true, t0)
			if err != nil {
				t.Fatalf("attempt %d: %v", i, err)
			}
		}
	})

	t.Run("a consumed use is not restored", func(t *testing.T) {
		op := mustOp(t, spec(operation.CategoryHTTPProbe, target))
		ap := approved(t, op)
		var err error
		for _, j := range []approval.JobID{"job-1", "job-2", "job-3", "job-4", "job-5"} {
			if ap, _, err = ap.Claim(j, 1, op, binding(), t0); err != nil {
				t.Fatal(err)
			}
		}
		// A denied launch, a failed claim and a revocation give nothing back.
		_ = ap.CheckLaunch("job-1", op, binding(), 9, true, t0)
		if _, _, err := ap.Claim("job-6", 1, op, binding(), t0); err == nil {
			t.Fatal("a sixth job was admitted")
		}
		revoked, err := ap.Revoke(human(t), "done", t0)
		if err != nil {
			t.Fatal(err)
		}
		if ap.UsesConsumed() != 5 || revoked.UsesConsumed() != 5 {
			t.Errorf("uses consumed: %d and %d, want 5 and 5", ap.UsesConsumed(), revoked.UsesConsumed())
		}
		if v := scope.Decide(w.input(scope.PointE2, op, &ap, "job-6", t0)); v.Outcome != scope.OutcomeDeny || reasons(v) != "approval_exhausted" {
			t.Errorf("sixth job at E2: %v %v", v.Outcome, v.Reasons)
		}
	})

	t.Run("a retry claim reuses the record", func(t *testing.T) {
		op := mustOp(t, spec(operation.CategoryHTTPProbe, target))
		ap := approved(t, op)
		var err error
		for _, j := range []approval.JobID{"job-1", "job-2", "job-3", "job-4", "job-5"} {
			if ap, _, err = ap.Claim(j, 1, op, binding(), t0); err != nil {
				t.Fatal(err)
			}
		}
		// Every use is consumed, and the job that holds one may still be claimed again.
		if v := scope.Decide(w.input(scope.PointE2, op, &ap, "job-3", t0)); v.Outcome != scope.OutcomeAllow {
			t.Fatalf("retry claim at E2: %v %v", v.Outcome, v.Reasons)
		}
		again, use, err := ap.Claim("job-3", 4, op, binding(), t0.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if again.UsesConsumed() != 5 || again.Version() != ap.Version() || use.ClaimEpoch() != 1 {
			t.Errorf("a retry claim consumed or changed something: uses %d, version %d -> %d, epoch %d",
				again.UsesConsumed(), ap.Version(), again.Version(), use.ClaimEpoch())
		}
	})

	t.Run("a zero-value AuthorizedOperation is rejected", func(t *testing.T) {
		var zero scope.AuthorizedOperation
		op := mustOp(t, spec(operation.CategoryPassiveRecon, target))
		if err := zero.Verify(op, "job-1", t0); !errors.Is(err, scope.ErrZeroAuthorization) {
			t.Errorf("Verify(zero) = %v", err)
		}
		// A denial leaves the field at its zero value, so a launcher that takes
		// the verdict as it is refuses to launch.
		denied := scope.Decide(w.input(scope.PointE3, mustOp(t, spec(operation.CategoryPassiveRecon, "other.example.org")), nil, "job-1", t0))
		if denied.Outcome != scope.OutcomeDeny || denied.Authorized.Verify(op, "job-1", t0) == nil {
			t.Error("a denial produced an authorization that verifies")
		}
		// An allowed decision verifies for its own operation only.
		allowed := scope.Decide(w.input(scope.PointE3, op, nil, "job-1", t0))
		if allowed.Outcome != scope.OutcomeAllow || allowed.Authorized.Verify(op, "job-1", t0) != nil {
			t.Fatalf("a passive operation in scope: %v %v", allowed.Outcome, allowed.Reasons)
		}
	})

	t.Run("an AI actor cannot approve", func(t *testing.T) {
		op := mustOp(t, spec(operation.CategoryStateChangingReplay, target))
		ai := mustActor(t, actor.KindAI, "ai-1")
		worker := mustActor(t, actor.KindSystem, "worker-1")
		terms, _ := approval.DefaultTerms(op.Class(), t0)
		terms.ReauthGrantID = "grant-1"
		for name, who := range map[string]actor.Actor{"ai": ai, "worker": worker} {
			if _, err := approval.Request("apr-1", op, who, binding(), t0); !errors.Is(err, approval.ErrActorNotHuman) {
				t.Errorf("%s requested an approval: %v", name, err)
			}
			pending, err := approval.Request("apr-1", op, human(t), binding(), t0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pending.Approve(who, terms, t0); !errors.Is(err, approval.ErrActorNotHuman) {
				t.Errorf("%s approved: %v", name, err)
			}
			if _, err := pending.Reject(who, t0); !errors.Is(err, approval.ErrActorNotHuman) {
				t.Errorf("%s rejected: %v", name, err)
			}
			// Nothing was approved, so the decision function still asks for one.
			if v := scope.Decide(w.input(scope.PointE1, op, &pending, "", t0)); v.Outcome != scope.OutcomeRequireApproval {
				t.Errorf("%s: after a refused approval the decision was %v %v", name, v.Outcome, v.Reasons)
			}
		}
	})
}
