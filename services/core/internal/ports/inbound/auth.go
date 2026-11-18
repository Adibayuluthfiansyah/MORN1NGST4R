package inbound

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
)

var (
	ErrUnauthenticated        = errors.New("no valid session")
	ErrSessionExpired         = errors.New("the session expired or was revoked")
	ErrInvalidCredentials     = errors.New("credentials were not accepted")
	ErrBootstrapSecretInvalid = errors.New("the bootstrap secret was not accepted")
	ErrRecoveryInvalid        = errors.New("the recovery code was not accepted")
	ErrSetupUnavailable       = errors.New("account setup is unavailable")
	ErrAccountExists          = errors.New("an account exists")
	ErrConflictState          = errors.New("the resource is not in a state that allows this")
	ErrNotFound               = errors.New("not found")
	ErrReauthRequired         = errors.New("a fresh password entry is required")
	ErrReauthInvalid          = errors.New("the re-authentication grant is not valid for this action")
	ErrReauthSubjectInvalid   = errors.New("the re-authentication subject is invalid")
	ErrRecoveryNotRequired    = errors.New("recovery is not required")
	ErrDatabaseUnavailable    = errors.New("the database is unavailable")
)

type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "rate limited" }

type FieldError struct {
	Field string
	Code  string
}

// ValidationError lists the fields that failed. It carries field names and fixed
// codes only, never the submitted value.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed (%d fields)", len(e.Fields))
}

// Principal is an authenticated request. Only the authentication use case
// creates one, and the human actor inside it is the only way a use case learns
// who acts (TD-M2-01).
type Principal interface {
	UserID() string
	SessionID() string
	Actor() actor.Actor
	// ValidCSRF reports whether presented is this session's CSRF token.
	ValidCSRF(presented string) bool
}

type Gate int

const (
	GateOpen Gate = iota
	// GateRecoveryRequired: a restore was detected and only GET /session and the reconcile route work.
	GateRecoveryRequired
	// GateNotReady: the recovery check has not completed, normally because the database is down.
	GateNotReady
)

type StateKind string

const (
	StateSetupRequired    StateKind = "setup_required"
	StateAnonymous        StateKind = "anonymous"
	StateAuthenticated    StateKind = "authenticated"
	StateRecoveryRequired StateKind = "recovery_required"
)

type UserSummary struct {
	ID          string
	Username    string
	DisplayName string
}

type RecoveryInfo struct {
	Reason             string
	MarkerGeneration   uint64
	DatabaseGeneration uint64
	DetectedAt         time.Time
}

type Cookies struct {
	Session string
	Anon    string
}

type SessionState struct {
	Kind              StateKind
	CSRFToken         string
	User              *UserSummary
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	SessionID         string
	Recovery          *RecoveryInfo
	// NewAnonToken is set when the caller has no valid pre-session token. The
	// adapter sets it in the ms_anon cookie.
	NewAnonToken string
}

// IssuedSession carries a fresh session token. The adapter sets it in the cookie
// and never puts it in a body.
type IssuedSession struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

type SetupInput struct {
	BootstrapSecret, Username, Password string
	ClientKey, UserAgent, RequestID     string
}

type SetupResult struct {
	User         UserSummary
	RecoveryCode string
	Session      IssuedSession
}

type LoginInput struct {
	Username, Password              string
	ClientKey, UserAgent, RequestID string
}

type LoginResult struct {
	User    UserSummary
	Session IssuedSession
	State   SessionState
}

type SessionView struct {
	ID                                                      string
	CreatedAt, LastSeenAt, IdleExpiresAt, AbsoluteExpiresAt time.Time
	Current                                                 bool
	UserAgent                                               string
}

type ReauthInput struct {
	Password  string
	Purpose   string
	Subject   map[string]any
	ClientKey string
	RequestID string
}

type ReauthGrant struct {
	ID        string
	Purpose   string
	ExpiresAt time.Time
}

type RecoveryResetInput struct {
	RecoveryCode, NewPassword       string
	ClientKey, UserAgent, RequestID string
}

type RecoveryResetResult struct {
	RecoveryCode string
	Session      IssuedSession
}

type ReconcileInput struct {
	// Password is the password of the restored database. NewPassword replaces it
	// in the same step.
	Password          string
	NewPassword       string
	ConfirmRevocation bool
	ClientKey         string
	RequestID         string
}

type ReconcileResult struct {
	Generation       uint64
	RevokedSessions  int
	RevokedApprovals int
	StaleValidations int
	RevokedLabs      int
	CancelledJobs    int
	RecoveryCode     string
}

type ChangePasswordInput struct {
	CurrentPassword, NewPassword    string
	ReauthGrantID                   string
	ClientKey, UserAgent, RequestID string
}

// Auth is the authentication use case that the HTTP adapter calls.
type Auth interface {
	Gate() Gate
	Authenticate(ctx context.Context, sessionToken string) (Principal, error)
	ValidAnonCSRF(anonToken, presented string) bool

	SessionState(ctx context.Context, c Cookies) (SessionState, error)
	Setup(ctx context.Context, in SetupInput) (SetupResult, error)
	Login(ctx context.Context, in LoginInput) (LoginResult, error)
	Logout(ctx context.Context, p Principal, requestID string) error
	ListSessions(ctx context.Context, p Principal, limit int) ([]SessionView, error)
	RevokeSession(ctx context.Context, p Principal, sessionID, requestID string) (SessionView, error)
	Reauthenticate(ctx context.Context, p Principal, in ReauthInput) (ReauthGrant, error)
	ResetPassword(ctx context.Context, in RecoveryResetInput) (RecoveryResetResult, error)
	ChangePassword(ctx context.Context, p Principal, in ChangePasswordInput) (IssuedSession, error)
	Reconcile(ctx context.Context, in ReconcileInput) (ReconcileResult, error)
}

// ReauthUse is what a use case that needs a fresh password entry passes in.
type ReauthUse struct {
	GrantID     string
	Purpose     identity.Purpose
	SubjectHash [32]byte
}

// Reauth is the contract for the use cases of later milestones (approval,
// scope, policy, lab, tools, providers). Consume runs the conditional update of
// CC-14 in the caller's transaction. A missing grant id is ErrReauthRequired.
// A grant that is expired, consumed, of another session, another purpose or
// another subject is ErrReauthInvalid.
type Reauth interface {
	Consume(ctx context.Context, p Principal, use ReauthUse) error
}
