package scope_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// The decision function (P-03) and its AuthorizedOperation (P-04), tested
// without an approval. The paths that need an approval are in
// tests/security: building an approval needs an actor, and the order table of
// structure.md 5.6 does not let the scope package import actor, even in tests.

var dcNow = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func dcSpec(cat operation.Category) operation.Spec {
	return operation.Spec{
		AssessmentID: "asm-1", ToolKey: "httpx", Kind: "probe", Category: cat,
		Target: "app.example.com", Port: 443,
		Params:         map[string]string{"path_depth": "1"},
		Destinations:   []string{"app.example.com:443"},
		Limits:         operation.Limits{MaxConcurrency: 2, MaxDuration: 10 * time.Minute, RequestsPerMinute: 60},
		ManifestDigest: [32]byte{1},
	}
}

func dcOp(t testing.TB, s operation.Spec) operation.Operation {
	t.Helper()
	op, err := operation.New(s)
	if err != nil {
		t.Fatal(err)
	}
	return op
}

func dcInput(t *testing.T, cat operation.Category) scope.DecisionInput {
	t.Helper()
	return scope.DecisionInput{
		Point:      scope.PointE1,
		DecisionID: "dec-1",
		Job:        "job-1",
		Now:        dcNow,
		Operation:  dcOp(t, dcSpec(cat)),
		Rules:      build(t, inc(scope.KindDomain, "app.example.com", "")),
		Validation: scope.ValidationState{ID: "val-1", Current: true},
		Policy:     scope.PolicyState{ScopeVersionID: "scv-1", PolicySnapshot: [32]byte{9}, IntrusiveEnabled: true},
	}
}

func wantDeny(t testing.TB, what string, v scope.Verdict, reason string) {
	t.Helper()
	if v.Outcome != scope.OutcomeDeny || !v.Authorized.IsZero() {
		t.Errorf("%s: outcome %v, authorized %v, want a deny with no authorization", what, v.Outcome, !v.Authorized.IsZero())
	}
	if len(v.Reasons) != 1 || v.Reasons[0] != reason {
		t.Errorf("%s: reasons %v, want [%s]", what, v.Reasons, reason)
	}
}

// FR-POLICY-001 AC3: missing data, an error or the zero value is a deny.
func TestTheZeroVerdictAndTheZeroInputDeny(t *testing.T) {
	var v scope.Verdict
	if v.Outcome != scope.OutcomeDeny || !v.Authorized.IsZero() {
		t.Error("the zero Verdict must be a deny without an authorization")
	}
	wantDeny(t, "the zero input", scope.Decide(scope.DecisionInput{}), "invalid_input")
}

func TestMissingOrBadInputDenies(t *testing.T) {
	cases := map[string]func(*scope.DecisionInput){
		"unknown point":            func(in *scope.DecisionInput) { in.Point = scope.PointInvalid },
		"point out of range":       func(in *scope.DecisionInput) { in.Point = scope.Point(9) },
		"no decision id":           func(in *scope.DecisionInput) { in.DecisionID = "" },
		"decision id with a space": func(in *scope.DecisionInput) { in.DecisionID = "dec 1" },
		"decision id too long":     func(in *scope.DecisionInput) { in.DecisionID = string(make([]byte, 65)) },
		"zero time":                func(in *scope.DecisionInput) { in.Now = time.Time{} },
		"zero operation":           func(in *scope.DecisionInput) { in.Operation = operation.Operation{} },
		"no validation id":         func(in *scope.DecisionInput) { in.Validation.ID = "" },
		"no scope version":         func(in *scope.DecisionInput) { in.Policy.ScopeVersionID = "" },
		"zero policy snapshot":     func(in *scope.DecisionInput) { in.Policy.PolicySnapshot = [32]byte{} },
	}
	for name, edit := range cases {
		in := dcInput(t, operation.CategoryHTTPProbe)
		edit(&in)
		wantDeny(t, name, scope.Decide(in), "invalid_input")
	}
	// A decision for a claim or a launch is for a job. E1 comes before the job exists.
	for _, p := range []scope.Point{scope.PointE2, scope.PointE3} {
		for _, j := range []approval.JobID{"", "job 1", approval.JobID(string(make([]byte, 65)))} {
			for _, cat := range []operation.Category{operation.CategoryPassiveRecon, operation.CategoryHTTPProbe} {
				in := dcInput(t, cat)
				in.Point, in.Job = p, j
				wantDeny(t, "job "+string(j)+" at "+p.String(), scope.Decide(in), "invalid_input")
			}
		}
	}
	e1 := dcInput(t, operation.CategoryPassiveRecon)
	e1.Job = ""
	if v := scope.Decide(e1); v.Outcome != scope.OutcomeAllow || v.Authorized.Job() != "" {
		t.Errorf("E1 without a job: %v %v", v.Outcome, v.Reasons)
	}

	// A passive operation is decided on scope alone, so the active-only fields may be empty.
	in := dcInput(t, operation.CategoryPassiveRecon)
	in.Validation, in.Policy = scope.ValidationState{}, scope.PolicyState{}
	if v := scope.Decide(in); v.Outcome != scope.OutcomeAllow {
		t.Errorf("passive without validation and policy: %v %v", v.Outcome, v.Reasons)
	}
}

