package evidence

import "time"

// Retention is the retention state of the content of an item.
type Retention string

const (
	RetentionActive       Retention = "active"
	RetentionPurgePending Retention = "purge_pending"
	RetentionPurged       Retention = "purged"
)

// Status is the mutable part of an item: what the last checks found and what
// became of its content. LastCheckedAt is nil until the first check.
type Status struct {
	Integrity     IntegrityState
	Retention     Retention
	LastCheckedAt *time.Time
	CompromisedAt *time.Time
	PurgedAt      *time.Time
}

// Record is an item with its status, as the store returns it. ProgramArchived
// says that the program of the item is archived and read-only.
type Record struct {
	Item            Item
	Status          Status
	ProgramArchived bool
}

type ActorKind string

const (
	ActorUser   ActorKind = "user"
	ActorWorker ActorKind = "worker"
	ActorSystem ActorKind = "system"
)

// CheckActor says who ran a check. A user carries an id, the others never do.
type CheckActor struct {
	Kind   ActorKind
	UserID string
}

// CheckRecord is one row of the check history. It is append-only. CheckedAt is
// set by the store and is zero until the row has been stored.
type CheckRecord struct {
	ID, EvidenceID string
	CheckedAt      time.Time
	Trigger        CheckTrigger
	Result         CheckResult
	ObservedSHA256 *Digest
	ObservedSize   *int64
	Actor          CheckActor
	RequestID      string
}

func validTrigger(t CheckTrigger) bool {
	switch t {
	case TriggerRead, TriggerManual, TriggerSweep, TriggerPreVerification:
		return true
	}
	return false
}

// NewCheck compares the observation with the item and builds the history row.
// The hash is kept only for a match and a hash mismatch, the two results that
// compared it (the database enforces the same rule).
func NewCheck(id string, it Item, o Observation, trigger CheckTrigger, who CheckActor) (CheckRecord, error) {
	if !isUUID(id) {
		return CheckRecord{}, invalid("id")
	}
	if !validTrigger(trigger) {
		return CheckRecord{}, invalid("trigger")
	}
	switch who.Kind {
	case ActorUser:
		if !isUUID(who.UserID) {
			return CheckRecord{}, invalid("actor")
		}
	case ActorWorker, ActorSystem:
		if who.UserID != "" {
			return CheckRecord{}, invalid("actor")
		}
	default:
		return CheckRecord{}, invalid("actor")
	}
	c := CheckRecord{ID: id, EvidenceID: it.id, Trigger: trigger, Result: Check(it, o), Actor: who}
	if c.Result == ResultMatch || c.Result == ResultHashMismatch {
		d := o.Digest
		c.ObservedSHA256 = &d
	}
	if o.Found {
		n := o.Size
		c.ObservedSize = &n
	}
	return c, nil
}
