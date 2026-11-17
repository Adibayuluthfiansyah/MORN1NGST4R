// Package security_test holds the security regression tests of the policy core
// (security.md 16, D-27). They drive the public API of the domain packages from
// outside, the way the application layer will, and they may import packages
// that the domain order table keeps apart (for example actor and scope).
package security_test

import (
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

var t0 = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func mustActor(t testing.TB, kind actor.Kind, id string) actor.Actor {
	t.Helper()
	a, err := actor.New(kind, id)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func human(t testing.TB) actor.Actor { return mustActor(t, actor.KindHuman, "user-1") }

func spec(cat operation.Category, target string) operation.Spec {
	return operation.Spec{
		AssessmentID: "asm-1", ToolKey: "httpx", Kind: "probe", Category: cat,
		Target: target, Port: 443,
		Params:         map[string]string{"path_depth": "1"},
		Destinations:   []string{target + ":443"},
		Limits:         operation.Limits{MaxConcurrency: 2, MaxDuration: 10 * time.Minute, RequestsPerMinute: 60},
		ManifestDigest: [32]byte{1},
	}
}

func mustOp(t testing.TB, s operation.Spec) operation.Operation {
	t.Helper()
	op, err := operation.New(s)
	if err != nil {
		t.Fatal(err)
	}
	return op
}

func rules(t testing.TB, domains ...string) scope.RuleSet {
	t.Helper()
	var rs scope.RuleSet
	for _, d := range domains {
		r, err := scope.NewRule(scope.KindDomain, false, d, "", scope.DefaultPrefixMin())
		if err != nil {
			t.Fatal(err)
		}
		rs = append(rs, r)
	}
	return rs
}

func binding() approval.Binding {
	return approval.Binding{ValidationID: "val-1", ScopeVersionID: "scv-1", PolicySnapshot: [32]byte{9}}
}

// world is the state the application layer would load before each decision.
type world struct {
	rules      scope.RuleSet
	validation scope.ValidationState
	policy     scope.PolicyState
}

func newWorld(t testing.TB, domains ...string) world {
	return world{
		rules:      rules(t, domains...),
		validation: scope.ValidationState{ID: "val-1", Current: true},
		policy:     scope.PolicyState{ScopeVersionID: "scv-1", PolicySnapshot: [32]byte{9}, IntrusiveEnabled: true},
	}
}

// binding is the state a new approval would be bound to right now.
func (w world) binding() approval.Binding {
	return approval.Binding{ValidationID: w.validation.ID, ScopeVersionID: w.policy.ScopeVersionID, PolicySnapshot: w.policy.PolicySnapshot}
}

func (w world) input(p scope.Point, op operation.Operation, ap *approval.Approval, job approval.JobID, now time.Time) scope.DecisionInput {
	return scope.DecisionInput{
		Point: p, DecisionID: "dec-" + p.String(), Now: now, Operation: op, Rules: w.rules,
		Validation: w.validation, Policy: w.policy, Approval: ap, Job: job,
		PreviousTreeConfirmed: true,
	}
}

// approved builds an approved approval for op with the default terms of S1.6.
func approved(t testing.TB, op operation.Operation) approval.Approval {
	t.Helper()
	a, err := approval.Request("apr-1", op, human(t), binding(), t0)
	if err != nil {
		t.Fatal(err)
	}
	terms, ok := approval.DefaultTerms(op.Class(), t0)
	if !ok {
		t.Fatal("no default terms")
	}
	if op.Class() == operation.ClassIntrusive {
		terms.ReauthGrantID = "grant-1"
	}
	a, err = a.Approve(human(t), terms, t0)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func reasons(v scope.Verdict) string {
	if len(v.Reasons) == 0 {
		return ""
	}
	return v.Reasons[0]
}
