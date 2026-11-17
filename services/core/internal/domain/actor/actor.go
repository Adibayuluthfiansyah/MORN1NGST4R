package actor

import "errors"

// ErrInvalid is returned for a kind or an identity that New or ParseKind
// refuse. The text is static and never echoes the input.
var ErrInvalid = errors.New("invalid actor")

// Kind says what sort of caller an Actor is. The zero value is KindInvalid, so
// an Actor that was never built is not a human.
type Kind int

const (
	KindInvalid Kind = iota
	// KindHuman is the one authenticated researcher account (D-60).
	KindHuman
	// KindSystem is the application itself, for example a worker.
	KindSystem
	// KindAI is an AI provider path. It proposes and never decides (P-11).
	KindAI
)

// String returns the stable name that is stored and audited.
func (k Kind) String() string {
	switch k {
	case KindHuman:
		return "human"
	case KindSystem:
		return "system"
	case KindAI:
		return "ai"
	}
	return "invalid"
}

// ParseKind reads a stored kind name. Only the three exact lower-case names are
// valid. The MVP has no API tokens (D-60), and a worker is a system actor.
func ParseKind(s string) (Kind, error) {
	switch s {
	case "human":
		return KindHuman, nil
	case "system":
		return KindSystem, nil
	case "ai":
		return KindAI, nil
	}
	return KindInvalid, ErrInvalid
}

const maxIDLen = 64

// Actor is a kind and an identity. Its fields are unexported, so it cannot be
// changed after New and the only way to get a non-zero value is New.
type Actor struct {
	kind Kind
	id   string
}

// New builds an Actor. The identity is 1 to 64 ASCII letters, digits, '.', '_',
// ':' and '-', and starts with a letter or digit. That covers UUIDs and worker
// instance names and keeps control characters and look-alikes out of audit
// records and approval screens.
func New(kind Kind, id string) (Actor, error) {
	if kind != KindHuman && kind != KindSystem && kind != KindAI {
		return Actor{}, ErrInvalid
	}
	if !validID(id) {
		return Actor{}, ErrInvalid
	}
	return Actor{kind: kind, id: id}, nil
}

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

// Kind returns the kind. The zero Actor has KindInvalid.
func (a Actor) Kind() Kind { return a.kind }

// ID returns the identity. The zero Actor has none.
func (a Actor) ID() string { return a.id }

// IsHuman reports whether the actor is the human researcher. It is the only
// check that approval, rejection and revocation accept.
func (a Actor) IsHuman() bool { return a.kind == KindHuman }

// IsZero reports whether the Actor was never built by New.
func (a Actor) IsZero() bool { return a == Actor{} }
