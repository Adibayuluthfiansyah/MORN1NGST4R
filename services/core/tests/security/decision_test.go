package security_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand/v2"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

const target = "app.example.com"

// FR-APPROVAL-001 AC1 and AC2, FR-POLICY-001: an approval authorizes the exact
// operation it was made for.
func TestAnApprovalAuthorizesOnlyItsOwnOperation(t *testing.T) {
	w := newWorld(t, target, "other.example.com")
	opA := mustOp(t, spec(operation.CategoryHTTPProbe, target))
	ap := approved(t, opA)

	opB := mustOp(t, spec(operation.CategoryHTTPProbe, "other.example.com"))
	s := spec(operation.CategoryHTTPProbe, target)
	s.Params = map[string]string{"path_depth": "2"}
	changed := mustOp(t, s)
	s = spec(operation.CategoryHTTPProbe, target)
	s.Limits.RequestsPerMinute = 600
	moreRate := mustOp(t, s)

	for _, p := range []scope.Point{scope.PointE1, scope.PointE2, scope.PointE3} {
		claimed := ap
		if p == scope.PointE3 {
			var err error
			if claimed, _, err = ap.Claim("job-1", 1, opA, binding(), t0); err != nil {
				t.Fatal(err)
			}
		}
		if v := scope.Decide(w.input(p, opA, &claimed, "job-1", t0)); v.Outcome != scope.OutcomeAllow {
			t.Fatalf("%v: the approved operation: %v %v", p, v.Outcome, v.Reasons)
		}
		for name, other := range map[string]operation.Operation{"another target": opB, "a changed parameter": changed, "a higher rate": moreRate} {
			if v := scope.Decide(w.input(p, other, &claimed, "job-1", t0)); v.Outcome != scope.OutcomeDeny || reasons(v) != "operation_mismatch" {
				t.Errorf("%v with %s: %v %v, want deny operation_mismatch", p, name, v.Outcome, v.Reasons)
			}
		}
	}
}

// FR-SCOPE-006 and D-90: a changed scope version, policy snapshot or validation
// record makes an existing approval unusable.
func TestAnApprovalDoesNotSurviveAChangeOfTheStateItWasMadeIn(t *testing.T) {
	op := mustOp(t, spec(operation.CategoryHTTPProbe, target))
	base := approved(t, op)
	claimed, _, err := base.Claim("job-1", 1, op, binding(), t0)
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]struct {
		edit   func(*world)
		reason string
	}{
		"scope version":   {func(w *world) { w.policy.ScopeVersionID = "scv-2" }, "approval_scope_changed"},
		"policy snapshot": {func(w *world) { w.policy.PolicySnapshot = [32]byte{10} }, "approval_policy_changed"},
		"validation":      {func(w *world) { w.validation.ID = "val-2" }, "approval_validation_changed"},
	}
	for name, c := range changes {
		w := newWorld(t, target)
		c.edit(&w)
		for _, p := range []scope.Point{scope.PointE1, scope.PointE2, scope.PointE3} {
			ap := base
			if p == scope.PointE3 {
				ap = claimed
			}
			if v := scope.Decide(w.input(p, op, &ap, "job-1", t0)); v.Outcome != scope.OutcomeDeny || reasons(v) != c.reason {
				t.Errorf("%s changed, %v: %v %v, want deny %s", name, p, v.Outcome, v.Reasons, c.reason)
			}
		}
	}
	// A stale validation denies even when the ids match.
	w := newWorld(t, target)
	w.validation.Current = false
	if v := scope.Decide(w.input(scope.PointE3, op, &claimed, "job-1", t0)); v.Outcome != scope.OutcomeDeny || reasons(v) != "validation_not_current" {
		t.Errorf("stale validation: %v %v", v.Outcome, v.Reasons)
	}
}

// FR-POLICY-001 AC2: narrowing scope after scheduling makes the launch check deny.
func TestNarrowingScopeAfterSchedulingDeniesTheLaunch(t *testing.T) {
	op := mustOp(t, spec(operation.CategoryHTTPProbe, target))
	ap := approved(t, op)
	w := newWorld(t, target)
	if v := scope.Decide(w.input(scope.PointE1, op, &ap, "", t0)); v.Outcome != scope.OutcomeAllow {
		t.Fatalf("E1: %v %v", v.Outcome, v.Reasons)
	}
	ap, _, err := ap.Claim("job-1", 1, op, binding(), t0)
	if err != nil {
		t.Fatal(err)
	}
	narrowed := w
	exc, err := scope.NewRule(scope.KindDomain, true, target, "", scope.DefaultPrefixMin())
	if err != nil {
		t.Fatal(err)
	}
	narrowed.rules = append(append(scope.RuleSet(nil), w.rules...), exc)
	if v := scope.Decide(narrowed.input(scope.PointE3, op, &ap, "job-1", t0)); v.Outcome != scope.OutcomeDeny || reasons(v) != "excluded" {
		t.Errorf("E3 after an exclusion: %v %v", v.Outcome, v.Reasons)
	}
	gone := w
	gone.rules = nil
	if v := scope.Decide(gone.input(scope.PointE3, op, &ap, "job-1", t0)); v.Outcome != scope.OutcomeDeny || reasons(v) != "no_matching_rule" {
		t.Errorf("E3 with no rules: %v %v", v.Outcome, v.Reasons)
	}
}

