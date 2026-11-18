package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

func (b *browser) reauth(password, purpose string, subject map[string]any) *reply {
	return b.post("/api/v1/session/reauthenticate", map[string]any{"password": password, "purpose": purpose, "subject": subject})
}

func principalOf(t *testing.T, a *app, b *browser) inbound.Principal {
	t.Helper()
	p, err := a.srv.Auth().Authenticate(context.Background(), b.cookies["ms_session"])
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func hashOf(t *testing.T, p identity.Purpose, s map[string]any) [32]byte {
	t.Helper()
	h, err := identity.SubjectHash(p, s)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// D-69: a grant needs a fresh password entry and is bound to one session, one
// purpose and one subject. It is single use and short lived.
func TestReauthenticationGrantIsBoundAndSingleUse(t *testing.T) {
	_, a, b, _ := fresh(t)
	op := map[string]any{"operationHash": "aa11", "destinationsHash": "bb22"}

	if r := b.reauth(wrongPass, "approval_intrusive", op); r.status != 401 || r.code() != "invalid_credentials" {
		t.Fatalf("wrong password = %d %s", r.status, r.body)
	}
	r := b.reauth(goodPass, "approval_intrusive", op)
	if r.status != 200 || r.str("grantId") == "" || r.str("purpose") != "approval_intrusive" {
		t.Fatalf("reauthenticate = %d %s", r.status, r.body)
	}
	grant := r.str("grantId")
	exp, _ := time.Parse(time.RFC3339, r.str("expiresAt"))
	if d := time.Until(exp); d <= 0 || d > 121*time.Second {
		t.Errorf("the grant expires in %v, want at most 120 seconds", d)
	}
	if a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'auth.reauthenticated'`) != "2" {
		t.Error("the entry and the failed entry are not both audited")
	}

	reauth := a.srv.Auth()
	p := principalOf(t, a, b)
	use := func(grantID string, purpose identity.Purpose, subject map[string]any) error {
		return reauth.Consume(context.Background(), p, inbound.ReauthUse{GrantID: grantID, Purpose: purpose, SubjectHash: hashOf(t, purpose, subject)})
	}
	if err := use("", identity.PurposeApprovalIntrusive, op); !errors.Is(err, inbound.ErrReauthRequired) {
		t.Errorf("no grant: %v", err)
	}
	for name, c := range map[string]struct {
		purpose identity.Purpose
		subject map[string]any
	}{
		"another purpose": {identity.PurposeScopeWidening, op},
		"another subject": {identity.PurposeApprovalIntrusive, map[string]any{"operationHash": "aa11", "destinationsHash": "cc33"}},
	} {
		if err := use(grant, c.purpose, c.subject); !errors.Is(err, inbound.ErrReauthInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := use("00000000-0000-7000-8000-000000000000", identity.PurposeApprovalIntrusive, op); !errors.Is(err, inbound.ErrReauthInvalid) {
		t.Errorf("unknown grant: %v", err)
	}
	if err := use("1' OR '1'='1", identity.PurposeApprovalIntrusive, op); !errors.Is(err, inbound.ErrReauthInvalid) {
		t.Errorf("malformed grant id: %v", err)
	}

	// Another session of the same account cannot use it.
	other := newBrowser(a)
	other.login(username, goodPass)
	op2 := principalOf(t, a, other)
	if err := reauth.Consume(context.Background(), op2, inbound.ReauthUse{GrantID: grant, Purpose: identity.PurposeApprovalIntrusive, SubjectHash: hashOf(t, identity.PurposeApprovalIntrusive, op)}); !errors.Is(err, inbound.ErrReauthInvalid) {
		t.Errorf("another session: %v", err)
	}

	// A rolled back transaction does not spend the grant.
	rollback := errors.New("rollback")
	err := a.srv.Transactor().InTx(context.Background(), func(ctx context.Context) error {
		if err := reauth.Consume(ctx, p, inbound.ReauthUse{GrantID: grant, Purpose: identity.PurposeApprovalIntrusive, SubjectHash: hashOf(t, identity.PurposeApprovalIntrusive, op)}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction: %v", err)
	}
	if a.scalar(`SELECT count(*) FROM morningstar.reauth_grants WHERE consumed_at IS NOT NULL`) != "0" {
		t.Fatal("a rolled back transaction consumed the grant")
	}
	if err := use(grant, identity.PurposeApprovalIntrusive, op); err != nil {
		t.Fatalf("the exact grant was refused: %v", err)
	}
	if err := use(grant, identity.PurposeApprovalIntrusive, op); !errors.Is(err, inbound.ErrReauthInvalid) {
		t.Errorf("a second use: %v", err)
	}
}

func TestReauthenticationGrantExpiresAndDiesWithTheSession(t *testing.T) {
	_, a, b, _ := fresh(t)
	reauth := a.srv.Auth()
	p := principalOf(t, a, b)
	subject := map[string]any{"x": "1"}
	mint := func() string {
		r := b.reauth(goodPass, "verification", subject)
		if r.status != 200 {
			t.Fatalf("reauthenticate = %d %s", r.status, r.body)
		}
		return r.str("grantId")
	}
	use := func(id string) error {
		return reauth.Consume(context.Background(), p, inbound.ReauthUse{GrantID: id, Purpose: identity.PurposeVerification, SubjectHash: hashOf(t, identity.PurposeVerification, subject)})
	}
	expired := mint()
	a.exec(`UPDATE morningstar.reauth_grants SET expires_at = created_at + interval '1 millisecond' WHERE id = $1`, expired)
	if err := use(expired); !errors.Is(err, inbound.ErrReauthInvalid) {
		t.Errorf("an expired grant: %v", err)
	}
	live := mint()
	a.exec(`UPDATE morningstar.sessions SET revoked_at = now(), revoked_reason = 'user_revoked' WHERE id = $1`, p.SessionID())
	if err := use(live); !errors.Is(err, inbound.ErrReauthInvalid) {
		t.Errorf("a grant of a revoked session: %v", err)
	}
}

func TestReauthenticationRefusesBadInput(t *testing.T) {
	_, _, b, _ := fresh(t)
	cases := map[string]struct {
		purpose string
		subject any
		status  int
		code    string
	}{
		"unknown purpose":                {"approve_everything", map[string]any{}, 422, "validation_failed"},
		"empty purpose":                  {"", map[string]any{}, 422, "validation_failed"},
		"nested subject":                 {"verification", map[string]any{"a": map[string]any{"b": 1}}, 422, "reauth_subject_invalid"},
		"array subject":                  {"verification", map[string]any{"a": []any{1}}, 422, "reauth_subject_invalid"},
		"null value":                     {"verification", map[string]any{"a": nil}, 422, "reauth_subject_invalid"},
		"bad key":                        {"verification", map[string]any{"Bad Key": "x"}, 422, "reauth_subject_invalid"},
		"password change with a subject": {"password_change", map[string]any{"a": "b"}, 422, "reauth_subject_invalid"},
		"missing subject":                {"verification", nil, 422, "validation_failed"},
	}
	for name, c := range cases {
		body := map[string]any{"password": goodPass, "purpose": c.purpose}
		if c.subject != nil {
			body["subject"] = c.subject
		}
		if r := b.post("/api/v1/session/reauthenticate", body); r.status != c.status || r.code() != c.code {
			t.Errorf("%s = %d %s", name, r.status, r.body)
		}
	}
	if r := b.post("/api/v1/session/reauthenticate", `{"password":"x","purpose":"verification","subject":{},"extra":1}`); r.status != 400 {
		t.Errorf("unknown field = %d", r.status)
	}
}

// G-auth: "intrusive approval is refused without fresh password entry". The
// approve use case belongs to M19. This test drives the contract that it must
// call: the domain refuses an intrusive approval without a grant id, and the
// grant is accepted only when it is fresh, unspent, of this session and bound to
// the exact operation.
func TestIntrusiveApprovalIsRefusedWithoutAFreshPasswordEntry(t *testing.T) {
	_, a, b, _ := fresh(t)
	reauth := a.srv.Auth()
	p := principalOf(t, a, b)
	if !p.Actor().IsHuman() || p.Actor().ID() != p.UserID() || p.Actor().Kind() != actor.KindHuman {
		t.Fatalf("the principal's actor is %+v", p.Actor())
	}
	operation := map[string]any{"operationHash": strings.Repeat("ab", 32), "destinationsHash": strings.Repeat("cd", 32)}
	consume := func(grant string, subject map[string]any) error {
		return reauth.Consume(context.Background(), p, inbound.ReauthUse{GrantID: grant, Purpose: identity.PurposeApprovalIntrusive, SubjectHash: hashOf(t, identity.PurposeApprovalIntrusive, subject)})
	}
	if err := consume("", operation); !errors.Is(err, inbound.ErrReauthRequired) {
		t.Errorf("without a grant: %v", err)
	}
	// A password entry for one operation does not approve another.
	r := b.reauth(goodPass, "approval_intrusive", operation)
	other := map[string]any{"operationHash": strings.Repeat("ef", 32), "destinationsHash": strings.Repeat("cd", 32)}
	if err := consume(r.str("grantId"), other); !errors.Is(err, inbound.ErrReauthInvalid) {
		t.Errorf("a grant of another operation: %v", err)
	}
	if err := consume(r.str("grantId"), operation); err != nil {
		t.Errorf("the right grant: %v", err)
	}
	if err := consume(r.str("grantId"), operation); !errors.Is(err, inbound.ErrReauthInvalid) {
		t.Errorf("the same grant twice: %v", err)
	}
}
