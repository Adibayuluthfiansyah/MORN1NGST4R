package approval_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/approval"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

func TestZeroApprovalIsNothing(t *testing.T) {
	var a approval.Approval
	if a.Status() != approval.StatusInvalid || a.Status().String() != "invalid" || !a.Operation().IsZero() || a.UsesConsumed() != 0 {
		t.Errorf("zero approval: status %v", a.Status())
	}
}

// FR-APPROVAL-001 AC3 and the M2 acceptance: an AI actor cannot approve. Every
// transition refuses every non-human kind, and the approval stays as it was.
func TestOnlyAHumanDecidesAnApproval(t *testing.T) {
	op := lowOp(t)
	terms, _ := approval.DefaultTerms(op.Class(), t0)
	for name, who := range map[string]actor.Actor{"ai": ai(t), "system": system(t), "zero": {}} {
		if a, err := approval.Request("apr-1", op, who, binding(), t0); !errors.Is(err, approval.ErrActorNotHuman) || a.Status() != approval.StatusInvalid {
			t.Errorf("%s: Request = %v, %v, want ErrActorNotHuman", name, a.Status(), err)
		}
		pending := requested(t, op)
		if a, err := pending.Approve(who, terms, t0); !errors.Is(err, approval.ErrActorNotHuman) || !reflect.DeepEqual(a, approval.Approval{}) {
			t.Errorf("%s: Approve = %v, %v, want ErrActorNotHuman and a zero value", name, a.Status(), err)
		}
		if _, err := pending.Reject(who, t0); !errors.Is(err, approval.ErrActorNotHuman) {
			t.Errorf("%s: Reject = %v, want ErrActorNotHuman", name, err)
		}
		approved := approvedLow(t)
		if _, err := approved.Revoke(who, "no longer needed", t0); !errors.Is(err, approval.ErrActorNotHuman) {
			t.Errorf("%s: Revoke = %v, want ErrActorNotHuman", name, err)
		}
		if pending.Status() != approval.StatusPending || approved.Status() != approval.StatusApproved {
			t.Errorf("%s: a refused call changed an approval", name)
		}
	}
}

func TestRequestRefusesWhatCannotBeApproved(t *testing.T) {
	// Passive operations need no approval (D-45, constraint c2 of the schema).
	if _, err := approval.Request("apr-1", passiveOp(t), human(t), binding(), t0); !errors.Is(err, approval.ErrNotApprovable) {
		t.Errorf("passive: %v, want ErrNotApprovable", err)
	}
	// FR-APPROVAL-006 AC2 and D-71: no numbers, no approval.
	s := spec(operation.CategoryHTTPProbe)
	s.Limits = operation.Limits{}
	if _, err := approval.Request("apr-1", mustOp(t, s), human(t), binding(), t0); !errors.Is(err, approval.ErrLimitsNotEstablished) {
		t.Errorf("no limits: %v, want ErrLimitsNotEstablished", err)
	}
	if _, err := approval.Request("apr-1", operation.Operation{}, human(t), binding(), t0); !errors.Is(err, approval.ErrInvalid) {
		t.Errorf("zero operation: %v, want ErrInvalid", err)
	}
	bad := []struct {
		name string
		id   string
		b    approval.Binding
		now  time.Time
	}{
		{"empty id", "", binding(), t0},
		{"id with a space", "apr 1", binding(), t0},
		{"no validation id", "apr-1", approval.Binding{ScopeVersionID: "scv-1", PolicySnapshot: [32]byte{9}}, t0},
		{"no scope version", "apr-1", approval.Binding{ValidationID: "val-1", PolicySnapshot: [32]byte{9}}, t0},
		{"zero policy snapshot", "apr-1", approval.Binding{ValidationID: "val-1", ScopeVersionID: "scv-1"}, t0},
		{"zero time", "apr-1", binding(), time.Time{}},
	}
	for _, tc := range bad {
		if a, err := approval.Request(tc.id, lowOp(t), human(t), tc.b, tc.now); !errors.Is(err, approval.ErrInvalid) || a.Status() != approval.StatusInvalid {
			t.Errorf("%s: Request = %v, %v, want ErrInvalid", tc.name, a.Status(), err)
		}
	}
}