func TestEveryApprovalStateDeniesWithItsCodeAtEveryPoint(t *testing.T) {
	w := newWorld(t, target)
	op := mustOp(t, spec(operation.CategoryHTTPProbe, target))
	pending, err := approval.Request("apr-1", op, human(t), binding(), t0)
	if err != nil {
		t.Fatal(err)
	}
	rejected, _ := pending.Reject(human(t), t0)
	live := approved(t, op)
	revoked, _ := live.Revoke(human(t), "withdrawn", t0)
	exhausted := live
	for _, j := range []approval.JobID{"job-1", "job-2", "job-3", "job-4", "job-5"} {
		if exhausted, _, err = exhausted.Claim(j, 1, op, binding(), t0); err != nil {
			t.Fatal(err)
		}
	}
	late := live.ExpiresAt().Add(time.Second)

	type tc struct {
		name   string
		ap     approval.Approval
		now    time.Time
		job    approval.JobID
		point  scope.Point
		reason string
	}
	cases := []tc{
		{"rejected", rejected, t0, "job-9", scope.PointE1, "approval_not_approved"},
		{"rejected", rejected, t0, "job-9", scope.PointE2, "approval_not_approved"},
		{"rejected", rejected, t0, "job-9", scope.PointE3, "approval_not_approved"},
		{"pending", pending, t0, "job-9", scope.PointE2, "approval_not_approved"},
		{"pending", pending, t0, "job-9", scope.PointE3, "approval_not_approved"},
		{"revoked", revoked, t0, "job-9", scope.PointE1, "approval_revoked"},
		{"revoked", revoked, t0, "job-9", scope.PointE2, "approval_revoked"},
		{"revoked", revoked, t0, "job-9", scope.PointE3, "approval_revoked"},
		{"expired", live, late, "job-9", scope.PointE1, "approval_expired"},
		{"expired", live, late, "job-9", scope.PointE2, "approval_expired"},
		{"expired", live, late, "job-9", scope.PointE3, "approval_expired"},
		{"exhausted", exhausted, t0, "job-9", scope.PointE1, "approval_exhausted"},
		{"exhausted", exhausted, t0, "job-9", scope.PointE2, "approval_exhausted"},
		{"not claimed", live, t0, "job-9", scope.PointE3, "use_not_claimed"},
	}
	for _, c := range cases {
		ap := c.ap
		v := scope.Decide(w.input(c.point, op, &ap, c.job, c.now))
		if v.Outcome != scope.OutcomeDeny || reasons(v) != c.reason || !v.Authorized.IsZero() {
			t.Errorf("%s at %v: %v %v authorized=%v, want deny %s", c.name, c.point, v.Outcome, v.Reasons, !v.Authorized.IsZero(), c.reason)
		}
	}
	// A pending approval at E1 is a request for approval, not a refusal.
	if v := scope.Decide(w.input(scope.PointE1, op, &pending, "", t0)); v.Outcome != scope.OutcomeRequireApproval || reasons(v) != "approval_not_approved" {
		t.Errorf("pending at E1: %v %v", v.Outcome, v.Reasons)
	}
}

func TestTheAuthorizationNeverOutlivesTheApproval(t *testing.T) {
	w := newWorld(t, target)
	op := mustOp(t, spec(operation.CategoryHTTPProbe, target))
	ap := approved(t, op)
	ap, _, err := ap.Claim("job-1", 1, op, binding(), t0)
	if err != nil {
		t.Fatal(err)
	}
	// Two seconds before the approval expires, which is sooner than the TTL.
	at := ap.ExpiresAt().Add(-2 * time.Second)
	v := scope.Decide(w.input(scope.PointE3, op, &ap, "job-1", at))
	if v.Outcome != scope.OutcomeAllow {
		t.Fatalf("%v %v", v.Outcome, v.Reasons)
	}
	if !v.Authorized.ExpiresAt().Equal(ap.ExpiresAt()) {
		t.Errorf("expiry %v, want the approval's %v", v.Authorized.ExpiresAt(), ap.ExpiresAt())
	}
	if err := v.Authorized.Verify(op, "job-1", ap.ExpiresAt()); err == nil {
		t.Error("the authorization is valid at the instant the approval expires")
	}
	// Far from expiry the TTL applies.
	early := scope.Decide(w.input(scope.PointE3, op, &ap, "job-1", t0))
	if !early.Authorized.ExpiresAt().Equal(t0.Add(scope.AuthorizationTTL)) {
		t.Errorf("expiry %v, want now + TTL", early.Authorized.ExpiresAt())
	}
}

