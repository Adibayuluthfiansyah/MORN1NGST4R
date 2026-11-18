package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

type resetAccounts struct{ outbound.AccountStore }

func (resetAccounts) SingleUser(context.Context) (outbound.User, bool, error) {
	return outbound.User{ID: "u1", Username: "researcher"}, true, nil
}

type resetCodes struct {
	outbound.RecoveryCodeStore
	markErr error
}

func (resetCodes) LockLive(context.Context, string) (outbound.RecoveryCodeRow, bool, error) {
	return outbound.RecoveryCodeRow{ID: "c1", Hash: "stored"}, true, nil
}

func (r resetCodes) MarkUsed(context.Context, string) error { return r.markErr }

type resetHasher struct{ outbound.PasswordHasher }

func (resetHasher) Verify(context.Context, string, string) (bool, error) { return true, nil }
func (resetHasher) Hash(context.Context, string) (string, error)         { return "phc", nil }

type directTx struct{}

func (directTx) InTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func resetWith(markErr error) error {
	s := New(Deps{
		Accounts: resetAccounts{}, RecoveryCodes: resetCodes{markErr: markErr}, Hasher: resetHasher{},
		Attempts: &fakeAttempts{allow: true}, Audit: &fakeAudit{}, Tx: directTx{}, Random: counterRandom(), NewID: newTestID,
	})
	shown, _, _ := identity.FormatRecoveryCode(counterRandom()(identity.RecoveryCodeBytes))
	_, err := s.ResetPassword(context.Background(), inbound.RecoveryResetInput{RecoveryCode: shown, NewPassword: "a long new passphrase 42", ClientKey: "loopback"})
	return err
}

// A failure of the database while the code is consumed is an outage (503), not
// a wrong code (401). A code that is no longer live is a wrong code.
func TestResetDistinguishesAnOutageFromAnInvalidCode(t *testing.T) {
	if err := resetWith(fmt.Errorf("use the recovery code: %w", outbound.ErrStoreUnavailable)); !errors.Is(err, inbound.ErrDatabaseUnavailable) {
		t.Fatalf("outage: err = %v", err)
	}
	if err := resetWith(outbound.ErrNotLive); !errors.Is(err, inbound.ErrRecoveryInvalid) {
		t.Fatalf("spent code: err = %v", err)
	}
}
