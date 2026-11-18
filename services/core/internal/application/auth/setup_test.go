package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

type countingHasher struct {
	outbound.PasswordHasher
	hashes int
}

func (h *countingHasher) Hash(context.Context, string) (string, error) {
	h.hashes++
	return "phc", nil
}

type fakeAccounts struct {
	outbound.AccountStore
	users int
}

func (f fakeAccounts) CountUsers(context.Context) (int, error) { return f.users, nil }

type fakeAttempts struct {
	outbound.AttemptStore
	reserved int
	allow    bool
}

func (f *fakeAttempts) Reserve(context.Context, outbound.AttemptKind, string, string, time.Duration, int) (bool, error) {
	f.reserved++
	return f.allow, nil
}

func (f *fakeAttempts) Settle(context.Context, outbound.AttemptKind, string, string) error {
	return nil
}

type fakeBootstraps struct {
	outbound.BootstrapStore
	hash string
}

func (f fakeBootstraps) LockLive(context.Context) (outbound.BootstrapSecretRow, bool, error) {
	return outbound.BootstrapSecretRow{ID: "b1", Hash: f.hash}, true, nil
}

type fakeAudit struct {
	actions  []string
	events   []audit.Event
	required int
}

func (f *fakeAudit) Require(_ context.Context, e audit.Event) error {
	f.required++
	f.events = append(f.events, e)
	f.actions = append(f.actions, string(e.Action())+"/"+string(e.Outcome()))
	return nil
}

func (f *fakeAudit) Record(_ context.Context, e audit.Event) error {
	f.events = append(f.events, e)
	f.actions = append(f.actions, string(e.Action())+"/"+string(e.Outcome()))
	return nil
}

func (f *fakeAudit) Gate(context.Context) error { return nil }

var testIDs int

// newTestID returns a valid lower-case uuid that differs on every call.
func newTestID() string {
	testIDs++
	return fmt.Sprintf("0198a2b4-%04x-7222-8333-444455556666", testIDs)
}

const testRequestID = "0198a2b4-3333-4222-8333-444455556666"

const validSecret = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func setupInput(secret string) inbound.SetupInput {
	return inbound.SetupInput{BootstrapSecret: secret, Username: "researcher", Password: "correct horse battery staple", ClientKey: "loopback", RequestID: testRequestID}
}

func TestSetupChecksTheSecretBeforeAnyHashing(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	hasher := &countingHasher{}
	sink := &fakeAudit{}
	s := New(Deps{
		Hasher:     hasher,
		Accounts:   fakeAccounts{},
		Attempts:   &fakeAttempts{allow: true},
		Bootstraps: fakeBootstraps{hash: hashSecret(validSecret)},
		Audit:      sink,
		NewID:      newTestID,
		Now:        func() time.Time { return now },
	})
	wrong := "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	_, err := s.Setup(context.Background(), setupInput(wrong))
	if !errors.Is(err, inbound.ErrBootstrapSecretInvalid) {
		t.Fatalf("err = %v", err)
	}
	if hasher.hashes != 0 {
		t.Fatalf("the password was hashed %d times for a wrong secret", hasher.hashes)
	}
	if len(sink.actions) != 1 || sink.actions[0] != "auth.account_created/denied" {
		t.Fatalf("audit = %v", sink.actions)
	}
}

func TestSetupRateLimitIsCheckedBeforeTheSecretAndHashing(t *testing.T) {
	hasher := &countingHasher{}
	s := New(Deps{
		Hasher:     hasher,
		Accounts:   fakeAccounts{},
		Attempts:   &fakeAttempts{allow: false},
		Bootstraps: fakeBootstraps{hash: hashSecret(validSecret)},
		Audit:      &fakeAudit{},
		NewID:      newTestID,
	})
	_, err := s.Setup(context.Background(), setupInput(validSecret))
	var rl *inbound.RateLimitedError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %v", err)
	}
	if hasher.hashes != 0 {
		t.Fatal("hashed while rate limited")
	}
}

func TestSetupRefusesWhenAnAccountExistsBeforeCountingAnAttempt(t *testing.T) {
	attempts := &fakeAttempts{allow: true}
	s := New(Deps{Hasher: &countingHasher{}, Accounts: fakeAccounts{users: 1}, Attempts: attempts})
	if _, err := s.Setup(context.Background(), setupInput(validSecret)); !errors.Is(err, inbound.ErrAccountExists) {
		t.Fatalf("err = %v", err)
	}
	if attempts.reserved != 0 {
		t.Fatal("an attempt was reserved for a request that cannot succeed")
	}
}

func TestSetupValidationNeverHashesOrTouchesTheDatabase(t *testing.T) {
	hasher := &countingHasher{}
	s := New(Deps{Hasher: hasher})
	in := setupInput("")
	in.Password = "tiny"
	_, err := s.Setup(context.Background(), in)
	var ve *inbound.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) != 2 {
		t.Fatalf("err = %v", err)
	}
	if hasher.hashes != 0 {
		t.Fatal("hashed an invalid request")
	}
}
