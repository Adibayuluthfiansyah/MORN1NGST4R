// Package audit is the audit event model of SEC 13 (FR-AUDIT-001, D-12, D-23).
// An Event is built once, validated and immutable. The package reads no clock
// and touches no storage: the time of an event is the database clock
// (occurred_at, DB-08), and writing is the job of the AuditSink port.
package audit

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/actor"
)

// ErrInvalid is the category of every refusal of an event. The text names the
// field and never the value, so a refused event cannot leak what it carried.
var ErrInvalid = errors.New("invalid audit event")

type invalidError struct{ field string }

func (e *invalidError) Error() string        { return "invalid audit event: " + e.field }
func (e *invalidError) Is(target error) bool { return target == ErrInvalid }

func invalid(field string) error { return &invalidError{field} }

var (
	uuidPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	refPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
	reasonPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)
	typePattern   = regexp.MustCompile(`^[a-z][a-z_]{0,47}$`)
)

func isUUID(s string) bool { return uuidPattern.MatchString(s) }

// Action is a dotted action name of the catalog (domain-model.md section 5).
type Action string

// Actions returns the catalog in alphabetical order. The slice is a copy.
func Actions() []Action {
	out := make([]Action, 0, len(catalog))
	for a := range catalog {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ParseAction accepts an action of the catalog and nothing else.
func ParseAction(s string) (Action, error) {
	a := Action(s)
	if _, ok := catalog[a]; !ok {
		return "", invalid("action")
	}
	return a, nil
}

type Outcome string

const (
	Success Outcome = "success"
	Denied  Outcome = "denied"
	Failure Outcome = "failure"
)

type ActorKind string

const (
	ActorUser      ActorKind = "user"
	ActorWorker    ActorKind = "worker"
	ActorSystem    ActorKind = "system"
	ActorAI        ActorKind = "ai"
	ActorAnonymous ActorKind = "anonymous"
)

// Actor says who acted. Only a user has a user id. A request that carried no
// valid session is Anonymous and never System, so a query for system actors
// does not return the noise of unauthenticated callers.
type Actor struct {
	kind   ActorKind
	userID string
	ref    string
}

func (a Actor) Kind() ActorKind { return a.kind }
func (a Actor) UserID() string  { return a.userID }
func (a Actor) Ref() string     { return a.ref }

func User(userID string) (Actor, error) {
	if !isUUID(userID) {
		return Actor{}, invalid("actor")
	}
	return Actor{kind: ActorUser, userID: userID}, nil
}

func Anonymous() Actor { return Actor{kind: ActorAnonymous} }

// System is the application itself with no further reference.
func System() Actor { return Actor{kind: ActorSystem} }

func Worker(ref string) (Actor, error) { return withRef(ActorWorker, ref) }
func AI(ref string) (Actor, error)     { return withRef(ActorAI, ref) }

func withRef(kind ActorKind, ref string) (Actor, error) {
	if !refPattern.MatchString(ref) {
		return Actor{}, invalid("actor")
	}
	return Actor{kind: kind, ref: ref}, nil
}

// FromActor maps the actor of the policy domain. A human must carry the uuid of
// the account.
func FromActor(a actor.Actor) (Actor, error) {
	switch {
	case a.IsZero():
		return Actor{}, invalid("actor")
	case a.IsHuman():
		return User(a.ID())
	case a.Kind() == actor.KindSystem:
		return withRef(ActorSystem, a.ID())
	case a.Kind() == actor.KindAI:
		return AI(a.ID())
	}
	return Actor{}, invalid("actor")
}

// Subject names what the event is about. Every field is optional. The ids are
// lower-case uuids, and the columns have foreign keys except EntityID (DB-16).
type Subject struct {
	ProgramID, AssessmentID, JobID string
	EntityType, EntityID           string
	ScopeVersionID, DecisionID     string
	ApprovalID                     string
}

func (s Subject) validate() error {
	for _, id := range []string{s.ProgramID, s.AssessmentID, s.JobID, s.EntityID, s.ScopeVersionID, s.DecisionID, s.ApprovalID} {
		if id != "" && !isUUID(id) {
			return invalid("subject")
		}
	}
	if s.EntityType != "" && !typePattern.MatchString(s.EntityType) {
		return invalid("subject")
	}
	return nil
}

// Input is what a caller supplies. Reason is a stable code and is stored in
// the detail under the key "reason". RequestID is the server generated
// correlation id of the request, never a caller supplied one.
type Input struct {
	Actor     Actor
	Action    string
	Outcome   Outcome
	Reason    string
	RequestID string
	Subject   Subject
	Detail    map[string]any
}

// Event is a validated audit record. It has no setters, and its accessors
// return copies.
type Event struct {
	id        string
	actor     Actor
	action    Action
	outcome   Outcome
	reason    string
	requestID string
	subject   Subject
	detail    []byte
}

// New validates in and builds the event with the given id (a lower-case uuid,
// UUIDv7 in practice, DB-04).
func New(id string, in Input) (Event, error) {
	if !isUUID(id) {
		return Event{}, invalid("id")
	}
	if in.Actor.kind == "" {
		return Event{}, invalid("actor")
	}
	action, err := ParseAction(in.Action)
	if err != nil {
		return Event{}, err
	}
	switch in.Outcome {
	case Success, Denied, Failure:
	default:
		return Event{}, invalid("outcome")
	}
	if in.RequestID != "" && !isUUID(in.RequestID) {
		return Event{}, invalid("requestId")
	}
	if in.Reason != "" && !reasonPattern.MatchString(in.Reason) {
		return Event{}, invalid("reason")
	}
	if err := in.Subject.validate(); err != nil {
		return Event{}, err
	}
	d := make(map[string]any, len(in.Detail)+1)
	for k, v := range in.Detail {
		d[k] = v
	}
	if in.Reason != "" {
		if _, dup := d["reason"]; dup {
			return Event{}, invalid("reason")
		}
		d["reason"] = in.Reason
	} else if v, ok := d["reason"]; ok {
		// A reason in the detail follows the same rule as the field.
		if s, isString := v.(string); !isString || !reasonPattern.MatchString(s) {
			return Event{}, invalid("reason")
		}
	}
	clean, err := sanitizeObject(d, 0)
	if err != nil {
		return Event{}, err
	}
	raw, err := json.Marshal(clean)
	if err != nil || len(raw) > MaxDetailBytes {
		return Event{}, invalid("detail")
	}
	return Event{
		id: id, actor: in.Actor, action: action, outcome: in.Outcome, reason: in.Reason,
		requestID: in.RequestID, subject: in.Subject, detail: raw,
	}, nil
}

func (e Event) ID() string        { return e.id }
func (e Event) Actor() Actor      { return e.actor }
func (e Event) Action() Action    { return e.action }
func (e Event) Outcome() Outcome  { return e.outcome }
func (e Event) Reason() string    { return e.reason }
func (e Event) RequestID() string { return e.requestID }
func (e Event) Subject() Subject  { return e.subject }

// DetailJSON returns the redacted detail as JSON. The slice is a copy.
func (e Event) DetailJSON() []byte { return append([]byte(nil), e.detail...) }