// Scope is evaluated by the M1 engine. Its reasons pass through unchanged.
func TestTheScopeDecisionIsNotOverridden(t *testing.T) {
	in := dcInput(t, operation.CategoryHTTPProbe)

	in.Rules = build(t, inc(scope.KindDomain, "other.example.com", ""))
	wantDeny(t, "no matching rule", scope.Decide(in), scope.ReasonNoMatchingRule)

	in.Rules = build(t, inc(scope.KindDomain, "app.example.com", ""), exc(scope.KindDomain, "app.example.com", ""))
	wantDeny(t, "excluded", scope.Decide(in), scope.ReasonExcluded)

	in.Rules = build(t, inc(scope.KindDomain, "app.example.com", "8443"))
	wantDeny(t, "a port the rule does not cover", scope.Decide(in), scope.ReasonNoMatchingRule)

	in.Rules = nil
	wantDeny(t, "no rules", scope.Decide(in), scope.ReasonNoMatchingRule)

	in.Rules = scope.RuleSet{{}} // a rule that was never built
	wantDeny(t, "an invalid rule", scope.Decide(in), scope.ReasonInvalidRuleSet)

	// A deny rule for an operation of any class, passive included.
	p := dcInput(t, operation.CategoryPassiveRecon)
	p.Rules = nil
	wantDeny(t, "passive out of scope", scope.Decide(p), scope.ReasonNoMatchingRule)
}

// The operation's target must be exactly what the scope engine would build
// from it. A form that only parses after repair does not pass.
func TestANonCanonicalOperationTargetDenies(t *testing.T) {
	for _, target := range []string{"APP.example.com", "app.example.com.", "127.1", "::ffff:0:1.2.3.4", "2130706433", "app_example.com", "a..b", "[::1]", "::ffff:127.0.0.1", "app.example.com:443"} {
		s := dcSpec(operation.CategoryHTTPProbe)
		s.Target = target
		in := dcInput(t, operation.CategoryHTTPProbe)
		in.Operation = dcOp(t, s)
		in.Rules = build(t, inc(scope.KindDomain, "app.example.com", ""))
		if v := scope.Decide(in); v.Outcome != scope.OutcomeDeny || len(v.Reasons) != 1 || v.Reasons[0] != scope.ReasonInvalidTarget {
			t.Errorf("target %q: %v %v, want deny invalid_target", target, v.Outcome, v.Reasons)
		}
	}
	// An IP literal in canonical form is a target like any other.
	s := dcSpec(operation.CategoryHTTPProbe)
	s.Target = "203.0.113.7"
	in := dcInput(t, operation.CategoryHTTPProbe)
	in.Operation = dcOp(t, s)
	in.Rules = build(t, inc(scope.KindIP, "203.0.113.7", ""))
	in.Policy.IntrusiveEnabled = false
	if v := scope.Decide(in); v.Outcome != scope.OutcomeRequireApproval {
		t.Errorf("canonical IP target: %v %v", v.Outcome, v.Reasons)
	}
}

// S1.6 item 6: a passive operation needs policy evaluation and nothing else.
func TestAPassiveOperationInScopeIsAllowedWithoutApproval(t *testing.T) {
	for _, p := range []scope.Point{scope.PointE1, scope.PointE2, scope.PointE3} {
		in := dcInput(t, operation.CategoryPassiveRecon)
		in.Point = p
		v := scope.Decide(in)
		if v.Outcome != scope.OutcomeAllow || len(v.Reasons) != 1 || v.Reasons[0] != scope.ReasonAllowed {
			t.Fatalf("point %v: %v %v", p, v.Outcome, v.Reasons)
		}
		a := v.Authorized
		if a.IsZero() || a.OperationHash() != in.Operation.Hash() || a.DecisionID() != "dec-1" || a.Point() != p ||
			!a.ExpiresAt().Equal(dcNow.Add(scope.AuthorizationTTL)) {
			t.Errorf("point %v: authorization %v %v %v", p, a.OperationHash(), a.Point(), a.ExpiresAt())
		}
	}
}

// FR-APPROVAL-001: an active operation is never allowed without an approval.
func TestAnActiveOperationNeedsAnApproval(t *testing.T) {
	for _, cat := range []operation.Category{operation.CategoryHTTPProbe, operation.CategoryStateChangingReplay} {
		in := dcInput(t, cat)
		v := scope.Decide(in)
		if v.Outcome != scope.OutcomeRequireApproval || !v.Authorized.IsZero() || len(v.Reasons) != 1 || v.Reasons[0] != scope.ReasonApprovalRequired {
			t.Errorf("%v at E1: %v %v", cat, v.Outcome, v.Reasons)
		}
		for _, p := range []scope.Point{scope.PointE2, scope.PointE3} {
			in.Point = p
			wantDeny(t, cat.String()+" at "+p.String(), scope.Decide(in), scope.ReasonApprovalRequired)
		}
	}
}