// FR-APPROVAL-001 AC1 and AC2: an approval is bound to the exact operation.
func TestAnApprovalHoldsTheExactOperation(t *testing.T) {
	a := requested(t, lowOp(t))
	if a.Operation().Hash() != lowOp(t).Hash() || a.Operation().Target() != "app.example.com" || a.Class() != operation.ClassLowImpact {
		t.Error("the approval does not hold the operation it was requested for")
	}
	s := spec(operation.CategoryHTTPProbe)
	s.Target = "other.example.com"
	b := requested(t, mustOp(t, s))
	if a.Operation().Hash() == b.Operation().Hash() {
		t.Error("two targets share one operation hash")
	}
	s = spec(operation.CategoryHTTPProbe)
	s.Params = map[string]string{"path_depth": "2"}
	if requested(t, mustOp(t, s)).Operation().Hash() == a.Operation().Hash() {
		t.Error("a changed parameter did not change the hash the approval holds")
	}
	if a.ID() != "apr-1" || a.Status() != approval.StatusPending || a.Version() != 1 || !a.CreatedAt().Equal(t0) {
		t.Errorf("requested approval: id %q status %v version %d", a.ID(), a.Status(), a.Version())
	}
	if a.RequestedBy().ID() != "user-1" || !a.RequestedBy().IsHuman() || !a.DecidedBy().IsZero() {
		t.Error("requester or decider recorded wrongly")
	}
	if b := a.Binding(); b.ValidationID != "val-1" || b.ScopeVersionID != "scv-1" || b.PolicySnapshot != [32]byte{9} {
		t.Errorf("binding = %+v", b)
	}
}

func TestApproveRecordsTheTerms(t *testing.T) {
	a := approvedLow(t)
	if a.Status() != approval.StatusApproved || a.Version() != 2 || a.DecidedBy().ID() != "user-1" || !a.DecidedAt().Equal(t0) {
		t.Errorf("approved: status %v version %d", a.Status(), a.Version())
	}
	if !a.ExpiresAt().Equal(t0.Add(24*time.Hour)) || a.MaxUses() != 5 || a.MaxAttempts() != 3 || a.UsesConsumed() != 0 {
		t.Errorf("terms: expires %v uses %d attempts %d consumed %d", a.ExpiresAt(), a.MaxUses(), a.MaxAttempts(), a.UsesConsumed())
	}
	i := approvedIntrusive(t)
	if i.ReauthGrantID() != "grant-1" || i.MaxUses() != 1 || i.MaxAttempts() != 1 || !i.ExpiresAt().Equal(t0.Add(15*time.Minute)) {
		t.Errorf("intrusive terms: grant %q uses %d attempts %d expires %v", i.ReauthGrantID(), i.MaxUses(), i.MaxAttempts(), i.ExpiresAt())
	}
}

// S1.6 item 6 and D-45: the starting values.
func TestDefaultTerms(t *testing.T) {
	low, ok := approval.DefaultTerms(operation.ClassLowImpact, t0)
	if !ok || low.MaxUses != 5 || low.MaxAttempts != 3 || !low.ExpiresAt.Equal(t0.Add(24*time.Hour)) || low.ReauthGrantID != "" {
		t.Errorf("low-impact defaults = %+v, %v", low, ok)
	}
	in, ok := approval.DefaultTerms(operation.ClassIntrusive, t0)
	if !ok || in.MaxUses != 1 || in.MaxAttempts != 1 || !in.ExpiresAt.Equal(t0.Add(15*time.Minute)) {
		t.Errorf("intrusive defaults = %+v, %v", in, ok)
	}
	for _, c := range []operation.RiskClass{operation.ClassPassive, operation.ClassUnknown, operation.RiskClass(9)} {
		if got, ok := approval.DefaultTerms(c, t0); ok || got != (approval.Terms{}) {
			t.Errorf("DefaultTerms(%v) = %+v, %v, want nothing", c, got, ok)
		}
	}
}

