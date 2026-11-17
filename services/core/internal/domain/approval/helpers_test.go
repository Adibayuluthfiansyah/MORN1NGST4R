package approval_test

import (
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

// t0 is a fixed instant. The package reads no clock, so every test passes time in.
var t0 = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func mustActor(t testing.TB, kind actor.Kind, id string) actor.Actor {
	t.Helper()
	a, err := actor.New(kind, id)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func human(t testing.TB) actor.Actor  { return mustActor(t, actor.KindHuman, "user-1") }
func ai(t testing.TB) actor.Actor     { return mustActor(t, actor.KindAI, "ai-1") }
func system(t testing.TB) actor.Actor { return mustActor(t, actor.KindSystem, "worker-1") }

func goodLimits() operation.Limits {
	return operation.Limits{MaxConcurrency: 2, MaxDuration: 10 * time.Minute, RequestsPerMinute: 60}
}

func spec(cat operation.Category) operation.Spec {
	return operation.Spec{
		AssessmentID: "asm-1", ToolKey: "httpx", Kind: "probe", Category: cat,
		Target: "app.example.com", Port: 443,
		Params:         map[string]string{"path_depth": "1"},
		Destinations:   []string{"app.example.com:443"},
		Limits:         goodLimits(),
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

func lowOp(t testing.TB) operation.Operation { return mustOp(t, spec(operation.CategoryHTTPProbe)) }
func intrusiveOp(t testing.TB) operation.Operation {
	return mustOp(t, spec(operation.CategoryStateChangingReplay))
}
func passiveOp(t testing.TB) operation.Operation {
	return mustOp(t, spec(operation.CategoryPassiveRecon))
}

func binding() approval.Binding {
	return approval.Binding{ValidationID: "val-1", ScopeVersionID: "scv-1", PolicySnapshot: [32]byte{9}}
}

func requested(t testing.TB, op operation.Operation) approval.Approval {
	t.Helper()
	a, err := approval.Request("apr-1", op, human(t), binding(), t0)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// approvedLow is a low-impact approval with the default terms of S1.6.
func approvedLow(t testing.TB) approval.Approval {
	t.Helper()
	op := lowOp(t)
	terms, ok := approval.DefaultTerms(op.Class(), t0)
	if !ok {
		t.Fatal("no default terms for a low-impact operation")
	}
	a, err := requested(t, op).Approve(human(t), terms, t0)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// approvedIntrusive is an intrusive approval with the default terms and a grant.
func approvedIntrusive(t testing.TB) approval.Approval {
	t.Helper()
	op := intrusiveOp(t)
	terms, ok := approval.DefaultTerms(op.Class(), t0)
	if !ok {
		t.Fatal("no default terms for an intrusive operation")
	}
	terms.ReauthGrantID = "grant-1"
	a, err := requested(t, op).Approve(human(t), terms, t0)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
