package approval

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

// The errors of the lifecycle. Their texts are static and never echo input.
var (
	// ErrInvalid is returned for input that is malformed or missing.
	ErrInvalid = errors.New("invalid approval input")
	// ErrActorNotHuman is returned when anything but the human researcher tries
	// to request, approve, reject or revoke (SEC 4, P-02). An AI, a worker and a
	// zero actor are all refused.
	ErrActorNotHuman = errors.New("only a human can decide an approval")
	// ErrNotApprovable is returned for a passive operation. It needs no approval.
	ErrNotApprovable = errors.New("operation needs no approval")
	// ErrLimitsNotEstablished is returned for an active operation whose numeric
	// limits are not established (D-71).
	ErrLimitsNotEstablished = errors.New("operation limits are not established")
	// ErrReauthRequired is returned for an intrusive approval without a
	// re-authentication grant (D-58, D-69).
	ErrReauthRequired = errors.New("intrusive approval needs a re-authentication grant")
	// ErrBadTransition is returned when the approval is not in a state that
	// allows the change.
	ErrBadTransition = errors.New("approval state does not allow this change")
	// ErrBadTerms is returned for terms that break an approval invariant.
	ErrBadTerms = errors.New("invalid approval terms")
)

// Status is the stored state of an approval. Expiry is not a status: it is
// derived from the expiry time (D-48). The zero value is StatusInvalid.
type Status int

const (
	StatusInvalid Status = iota
	StatusPending
	StatusApproved
	StatusRejected
	StatusRevoked
)

// String returns the stable name that is stored and audited.
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusApproved:
		return "approved"
	case StatusRejected:
		return "rejected"
	case StatusRevoked:
		return "revoked"
	}
	return "invalid"
}

// Binding ties an approval to the state it was made in. The decision function
// compares it with the state in force (a changed scope version, policy snapshot
// or validation record makes the approval unusable, D-90, FR-SCOPE-006).
type Binding struct {
	ValidationID   string
	ScopeVersionID string
	PolicySnapshot [32]byte
}

func (b Binding) valid() bool {
	return validID(b.ValidationID) && validID(b.ScopeVersionID) && b.PolicySnapshot != [32]byte{}
}

const (
	maxIDLen        = 64
	maxReasonLength = 512
	maxAttempts     = 3
)

// Approval is an approval bound to one exact operation (D-06, D-51). It has
// value semantics: every transition returns a new Approval and the receiver is
// never changed, so the pure checks cannot disagree with the state change. The
// zero Approval authorizes nothing.
type Approval struct {
	id      string
	op      operation.Operation
	status  Status
	binding Binding

	requestedBy actor.Actor
	decidedBy   actor.Actor
	decidedAt   time.Time
	createdAt   time.Time

	expiresAt     time.Time
	maxUses       int
	maxAttempts   int
	reauthGrantID string
	uses          []Use

	revokedAt    time.Time
	revokedBy    actor.Actor
	revokeReason string
	version      int
}

// Request creates a pending approval for an exact operation. Only a human
// requests (DQ-12). A passive operation needs no approval, and an active
// operation without established limits cannot be approved (D-71).
func Request(id string, op operation.Operation, by actor.Actor, b Binding, now time.Time) (Approval, error) {
	if !by.IsHuman() {
		return Approval{}, ErrActorNotHuman
	}
	if op.IsZero() || !validID(id) || !b.valid() || now.IsZero() {
		return Approval{}, ErrInvalid
	}
	if !op.Class().IsActive() {
		return Approval{}, ErrNotApprovable
	}
	if !op.Limits().Established() {
		return Approval{}, ErrLimitsNotEstablished
	}
	return Approval{id: id, op: op, status: StatusPending, binding: b, requestedBy: by, createdAt: now.Round(0), version: 1}, nil
}

