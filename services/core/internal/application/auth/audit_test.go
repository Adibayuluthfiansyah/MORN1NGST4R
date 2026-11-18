package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
)

// TD-M4-03: repeated rate-limited refusals are coalesced, refusals for a wrong
// credential never are.
func TestRepeatedRateLimitedDenialsAreCoalescedPerWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	sink := &fakeAudit{}
	s := New(Deps{Audit: sink, NewID: newTestID, Now: func() time.Time { return now }})
	for i := 0; i < 5; i++ {
		s.auditDenied(context.Background(), audit.Anonymous(), "auth.login", testRequestID, "rate_limited")
		now = now.Add(time.Minute)
	}
	if len(sink.events) != 1 {
		t.Fatalf("%d records for a burst of five refusals", len(sink.events))
	}
	s.auditDenied(context.Background(), audit.Anonymous(), "auth.recovery_reset", testRequestID, "rate_limited")
	if len(sink.events) != 2 {
		t.Fatal("another action shares the budget of the first")
	}
	now = now.Add(attemptWindow)
	s.auditDenied(context.Background(), audit.Anonymous(), "auth.login", testRequestID, "rate_limited")
	if len(sink.events) != 3 {
		t.Fatalf("%d records after the window", len(sink.events))
	}
	var d map[string]any
	_ = json.Unmarshal(sink.events[2].DetailJSON(), &d)
	if d["suppressed"] != float64(4) || d["reason"] != "rate_limited" {
		t.Fatalf("detail = %v, want the 4 refusals that the window suppressed", d)
	}
}

func TestRefusalsForAWrongCredentialAreNeverCoalesced(t *testing.T) {
	sink := &fakeAudit{}
	s := New(Deps{Audit: sink, NewID: newTestID, Now: func() time.Time { return time.Unix(1_700_000_000, 0) }})
	for i := 0; i < 20; i++ {
		s.auditDenied(context.Background(), audit.Anonymous(), "auth.login", testRequestID, "invalid_credentials")
	}
	if len(sink.events) != 20 {
		t.Fatalf("%d of 20 refusals for a wrong credential were recorded", len(sink.events))
	}
	for _, e := range sink.events {
		if e.Actor().Kind() != audit.ActorAnonymous || e.Outcome() != audit.Denied {
			t.Fatalf("event = %+v", e)
		}
	}
}

// A denial is recorded and nothing that the sink does can change it. A
// sink that fails still leaves the caller with its refusal.
func TestAFailingSinkDoesNotChangeARefusal(t *testing.T) {
	sink := &failingSink{}
	s := New(Deps{Audit: sink, NewID: newTestID})
	s.auditDenied(context.Background(), audit.Anonymous(), "auth.login", testRequestID, "invalid_credentials")
	s.auditRevoked(context.Background(), "auth.logout", "0198a2b4-2222-7222-8333-444455556666", testRequestID, nil)
	if sink.records != 2 {
		t.Fatalf("%d records tried", sink.records)
	}
}

type failingSink struct{ records int }

func (f *failingSink) Require(context.Context, audit.Event) error { return errSink }
func (f *failingSink) Record(context.Context, audit.Event) error  { f.records++; return errSink }
func (f *failingSink) Gate(context.Context) error                 { return errSink }

var errSink = errors.New("the sink failed")

// A refusal whose record was not written gives its place back, so the next
// rate-limited refusal is recorded and reports the count that was lost.
func TestARateLimitedRecordThatFailedIsTriedAgainOnTheNextRefusal(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	sink := &flakySink{failFirst: true}
	s := New(Deps{Audit: sink, NewID: newTestID, Now: func() time.Time { return now }})
	s.auditDenied(context.Background(), audit.Anonymous(), "auth.login", testRequestID, "rate_limited")
	s.auditDenied(context.Background(), audit.Anonymous(), "auth.login", testRequestID, "rate_limited")
	if len(sink.recorded) != 1 {
		t.Fatalf("%d records after a failed first write, want the second refusal recorded", len(sink.recorded))
	}
}

type flakySink struct {
	failFirst bool
	tried     int
	recorded  []audit.Event
}

func (f *flakySink) Require(context.Context, audit.Event) error { return nil }
func (f *flakySink) Gate(context.Context) error                 { return nil }
func (f *flakySink) Record(_ context.Context, e audit.Event) error {
	f.tried++
	if f.failFirst && f.tried == 1 {
		return errSink
	}
	f.recorded = append(f.recorded, e)
	return nil
}
