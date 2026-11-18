package evidence_test

import (
	"errors"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
)

const (
	checkID = "0198a2b4-5555-7222-8333-444455556666"
	userID  = "0198a2b4-6666-7222-8333-444455556666"
)

func item(t *testing.T) evidence.Item {
	t.Helper()
	it, err := evidence.New(valid())
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestACheckRecordKeepsWhatWasObservedAndWhoLooked(t *testing.T) {
	it := item(t)
	who := evidence.CheckActor{Kind: evidence.ActorUser, UserID: userID}
	good := evidence.Observation{Found: true, Safe: true, Size: 1, Digest: evidence.DigestOf([]byte("a"))}

	c, err := evidence.NewCheck(checkID, it, good, evidence.TriggerManual, who)
	if err != nil {
		t.Fatal(err)
	}
	if c.Result != evidence.ResultMatch || c.ObservedSHA256 == nil || *c.ObservedSHA256 != it.SHA256() || c.EvidenceID != itemID || c.Trigger != evidence.TriggerManual {
		t.Fatalf("check = %+v", c)
	}

	bad := evidence.Observation{Found: true, Safe: true, Size: 1, Digest: evidence.DigestOf([]byte("b"))}
	c, err = evidence.NewCheck(checkID, it, bad, evidence.TriggerRead, who)
	if err != nil || c.Result != evidence.ResultHashMismatch || c.ObservedSHA256 == nil || *c.ObservedSHA256 == it.SHA256() {
		t.Fatalf("check = %+v, %v", c, err)
	}
}

// The database requires an observed hash exactly for a match and a hash mismatch (evidence_integrity_checks_ck_c4).
func TestACheckRecordHasAHashOnlyWhenTheHashWasCompared(t *testing.T) {
	it := item(t)
	who := evidence.CheckActor{Kind: evidence.ActorSystem}
	for name, o := range map[string]evidence.Observation{
		"missing":  {},
		"unsafe":   {Found: true, Safe: false, Size: 1, Digest: evidence.DigestOf([]byte("a"))},
		"too long": {Found: true, Safe: true, Size: 2, Digest: evidence.DigestOf([]byte("a"))},
	} {
		c, err := evidence.NewCheck(checkID, it, o, evidence.TriggerSweep, who)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c.ObservedSHA256 != nil {
			t.Errorf("%s: a hash was recorded for result %s", name, c.Result)
		}
	}
	c, _ := evidence.NewCheck(checkID, it, evidence.Observation{Found: true, Safe: true, Size: 2}, evidence.TriggerSweep, who)
	if c.ObservedSize == nil || *c.ObservedSize != 2 {
		t.Errorf("the observed size of a size mismatch is not kept: %+v", c)
	}
	c, _ = evidence.NewCheck(checkID, it, evidence.Observation{}, evidence.TriggerSweep, who)
	if c.ObservedSize != nil {
		t.Errorf("a size was recorded for a missing file: %+v", c)
	}
}

func TestACheckRecordRefusesAnInvalidIdentityOrTrigger(t *testing.T) {
	it := item(t)
	o := evidence.Observation{Found: true, Safe: true, Size: 1, Digest: it.SHA256()}
	for name, c := range map[string]struct {
		id      string
		trigger evidence.CheckTrigger
		who     evidence.CheckActor
	}{
		"id not a uuid":         {"x", evidence.TriggerRead, evidence.CheckActor{Kind: evidence.ActorSystem}},
		"unknown trigger":       {checkID, "cron", evidence.CheckActor{Kind: evidence.ActorSystem}},
		"user without an id":    {checkID, evidence.TriggerRead, evidence.CheckActor{Kind: evidence.ActorUser}},
		"user id not a uuid":    {checkID, evidence.TriggerRead, evidence.CheckActor{Kind: evidence.ActorUser, UserID: "bob"}},
		"system with a user id": {checkID, evidence.TriggerRead, evidence.CheckActor{Kind: evidence.ActorSystem, UserID: userID}},
		"unknown actor kind":    {checkID, evidence.TriggerRead, evidence.CheckActor{Kind: "ai"}},
	} {
		if _, err := evidence.NewCheck(c.id, it, o, c.trigger, c.who); !errors.Is(err, evidence.ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