// Approve records the human decision with its terms. Terms must keep the
// invariants of the schema: an expiry in the future, at least one use, one to
// three attempts, and for an intrusive operation exactly one use, one attempt
// and a re-authentication grant. Lifetime caps are policy values owned by the
// policy store and are not judged here.
func (a Approval) Approve(by actor.Actor, t Terms, now time.Time) (Approval, error) {
	if !by.IsHuman() {
		return Approval{}, ErrActorNotHuman
	}
	if a.status != StatusPending {
		return Approval{}, ErrBadTransition
	}
	if now.IsZero() {
		return Approval{}, ErrInvalid
	}
	if err := t.validate(a.op.Class(), now); err != nil {
		return Approval{}, err
	}
	a.status = StatusApproved
	a.decidedBy, a.decidedAt = by, now.Round(0)
	a.expiresAt, a.maxUses, a.maxAttempts, a.reauthGrantID = t.ExpiresAt.Round(0), t.MaxUses, t.MaxAttempts, t.ReauthGrantID
	a.version++
	return a, nil
}

// Reject records a human refusal of a pending approval.
func (a Approval) Reject(by actor.Actor, now time.Time) (Approval, error) {
	if !by.IsHuman() {
		return Approval{}, ErrActorNotHuman
	}
	if a.status != StatusPending {
		return Approval{}, ErrBadTransition
	}
	if now.IsZero() {
		return Approval{}, ErrInvalid
	}
	a.status = StatusRejected
	a.decidedBy, a.decidedAt = by, now.Round(0)
	a.version++
	return a, nil
}

// Revoke withdraws an approved approval. A revoked approval stays revoked. Uses
// that were consumed stay consumed.
func (a Approval) Revoke(by actor.Actor, reason string, now time.Time) (Approval, error) {
	if !by.IsHuman() {
		return Approval{}, ErrActorNotHuman
	}
	if a.status != StatusApproved {
		return Approval{}, ErrBadTransition
	}
	if now.IsZero() || !validReason(reason) {
		return Approval{}, ErrInvalid
	}
	a.status = StatusRevoked
	a.revokedAt, a.revokedBy, a.revokeReason = now.Round(0), by, reason
	a.version++
	return a, nil
}

// Expired reports whether the approval has an expiry that is not after now. The
// instant of expiry is already expired. Expiry prevents new launches only
// (D-48).
//
// Times are kept as wall-clock instants. A monotonic clock reading, which a
// value from time.Now carries and which stands still while the machine is
// suspended, is dropped when a time is stored and when one is compared.
func (a Approval) Expired(now time.Time) bool {
	return !a.expiresAt.IsZero() && !now.Round(0).Before(a.expiresAt)
}

func (a Approval) ID() string                     { return a.id }
func (a Approval) Operation() operation.Operation { return a.op }
func (a Approval) Status() Status                 { return a.status }
func (a Approval) Class() operation.RiskClass     { return a.op.Class() }
func (a Approval) Binding() Binding               { return a.binding }
func (a Approval) RequestedBy() actor.Actor       { return a.requestedBy }
func (a Approval) DecidedBy() actor.Actor         { return a.decidedBy }
func (a Approval) DecidedAt() time.Time           { return a.decidedAt }
func (a Approval) CreatedAt() time.Time           { return a.createdAt }
func (a Approval) ExpiresAt() time.Time           { return a.expiresAt }
func (a Approval) MaxUses() int                   { return a.maxUses }
func (a Approval) MaxAttempts() int               { return a.maxAttempts }
func (a Approval) ReauthGrantID() string          { return a.reauthGrantID }
func (a Approval) RevokedAt() time.Time           { return a.revokedAt }
func (a Approval) RevokedBy() actor.Actor         { return a.revokedBy }
func (a Approval) RevokeReason() string           { return a.revokeReason }

// Version is a counter that rises with every change of status or of the number
// of consumed uses. It is the optimistic-lock value of the stored row.
func (a Approval) Version() int { return a.version }

func validID(s string) bool {
	if len(s) == 0 || len(s) > maxIDLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if i == 0 && !alnum {
			return false
		}
		if !alnum && c != '.' && c != '_' && c != ':' && c != '-' {
			return false
		}
	}
	return true
}

func validReason(s string) bool {
	if len(s) == 0 || len(s) > maxReasonLength || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
