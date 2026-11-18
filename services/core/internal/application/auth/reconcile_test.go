package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

type reconcileHasher struct {
	outbound.PasswordHasher
	match  bool
	hashes []string
}

func (h *reconcileHasher) Verify(context.Context, string, string) (bool, error) { return h.match, nil }
func (h *reconcileHasher) Hash(_ context.Context, secret string) (string, error) {
	h.hashes = append(h.hashes, secret)
	return "phc:" + secret, nil
}

type recordingRecovery struct {
	inbound.Recovery
	state inbound.RecoveryState
	calls []inbound.ReconcileRequest
}

func (r *recordingRecovery) State() inbound.RecoveryState { return r.state }
func (r *recordingRecovery) Reconcile(_ context.Context, req inbound.ReconcileRequest) (inbound.ReconcileCounts, error) {
	r.calls = append(r.calls, req)
	return inbound.ReconcileCounts{Generation: 2}, nil
}

type reconcileAccounts struct{ resetAccounts }

func (reconcileAccounts) CurrentPasswordHash(context.Context, string) (string, bool, error) {
	return "phc:restored", true, nil
}

type reconcileFixture struct {
	svc      *Service
	hasher   *reconcileHasher
	recovery *recordingRecovery
	attempts *fakeAttempts
}

func newReconcileFixture(match bool) reconcileFixture {
	h := &reconcileHasher{match: match}
	r := &recordingRecovery{state: inbound.RecoveryState{Required: true}}
	a := &fakeAttempts{allow: true}
	ids := 0
	svc := New(Deps{
		Accounts: reconcileAccounts{}, Hasher: h, Recovery: r, Attempts: a, Audit: &fakeAudit{},
		Random: counterRandom(), NewID: func() string { ids++; return "id" + string(rune('0'+ids)) },
	})
	return reconcileFixture{svc, h, r, a}
}

func reconcileInput(old, fresh string) inbound.ReconcileInput {
	return inbound.ReconcileInput{Password: old, NewPassword: fresh, ConfirmRevocation: true, ClientKey: "loopback", RequestID: "r1"}
}

// The new password is hashed only for a caller who proved the restored one.
func TestReconcileHashesTheNewPasswordOnlyAfterTheOldOneIsVerified(t *testing.T) {
	f := newReconcileFixture(false)
	_, err := f.svc.Reconcile(context.Background(), reconcileInput("the restored passphrase 1", "a brand new passphrase 22"))
	if !errors.Is(err, inbound.ErrInvalidCredentials) {
		t.Fatalf("err = %v", err)
	}
	if len(f.hasher.hashes) != 0 || len(f.recovery.calls) != 0 {
		t.Fatalf("hashed %d values and reconciled %d times for a wrong password", len(f.hasher.hashes), len(f.recovery.calls))
	}
}

// Phase 1 receives the new credential, so the password and the revocation of
// the old authority commit together or not at all.
func TestReconcileHandsTheNewCredentialToPhaseOne(t *testing.T) {
	f := newReconcileFixture(true)
	res, err := f.svc.Reconcile(context.Background(), reconcileInput("the restored passphrase 1", "a brand new passphrase 22"))
	if err != nil || res.RecoveryCode == "" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if len(f.recovery.calls) != 1 {
		t.Fatalf("%d reconciliations", len(f.recovery.calls))
	}
	req := f.recovery.calls[0]
	if req.NewPasswordHash != "phc:a brand new passphrase 22" || req.NewCredentialID == "" || req.NewRecoveryCodeHash == "" {
		t.Fatalf("request = %+v", req)
	}
	if req.NewCredentialID == req.NewRecoveryCodeID {
		t.Fatal("the credential and the recovery code share an id")
	}
}

func TestReconcileRefusesAnInvalidOrUnchangedNewPasswordBeforeAnyWork(t *testing.T) {
	for name, fresh := range map[string]string{
		"empty":     "",
		"too short": "short",
		"unchanged": "the restored passphrase 1",
		"username":  "researcher",
	} {
		f := newReconcileFixture(true)
		_, err := f.svc.Reconcile(context.Background(), reconcileInput("the restored passphrase 1", fresh))
		var ve *inbound.ValidationError
		if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Field != "newPassword" {
			t.Errorf("%s: err = %v", name, err)
		}
		if f.attempts.reserved != 0 || len(f.hasher.hashes) != 0 || len(f.recovery.calls) != 0 {
			t.Errorf("%s: reserved %d, hashed %d, reconciled %d", name, f.attempts.reserved, len(f.hasher.hashes), len(f.recovery.calls))
		}
	}
}
