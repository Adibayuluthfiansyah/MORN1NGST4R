// Package auth is the authentication use case: the single local account,
// sessions, re-authentication, bootstrap and recovery (D-60, D-68, D-69).
//
// It is the only production code that builds a human actor.Actor (TD-M2-01). An
// architecture test enforces that. A Principal exists only for a request that
// presented a live session, so the system and AI actors cannot be mistaken for a
// human.
package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// auditTimeout bounds the write of a denial that outlives its request.
const auditTimeout = 5 * time.Second

type Deps struct {
	Hasher        outbound.PasswordHasher
	Accounts      outbound.AccountStore
	Bootstraps    outbound.BootstrapStore
	RecoveryCodes outbound.RecoveryCodeStore
	Sessions      outbound.SessionStore
	Reauth        outbound.ReauthStore
	Attempts      outbound.AttemptStore
	Audit         outbound.AuditLog
	SecretFile    outbound.BootstrapSecretFile
	Tx            outbound.Transactor
	Recovery      inbound.Recovery

	NewID  func() string
	Now    func() time.Time
	Random func(n int) []byte
	Logger *slog.Logger
}

type Service struct {
	Deps
	anonKey []byte
}

var (
	_ inbound.Auth   = (*Service)(nil)
	_ inbound.Reauth = (*Service)(nil)
)

// New returns the use case. The key of the pre-session token is random for this
// process: a restart invalidates every ms_anon cookie, and the client asks
// GET /session again.
func New(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Random == nil {
		d.Random = func(n int) []byte {
			b := make([]byte, n)
			if _, err := rand.Read(b); err != nil {
				panic("auth: no random source: " + err.Error())
			}
			return b
		}
	}
	return &Service{Deps: d, anonKey: d.Random(32)}
}

func (s *Service) Gate() inbound.Gate {
	st := s.Recovery.State()
	switch {
	case st.Required:
		return inbound.GateRecoveryRequired
	case !st.Normal:
		return inbound.GateNotReady
	}
	return inbound.GateOpen
}

func (s *Service) mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, outbound.ErrStoreUnavailable) {
		return inbound.ErrDatabaseUnavailable
	}
	return err
}

// auditDenied records a refusal in its own transaction, so that a rollback of
// the request cannot erase it (P-33a). A failure to write it is logged and does
// not turn the refusal into a success.
func (s *Service) auditDenied(ctx context.Context, action, requestID string, detail map[string]any) {
	// A client that disconnects while a hash runs cancels the request context.
	// The denial must still be recorded, so the write is detached from the
	// request and bounded in time.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditTimeout)
	defer cancel()
	e := outbound.AuditEvent{ActorKind: "system", ActorRef: "anonymous", Action: action, Outcome: "denied", RequestID: requestID, Detail: detail}
	if err := s.Audit.Append(ctx, e); err != nil {
		s.Logger.Error("auth: a denial could not be audited", "action", action)
	}
}

func (s *Service) auditUser(ctx context.Context, action, userID, requestID string, detail map[string]any) error {
	return s.Audit.Append(ctx, outbound.AuditEvent{ActorKind: "user", ActorUserID: userID, Action: action, Outcome: "success", RequestID: requestID, Detail: detail})
}

func validationError(fields ...inbound.FieldError) error {
	return &inbound.ValidationError{Fields: fields}
}

func rateLimited() error { return &inbound.RateLimitedError{RetryAfter: attemptWindow} }