func TestApproveRefusesBadTerms(t *testing.T) {
	good := func() approval.Terms {
		return approval.Terms{ExpiresAt: t0.Add(time.Hour), MaxUses: 2, MaxAttempts: 3}
	}
	lowCases := []struct {
		name string
		edit func(*approval.Terms)
	}{
		{"expiry equal to now", func(x *approval.Terms) { x.ExpiresAt = t0 }},
		{"expiry in the past", func(x *approval.Terms) { x.ExpiresAt = t0.Add(-time.Second) }},
		{"no expiry", func(x *approval.Terms) { x.ExpiresAt = time.Time{} }},
		{"zero uses", func(x *approval.Terms) { x.MaxUses = 0 }},
		{"negative uses", func(x *approval.Terms) { x.MaxUses = -1 }},
		{"zero attempts", func(x *approval.Terms) { x.MaxAttempts = 0 }},
		{"four attempts", func(x *approval.Terms) { x.MaxAttempts = 4 }},
		{"negative attempts", func(x *approval.Terms) { x.MaxAttempts = -1 }},
		{"bad grant id", func(x *approval.Terms) { x.ReauthGrantID = "g r" }},
	}
	for _, tc := range lowCases {
		terms := good()
		tc.edit(&terms)
		if a, err := requested(t, lowOp(t)).Approve(human(t), terms, t0); !errors.Is(err, approval.ErrBadTerms) || a.Status() != approval.StatusInvalid {
			t.Errorf("low-impact %s: %v, want ErrBadTerms", tc.name, err)
		}
	}
	// D-45, D-49 and constraint c5: an intrusive approval is one use and one attempt.
	for _, tc := range []struct {
		name  string
		edit  func(*approval.Terms)
		wants error
	}{
		{"two uses", func(x *approval.Terms) { x.MaxUses = 2 }, approval.ErrBadTerms},
		{"two attempts", func(x *approval.Terms) { x.MaxAttempts = 2 }, approval.ErrBadTerms},
		{"three attempts", func(x *approval.Terms) { x.MaxAttempts = 3 }, approval.ErrBadTerms},
		{"no grant (D-58, D-69)", func(x *approval.Terms) { x.ReauthGrantID = "" }, approval.ErrReauthRequired},
	} {
		terms := approval.Terms{ExpiresAt: t0.Add(time.Minute), MaxUses: 1, MaxAttempts: 1, ReauthGrantID: "grant-1"}
		tc.edit(&terms)
		if _, err := requested(t, intrusiveOp(t)).Approve(human(t), terms, t0); !errors.Is(err, tc.wants) {
			t.Errorf("intrusive %s: %v, want %v", tc.name, err, tc.wants)
		}
	}
	if _, err := requested(t, lowOp(t)).Approve(human(t), good(), time.Time{}); !errors.Is(err, approval.ErrInvalid) {
		t.Errorf("zero time: %v, want ErrInvalid", err)
	}
	// A low-impact approval needs no grant.
	if _, err := requested(t, lowOp(t)).Approve(human(t), good(), t0); err != nil {
		t.Errorf("low-impact without a grant: %v", err)
	}
}

