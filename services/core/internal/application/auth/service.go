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

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

type Deps struct {
	Hasher        outbound.PasswordHasher
	Accounts      outbound.AccountStore
	Bootstraps    outbound.BootstrapStore
	RecoveryCodes outbound.RecoveryCodeStore
	Sessions      outbound.SessionStore
	Reauth        outbound.ReauthStore
	Attempts      outbound.AttemptStore
	Audit         outbound.AuditSink
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
	anonKey  []byte
	throttle *audit.Coalescer
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
	return &Service{Deps: d, anonKey: d.Random(32), throttle: audit.NewCoalescer(attemptWindow, d.Now)}
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

// auditDenied records a refusal on its own, so that the rollback of the request
// cannot erase it (P-33a). Whatever happens to the write, the refusal stands.
// Repeated rate-limited refusals of one action are coalesced (TD-M4-03): the
// first is recorded, the next ones are counted until the attempt window has
// passed, and the next record reports the count. A refusal for a wrong
// credential is never coalesced.
func (s *Service) auditDenied(ctx context.Context, who audit.Actor, action, requestID, reason string) {
	var detail map[string]any
	if reason == "rate_limited" {
		record, suppressed := s.throttle.Admit(action)
		if !record {
			return
		}
		if suppressed > 0 {
			detail = map[string]any{"suppressed": suppressed}
		}
	}
	suppressed, _ := detail["suppressed"].(int)
	e, err := audit.New(s.NewID(), audit.Input{Actor: who, Action: action, Outcome: audit.Denied, Reason: reason, RequestID: requestID, Detail: detail})
	if err == nil {
		err = s.Audit.Record(ctx, e)
	} else {
		s.Logger.Error("auth: a denial could not be built as an audit event", "action", action)
	}
	if err != nil && reason == "rate_limited" {
		s.throttle.Retract(action, suppressed)
	}
}

// userActor is the audit actor of an authenticated request.
func userActor(p inbound.Principal) audit.Actor {
	a, err := audit.FromActor(p.Actor())
	if err != nil {
		return audit.Anonymous()
	}
	return a
}

// auditUser is the record that a change needs before it may commit. Inside the
// transaction of the change it joins it, so a failed write rolls the change back.
func (s *Service) auditUser(ctx context.Context, action, userID, requestID string, detail map[string]any) error {
	who, err := audit.User(userID)
	if err != nil {
		return err
	}
	e, err := audit.New(s.NewID(), audit.Input{Actor: who, Action: action, Outcome: audit.Success, RequestID: requestID, Detail: detail})
	if err != nil {
		return err
	}
	return s.Audit.Require(ctx, e)
}

// auditRevoked records a revocation that has already taken effect. A revocation
// must work when audit cannot be written (SEC 13), so it is recorded after its
// commit and its outcome does not change the answer.
func (s *Service) auditRevoked(ctx context.Context, action, userID, requestID string, detail map[string]any) {
	who, err := audit.User(userID)
	if err != nil {
		return
	}
	e, err := audit.New(s.NewID(), audit.Input{Actor: who, Action: action, Outcome: audit.Success, RequestID: requestID, Detail: detail})
	if err != nil {
		s.Logger.Error("auth: a revocation could not be built as an audit event", "action", action)
		return
	}
	_ = s.Audit.Record(ctx, e)
}

func validationError(fields ...inbound.FieldError) error {
	return &inbound.ValidationError{Fields: fields}
}

func rateLimited() error { return &inbound.RateLimitedError{RetryAfter: attemptWindow} }
