// Package recovery implements the recovery generation of
// docs/design/database/recovery-generation.md (DB-28): the startup decision
// that compares the external marker with the database, the first-run
// initialization, the reconciliation after a restore, and the watermark guard
// that the worker calls before a launch.
//
// The decision is a pure function (Decide). The service orchestrates the
// ports and keeps no business rule of its own.
package recovery

import "github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"

// Reason is the stable code of a recovery decision. The codes are stored in
// recovery_state.last_reason and shown on the recovery page.
type Reason string

const (
	ReasonMarkerMissing        Reason = "marker_missing"
	ReasonMarkerInvalid        Reason = "marker_invalid"
	ReasonDatabaseOlder        Reason = "database_older"
	ReasonInstanceMismatch     Reason = "instance_mismatch"
	ReasonMarkerOlder          Reason = "marker_older"
	ReasonReconcileInterrupted Reason = "reconcile_interrupted"
	ReasonFirstRun             Reason = "first_run"
)

// Action is what the startup does next.
type Action int

const (
	// ActionNormal: the marker and the database agree.
	ActionNormal Action = iota + 1
	// ActionFirstRun: initialize both halves (section 8).
	ActionFirstRun
	// ActionResumePhase2: a reconciliation crashed after phase 1. Write the
	// marker and complete phase 3 (row 7).
	ActionResumePhase2
	// ActionResumePhase3: a reconciliation crashed after the marker write.
	// Complete phase 3 (row 9).
	ActionResumePhase3
	// ActionRequired: the installation needs the reconciliation procedure. No
	// claim and no authenticated route works until it ran.
	ActionRequired
)

// Input is everything the decision looks at.
type Input struct {
	// Marker is nil when the marker is missing or invalid.
	Marker *outbound.RecoveryMarker
	// MarkerInvalid is true when a marker file exists and could not be accepted.
	MarkerInvalid bool
	Facts         outbound.RecoveryFacts
}

// Decision is the result of Decide.
type Decision struct {
	Action Action
	Reason Reason
}

// Decide is the table of section 5, row by row. M is the marker, D the
// recovery_state row. Only a clean first run and a clean row 11 continue as
// normal. Every other combination fails closed.
func Decide(in Input) Decision {
	m, d, f := in.Marker, in.Facts.Row, in.Facts
	required := func(r Reason) Decision { return Decision{Action: ActionRequired, Reason: r} }

	switch {
	case in.MarkerInvalid:
		return required(ReasonMarkerInvalid) // row 3
	case m == nil && d == nil && !f.HasUsers:
		return Decision{Action: ActionFirstRun} // row 1
	case m == nil:
		return required(ReasonMarkerMissing) // row 2
	case d == nil:
		return required(ReasonDatabaseOlder) // row 4
	case m.InstanceID != d.InstanceID:
		return required(ReasonInstanceMismatch) // row 5
	case m.Generation > d.Generation:
		return required(ReasonDatabaseOlder) // row 6
	case m.Generation < d.Generation:
		if d.Reconciling && d.Generation == m.Generation+1 {
			return Decision{Action: ActionResumePhase2, Reason: ReasonReconcileInterrupted} // row 7
		}
		return required(ReasonMarkerOlder) // row 8
	case d.Reconciling:
		return Decision{Action: ActionResumePhase3, Reason: ReasonReconcileInterrupted} // row 9
	case f.MaxAuditSeq < m.AuditWatermark:
		return required(ReasonDatabaseOlder) // row 10
	}
	return Decision{Action: ActionNormal} // row 11
}