// A passive operation is decided on scope alone. An approval that is attached
// to it changes nothing, good or bad.
func TestAnApprovalAttachedToAPassiveOperationIsIgnored(t *testing.T) {
	w := newWorld(t, target)
	passive := mustOp(t, spec(operation.CategoryPassiveRecon, target))
	active := mustOp(t, spec(operation.CategoryHTTPProbe, target))
	ap := approved(t, active)
	revoked, _ := ap.Revoke(human(t), "withdrawn", t0)
	for _, a := range []*approval.Approval{nil, &ap, &revoked} {
		if v := scope.Decide(w.input(scope.PointE3, passive, a, "job-1", t0)); v.Outcome != scope.OutcomeAllow {
			t.Errorf("passive with approval %v: %v %v", a != nil, v.Outcome, v.Reasons)
		}
	}
}

// P-04: the only function of the scope package that produces an authorization
// is Decide. This reads the source, so a new exported constructor fails the test.
func TestNoExportedFunctionOfTheScopePackageReturnsAnAuthorization(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "domain", "scope")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files in %s: %v", dir, err)
	}
	var returnsVerdict []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() || fn.Type.Results == nil {
				continue
			}
			for _, r := range fn.Type.Results.List {
				name := ""
				switch x := r.Type.(type) {
				case *ast.Ident:
					name = x.Name
				case *ast.StarExpr:
					if id, ok := x.X.(*ast.Ident); ok {
						name = id.Name
					}
				}
				switch name {
				case "AuthorizedOperation":
					t.Errorf("%s: exported %s returns an AuthorizedOperation", filepath.Base(f), fn.Name.Name)
				case "Verdict":
					returnsVerdict = append(returnsVerdict, fn.Name.Name)
				}
			}
		}
	}
	sort.Strings(returnsVerdict)
	if len(returnsVerdict) != 1 || returnsVerdict[0] != "Decide" {
		t.Errorf("exported functions that return a Verdict = %v, want only Decide", returnsVerdict)
	}
}

// The decision function and the state change must agree: whenever Decide
// allows E2 the claim succeeds, whenever it allows E3 the launch is recorded,
// and whenever Decide denies for an approval reason the state change denies
// too. They share the Check functions, and this walk proves it on random
// sequences, intrusive and low-impact, with changing time and state.
func TestTheDecisionAndTheStateChangeNeverDisagree(t *testing.T) {
	jobs := []approval.JobID{"job-1", "job-2", "job-3", "job-4", "job-5", "job-6"}
	for run := 0; run < 200; run++ {
		rng := rand.New(rand.NewPCG(uint64(run), 11))
		cat := operation.CategoryHTTPProbe
		if run%4 == 0 {
			cat = operation.CategoryStateChangingReplay
		}
		op := mustOp(t, spec(cat, target))
		ap := approved(t, op)
		w := newWorld(t, target)
		now := t0
		for step := 0; step < 100; step++ {
			job := jobs[rng.IntN(len(jobs))]
			switch rng.IntN(6) {
			case 0, 1: // claim through E2
				v := scope.Decide(w.input(scope.PointE2, op, &ap, job, now))
				next, _, err := ap.Claim(job, 1+rng.IntN(4), op, w.binding(), now)
				if (v.Outcome == scope.OutcomeAllow) != (err == nil) {
					t.Fatalf("run %d step %d: E2 decision %v %v but claim error %v", run, step, v.Outcome, v.Reasons, err)
				}
				if err == nil {
					ap = next
				}
			case 2, 3: // launch through E3
				running := rng.IntN(3)
				in := w.input(scope.PointE3, op, &ap, job, now)
				in.RunningJobs, in.PreviousTreeConfirmed = running, rng.IntN(2) == 0
				v := scope.Decide(in)
				next, err := ap.AuthorizeLaunch(job, op, w.binding(), running, in.PreviousTreeConfirmed, now)
				if (v.Outcome == scope.OutcomeAllow) != (err == nil) {
					t.Fatalf("run %d step %d: E3 decision %v %v but launch error %v", run, step, v.Outcome, v.Reasons, err)
				}
				if err == nil {
					if v.Authorized.Verify(op, job, now) != nil {
						t.Fatalf("run %d step %d: an E3 allow does not verify", run, step)
					}
					ap = next
				} else if !v.Authorized.IsZero() {
					t.Fatalf("run %d step %d: a denial carries an authorization", run, step)
				}
			case 5: // the scope version, the policy snapshot or the validation moves on, or comes back
				switch rng.IntN(3) {
				case 0:
					w.policy.ScopeVersionID = map[string]string{"scv-1": "scv-2", "scv-2": "scv-1"}[w.policy.ScopeVersionID]
				case 1:
					w.policy.PolicySnapshot[1] ^= 1
				case 2:
					w.validation.ID = map[string]string{"val-1": "val-2", "val-2": "val-1"}[w.validation.ID]
				}
			case 4:
				now = now.Add(time.Duration(rng.IntN(6*3600)) * time.Second)
				if rng.IntN(20) == 0 && ap.Status() == approval.StatusApproved {
					if revoked, err := ap.Revoke(human(t), "random", now); err == nil {
						ap = revoked
					}
				}
			}
		}
	}
}
