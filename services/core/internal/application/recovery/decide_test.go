package recovery_test

import (
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/application/recovery"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

const (
	idA = "0196a5c0-1111-7222-8333-444455556666"
	idB = "0196a5c0-9999-7222-8333-444455556666"
)

func marker(id string, gen, wm uint64) *outbound.RecoveryMarker {
	return &outbound.RecoveryMarker{InstanceID: id, Generation: gen, AuditWatermark: wm}
}

func row(id string, gen uint64, reconciling bool) *outbound.RecoveryRow {
	return &outbound.RecoveryRow{InstanceID: id, Generation: gen, Reconciling: reconciling}
}

// The 11 rows of the decision table of recovery-generation.md section 5, in
// order, plus the edges between them.
func TestDecideFollowsTheTableOfSection5(t *testing.T) {
	tests := []struct {
		name string
		in   recovery.Input
		want recovery.Decision
	}{
		{"1 first run", recovery.Input{}, recovery.Decision{Action: recovery.ActionFirstRun}},
		{"2 marker missing with a row", recovery.Input{Facts: outbound.RecoveryFacts{Row: row(idA, 1, false)}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonMarkerMissing}},
		{"2 marker missing with a user", recovery.Input{Facts: outbound.RecoveryFacts{HasUsers: true}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonMarkerMissing}},
		{"3 marker invalid", recovery.Input{MarkerInvalid: true, Facts: outbound.RecoveryFacts{Row: row(idA, 1, false)}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonMarkerInvalid}},
		{"3 marker invalid on an empty database", recovery.Input{MarkerInvalid: true}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonMarkerInvalid}},
		{"4 marker without a row", recovery.Input{Marker: marker(idA, 1, 0)}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonDatabaseOlder}},
		{"5 instance mismatch", recovery.Input{Marker: marker(idA, 1, 0), Facts: outbound.RecoveryFacts{Row: row(idB, 1, false)}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonInstanceMismatch}},
		{"5 instance mismatch wins over a newer database", recovery.Input{Marker: marker(idA, 1, 0), Facts: outbound.RecoveryFacts{Row: row(idB, 2, true)}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonInstanceMismatch}},
		{"6 marker ahead of the database", recovery.Input{Marker: marker(idA, 3, 0), Facts: outbound.RecoveryFacts{Row: row(idA, 2, false)}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonDatabaseOlder}},
		{"6 marker ahead of a reconciling database", recovery.Input{Marker: marker(idA, 3, 0), Facts: outbound.RecoveryFacts{Row: row(idA, 2, true)}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonDatabaseOlder}},
		{"7 crash after phase 1", recovery.Input{Marker: marker(idA, 1, 5), Facts: outbound.RecoveryFacts{Row: row(idA, 2, true)}}, recovery.Decision{Action: recovery.ActionResumePhase2, Reason: recovery.ReasonReconcileInterrupted}},
		{"8 database two generations ahead", recovery.Input{Marker: marker(idA, 1, 5), Facts: outbound.RecoveryFacts{Row: row(idA, 3, true)}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonMarkerOlder}},
		{"8 database ahead and normal", recovery.Input{Marker: marker(idA, 1, 5), Facts: outbound.RecoveryFacts{Row: row(idA, 2, false)}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonMarkerOlder}},
		{"9 crash after the marker write", recovery.Input{Marker: marker(idA, 2, 5), Facts: outbound.RecoveryFacts{Row: row(idA, 2, true)}}, recovery.Decision{Action: recovery.ActionResumePhase3, Reason: recovery.ReasonReconcileInterrupted}},
		{"10 database behind the watermark", recovery.Input{Marker: marker(idA, 2, 50), Facts: outbound.RecoveryFacts{Row: row(idA, 2, false), MaxAuditSeq: 49}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonDatabaseOlder}},
		{"10 the empty audit table is behind a watermark", recovery.Input{Marker: marker(idA, 2, 1), Facts: outbound.RecoveryFacts{Row: row(idA, 2, false)}}, recovery.Decision{Action: recovery.ActionRequired, Reason: recovery.ReasonDatabaseOlder}},
		{"11 normal at the watermark", recovery.Input{Marker: marker(idA, 2, 50), Facts: outbound.RecoveryFacts{Row: row(idA, 2, false), MaxAuditSeq: 50}}, recovery.Decision{Action: recovery.ActionNormal}},
		{"11 normal above the watermark", recovery.Input{Marker: marker(idA, 2, 50), Facts: outbound.RecoveryFacts{Row: row(idA, 2, false), MaxAuditSeq: 80, HasUsers: true}}, recovery.Decision{Action: recovery.ActionNormal}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := recovery.Decide(tc.in); got != tc.want {
				t.Errorf("Decide = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// Nothing that is not the first run or a clean row 11 lets the system continue
// as normal. This walks every combination of the inputs and requires that a
// Normal result is only ever the clean case (fail closed).
func TestDecideNormalOnlyWhenMarkerAndDatabaseAgree(t *testing.T) {
	markers := []*outbound.RecoveryMarker{nil, marker(idA, 1, 0), marker(idA, 2, 5), marker(idA, 3, 5), marker(idB, 2, 5)}
	rows := []*outbound.RecoveryRow{nil, row(idA, 1, false), row(idA, 2, false), row(idA, 2, true), row(idA, 3, false), row(idB, 2, false)}
	for _, m := range markers {
		for _, invalid := range []bool{false, true} {
			for _, r := range rows {
				for _, users := range []bool{false, true} {
					for _, seq := range []uint64{0, 4, 5, 6} {
						in := recovery.Input{Marker: m, MarkerInvalid: invalid && m == nil, Facts: outbound.RecoveryFacts{Row: r, HasUsers: users, MaxAuditSeq: seq}}
						d := recovery.Decide(in)
						if d.Action != recovery.ActionNormal {
							continue
						}
						ok := m != nil && r != nil && m.InstanceID == r.InstanceID && m.Generation == r.Generation && !r.Reconciling && seq >= m.AuditWatermark
						if !ok {
							t.Errorf("Decide returned Normal for %+v / %+v / %+v", m, r, in.Facts)
						}
					}
				}
			}
		}
	}
}
