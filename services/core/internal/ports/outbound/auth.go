package outbound

import (
	"context"
	"time"
)

// PasswordHasher hashes and checks passwords and recovery codes with Argon2id.
// Concurrency is bounded inside the adapter. A hash that cannot be parsed
// verifies as false and costs the same as a real check.
type PasswordHasher interface {
	Hash(ctx context.Context, secret string) (phc string, err error)
	Verify(ctx context.Context, secret, phc string) (bool, error)
	// SpendVerifyCost does the work of one check against a fixed dummy hash, so a
	// login for an unknown user takes as long as one for a known user.
	SpendVerifyCost(ctx context.Context, secret string) error
}

// BootstrapSecretFile is the memory-backed file that holds the first-run secret
// (D-68). Write fails when the location is not tmpfs or ramfs.
type BootstrapSecretFile interface {
	Write(secret string) error
	Remove() error
}

type User struct {
	ID          string
	Username    string
	DisplayName string
	Disabled    bool
}

type AccountStore interface {
	CountUsers(ctx context.Context) (int, error)
	SingleUser(ctx context.Context) (User, bool, error)
	UserByUsername(ctx context.Context, username string) (User, bool, error)
	UserByID(ctx context.Context, id string) (User, bool, error)
	InsertUser(ctx context.Context, id, username string) error
	CurrentPasswordHash(ctx context.Context, userID string) (string, bool, error)
	// SetPassword supersedes the current credential, if any, and inserts the new one.
	SetPassword(ctx context.Context, userID, credentialID, phc string) error
}

type BootstrapSecretRow struct {
	ID   string
	Hash string
}

type BootstrapStore interface {
	// LockLive returns the live secret and holds a row lock until the transaction ends.
	LockLive(ctx context.Context) (BootstrapSecretRow, bool, error)
	// Supersede marks every live secret as superseded and returns how many.
	Supersede(ctx context.Context) (int, error)
	Insert(ctx context.Context, id, hash string) error
	Consume(ctx context.Context, id, userID string) error
}

type RecoveryCodeRow struct {
	ID   string
	Hash string
}

type RecoveryCodeStore interface {
	// LockLive returns the live code of the user and holds a row lock.
	LockLive(ctx context.Context, userID string) (RecoveryCodeRow, bool, error)
	Insert(ctx context.Context, id, userID, hash string) error
	MarkUsed(ctx context.Context, id string) error
	RevokeLive(ctx context.Context, userID string) error
}

type NewSession struct {
	ID          string
	UserID      string
	TokenHash   []byte
	CSRFHash    []byte
	IdleTTL     time.Duration
	AbsoluteTTL time.Duration
	UserAgent   string
}

type SessionRecord struct {
	ID                string
	UserID            string
	CSRFHash          []byte
	CreatedAt         time.Time
	LastSeenAt        time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	Revoked           bool
	UserAgent         string
	UserDisabled      bool
	// Now is the database clock at the time of the read (DB-08).
	Now time.Time
}

type SessionStore interface {
	Insert(ctx context.Context, s NewSession) error
	Lookup(ctx context.Context, tokenHash []byte) (SessionRecord, bool, error)
	// Touch extends the idle expiry and records the time, at most once per
	// minute. It never revives a revoked or expired session.
	Touch(ctx context.Context, id string, idleTTL, every time.Duration) error
	// Revoke revokes one live session of the user. It reports false when the
	// session does not exist, belongs to another user or is already revoked.
	Revoke(ctx context.Context, id, userID, reason string) (bool, error)
	// Find returns a session of the user whether or not it is still live.
	Find(ctx context.Context, id, userID string) (SessionRecord, bool, error)
	// RevokeAllExcept revokes every live session of the user but exceptID. An
	// empty exceptID revokes them all.
	RevokeAllExcept(ctx context.Context, userID, exceptID, reason string) (int, error)
	ListLive(ctx context.Context, userID string, limit int) ([]SessionRecord, error)
}

type NewGrant struct {
	ID          string
	UserID      string
	SessionID   string
	Purpose     string
	SubjectHash []byte
	TTL         time.Duration
}

type GrantRow struct {
	ID        string
	Purpose   string
	ExpiresAt time.Time
}

type ReauthStore interface {
	Insert(ctx context.Context, g NewGrant) (GrantRow, error)
	// Consume is the conditional update of CC-14: the grant must be unconsumed,
	// unexpired, of this session, this purpose and this subject. It reports
	// whether exactly one grant was consumed.
	Consume(ctx context.Context, id, sessionID, purpose string, subjectHash []byte) (bool, error)
}

type AttemptKind string

const (
	AttemptLogin          AttemptKind = "login"
	AttemptReauthenticate AttemptKind = "reauthenticate"
	AttemptRecovery       AttemptKind = "recovery"
	AttemptBootstrap      AttemptKind = "bootstrap"
)

// Budget returns the kinds whose failures count against k. Guesses at the
// password (login and re-authentication) share one budget, so the second route
// is not a second chance. A recovery-code guess and the bootstrap secret each
// have their own, so locking the password does not lock the route that gets
// the owner back in.
func (k AttemptKind) Budget() []AttemptKind {
	switch k {
	case AttemptLogin, AttemptReauthenticate:
		return []AttemptKind{AttemptLogin, AttemptReauthenticate}
	}
	return []AttemptKind{k}
}

// AttemptStore counts failed attempts in a rolling window. Reserve runs in its
// own transaction under a lock, counts the failures of the budget of kind for
// the client key and for the account, and either refuses or records the
// attempt as failed before any hashing starts. Settle records a success and
// cancels that reserved failure.
type AttemptStore interface {
	Reserve(ctx context.Context, kind AttemptKind, sourceKey, userID string, window time.Duration, max int) (allowed bool, err error)
	Settle(ctx context.Context, kind AttemptKind, sourceKey, userID string) error
}