// FR-SCOPE-005 AC1 and FR-SCOPE-006: a missing or stale validation denies.
func TestAnActiveOperationNeedsACurrentValidation(t *testing.T) {
	in := dcInput(t, operation.CategoryHTTPProbe)
	in.Validation.Current = false
	for _, p := range []scope.Point{scope.PointE1, scope.PointE2, scope.PointE3} {
		in.Point = p
		wantDeny(t, "stale validation at "+p.String(), scope.Decide(in), scope.ReasonValidationNotCurrent)
	}
	in.Validation = scope.ValidationState{ID: "val-1"} // never validated
	wantDeny(t, "no validation", scope.Decide(in), scope.ReasonValidationNotCurrent)
}

// FR-APPROVAL-006 AC2 and D-71.
func TestAnActiveOperationWithoutEstablishedLimitsIsBlocked(t *testing.T) {
	for _, l := range []operation.Limits{{}, {MaxConcurrency: 2, MaxDuration: time.Minute}, {MaxConcurrency: 5, MaxDuration: time.Minute, RequestsPerMinute: 4}} {
		s := dcSpec(operation.CategoryHTTPProbe)
		s.Limits = l
		in := dcInput(t, operation.CategoryHTTPProbe)
		in.Operation = dcOp(t, s)
		wantDeny(t, "limits", scope.Decide(in), scope.ReasonLimitsNotEstablished)
	}
	// A passive operation has no limits to establish.
	s := dcSpec(operation.CategoryPassiveRecon)
	s.Limits = operation.Limits{}
	in := dcInput(t, operation.CategoryPassiveRecon)
	in.Operation = dcOp(t, s)
	if v := scope.Decide(in); v.Outcome != scope.OutcomeAllow {
		t.Errorf("passive without limits: %v %v", v.Outcome, v.Reasons)
	}
}

// D-58: intrusive operations stay disabled until authentication and
// re-authentication are tested. The zero PolicyState is disabled.
func TestIntrusiveOperationsAreDisabledByDefault(t *testing.T) {
	in := dcInput(t, operation.CategoryStateChangingReplay)
	in.Policy.IntrusiveEnabled = false
	for _, p := range []scope.Point{scope.PointE1, scope.PointE2, scope.PointE3} {
		in.Point = p
		wantDeny(t, "intrusive at "+p.String(), scope.Decide(in), scope.ReasonIntrusiveDisabled)
	}
	in.Policy = scope.PolicyState{}
	if v := scope.Decide(in); v.Outcome != scope.OutcomeDeny {
		t.Errorf("a zero PolicyState allowed an intrusive operation: %v", v.Outcome)
	}
	// The switch does not affect low-impact operations.
	low := dcInput(t, operation.CategoryHTTPProbe)
	low.Policy.IntrusiveEnabled = false
	if v := scope.Decide(low); v.Outcome != scope.OutcomeRequireApproval {
		t.Errorf("low-impact with the switch off: %v %v", v.Outcome, v.Reasons)
	}
	// A state-changing method raises a probe to intrusive and the switch applies.
	s := dcSpec(operation.CategoryHTTPProbe)
	s.Params = map[string]string{operation.ParamHTTPMethod: "DELETE"}
	raised := dcInput(t, operation.CategoryHTTPProbe)
	raised.Operation = dcOp(t, s)
	raised.Policy.IntrusiveEnabled = false
	wantDeny(t, "a DELETE probe", scope.Decide(raised), scope.ReasonIntrusiveDisabled)
}

func TestDecideIsAPureFunction(t *testing.T) {
	in := dcInput(t, operation.CategoryPassiveRecon)
	hashBefore := in.Operation.Hash()
	first, second := scope.Decide(in), scope.Decide(in)
	if !reflect.DeepEqual(first, second) {
		t.Error("equal inputs gave different verdicts")
	}
	if in.Operation.Hash() != hashBefore || in.Now != dcNow {
		t.Error("Decide changed its input")
	}
	// The reasons slice is the caller's to keep.
	first.Reasons[0] = "changed"
	if scope.Decide(in).Reasons[0] != scope.ReasonAllowed {
		t.Error("a returned slice is shared between verdicts")
	}
}

func TestOutcomeAndPointNames(t *testing.T) {
	for o, n := range map[scope.Outcome]string{scope.OutcomeDeny: "deny", scope.OutcomeAllow: "allow", scope.OutcomeRequireApproval: "require_approval", scope.Outcome(9): "deny"} {
		if o.String() != n {
			t.Errorf("Outcome(%d) = %q, want %q", o, o.String(), n)
		}
	}
	for p, n := range map[scope.Point]string{scope.PointE1: "E1", scope.PointE2: "E2", scope.PointE3: "E3", scope.PointInvalid: "invalid", scope.Point(9): "invalid"} {
		if p.String() != n {
			t.Errorf("Point(%d) = %q, want %q", p, p.String(), n)
		}
	}
}