func TestOnlyTheDefinedTransitionsAreAllowed(t *testing.T) {
	pending := requested(t, lowOp(t))
	approved := approvedLow(t)
	rejected, err := pending.Reject(human(t), t0)
	if err != nil || rejected.Status() != approval.StatusRejected || rejected.Version() != 2 || !rejected.DecidedAt().Equal(t0) {
		t.Fatalf("Reject = %v, %v", rejected.Status(), err)
	}
	revoked, err := approved.Revoke(human(t), "scope changed", t0.Add(time.Minute))
	if err != nil || revoked.Status() != approval.StatusRevoked || revoked.Version() != 3 ||
		!revoked.RevokedAt().Equal(t0.Add(time.Minute)) || revoked.RevokedBy().ID() != "user-1" || revoked.RevokeReason() != "scope changed" {
		t.Fatalf("Revoke = %v, %v", revoked.Status(), err)
	}
	terms, _ := approval.DefaultTerms(operation.ClassLowImpact, t0)
	steps := map[string]func() error{
		"approve an approved": func() error { _, err := approved.Approve(human(t), terms, t0); return err },
		"approve a rejected":  func() error { _, err := rejected.Approve(human(t), terms, t0); return err },
		"approve a revoked":   func() error { _, err := revoked.Approve(human(t), terms, t0); return err },
		"reject an approved":  func() error { _, err := approved.Reject(human(t), t0); return err },
		"reject a rejected":   func() error { _, err := rejected.Reject(human(t), t0); return err },
		"reject a revoked":    func() error { _, err := revoked.Reject(human(t), t0); return err },
		"revoke a pending":    func() error { _, err := pending.Revoke(human(t), "x", t0); return err },
		"revoke a rejected":   func() error { _, err := rejected.Revoke(human(t), "x", t0); return err },
		"revoke a revoked":    func() error { _, err := revoked.Revoke(human(t), "x", t0); return err },
		"approve the zero":    func() error { _, err := (approval.Approval{}).Approve(human(t), terms, t0); return err },
		"reject the zero":     func() error { _, err := (approval.Approval{}).Reject(human(t), t0); return err },
		"revoke the zero":     func() error { _, err := (approval.Approval{}).Revoke(human(t), "x", t0); return err },
	}
	for name, step := range steps {
		if err := step(); !errors.Is(err, approval.ErrBadTransition) {
			t.Errorf("%s: %v, want ErrBadTransition", name, err)
		}
	}
}

func TestRevokeNeedsAReason(t *testing.T) {
	a := approvedLow(t)
	for name, reason := range map[string]string{
		"empty": "", "newline": "a\nb", "NUL": "a\x00b", "DEL": "a\x7fb", "not UTF-8": "a\xffb", "513 bytes": strings.Repeat("a", 513),
	} {
		if _, err := a.Revoke(human(t), reason, t0); !errors.Is(err, approval.ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if _, err := a.Revoke(human(t), strings.Repeat("a", 512), t0); err != nil {
		t.Errorf("a 512 byte reason: %v", err)
	}
	if _, err := a.Revoke(human(t), "reason", time.Time{}); !errors.Is(err, approval.ErrInvalid) {
		t.Errorf("zero time: %v, want ErrInvalid", err)
	}
}

func TestExpiryIsDerivedAndTheInstantOfExpiryIsExpired(t *testing.T) {
	a := approvedLow(t)
	exp := a.ExpiresAt()
	if a.Expired(exp.Add(-time.Nanosecond)) || !a.Expired(exp) || !a.Expired(exp.Add(time.Nanosecond)) {
		t.Error("expiry boundary: the approval must be live one nanosecond before ExpiresAt and expired at it")
	}
	if requested(t, lowOp(t)).Expired(t0.Add(1000 * time.Hour)) {
		t.Error("a pending approval has no expiry")
	}
}

func TestStatusNames(t *testing.T) {
	for s, n := range map[approval.Status]string{
		approval.StatusPending: "pending", approval.StatusApproved: "approved", approval.StatusRejected: "rejected",
		approval.StatusRevoked: "revoked", approval.StatusInvalid: "invalid", approval.Status(42): "invalid",
	} {
		if s.String() != n {
			t.Errorf("Status(%d).String() = %q, want %q", s, s.String(), n)
		}
	}
}
