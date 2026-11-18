package evidence_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
)

const (
	itemID       = "0198a2b4-1111-7222-8333-444455556666"
	programID    = "0198a2b4-2222-7222-8333-444455556666"
	assessmentID = "0198a2b4-3333-7222-8333-444455556666"
	jobID        = "0198a2b4-4444-7222-8333-444455556666"
)

var captured = time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

func valid() evidence.NewItem {
	return evidence.NewItem{
		ID: itemID, ProgramID: programID, AssessmentID: assessmentID, Kind: "response", ContentType: "text/plain",
		Origin: "observed", Redaction: evidence.Redacted, SHA256: evidence.DigestOf([]byte("a")), Size: 1, CapturedAt: captured,
	}
}

func TestNewBuildsAnItemWithItsProvenance(t *testing.T) {
	in := valid()
	in.JobID, in.ToolKey, in.ToolVersion = jobID, "httpx", "1.6.0"
	in.OperationHash = make([]byte, 32)
	in.RedactedArgs = []string{"-u", "https://example.com", "-H", "Authorization: Bearer abcdefghijklmnop"}
	it, err := evidence.New(in)
	if err != nil {
		t.Fatal(err)
	}
	if it.ID() != itemID || it.StorageKey() != itemID || it.Kind() != evidence.KindResponse || it.Sensitivity() != evidence.Sensitive || it.Size() != 1 {
		t.Fatalf("item = %+v", it)
	}
	if it.SHA256() != evidence.DigestOf([]byte("a")) || it.Redaction() != evidence.Redacted || it.JobID() != jobID || it.ToolKey() != "httpx" {
		t.Fatalf("item = %+v", it)
	}
	for _, a := range it.RedactedArgs() {
		if strings.Contains(a, "abcdefghijklmnop") {
			t.Errorf("an argument kept its secret: %q", a)
		}
	}
}

func TestNewRefusesAnInvalidItem(t *testing.T) {
	for name, mod := range map[string]func(*evidence.NewItem){
		"id not a uuid":         func(in *evidence.NewItem) { in.ID = "ev-1" },
		"program not a uuid":    func(in *evidence.NewItem) { in.ProgramID = "" },
		"assessment not a uuid": func(in *evidence.NewItem) { in.AssessmentID = "x" },
		"job not a uuid":        func(in *evidence.NewItem) { in.JobID = "x" },
		"unknown kind":          func(in *evidence.NewItem) { in.Kind = "screenshot" },
		"unknown origin":        func(in *evidence.NewItem) { in.Origin = "guessed" },
		"unknown redaction":     func(in *evidence.NewItem) { in.Redaction = "none" },
		"negative size":         func(in *evidence.NewItem) { in.Size = -1 },
		"zero time":             func(in *evidence.NewItem) { in.CapturedAt = time.Time{} },
		"empty content type":    func(in *evidence.NewItem) { in.ContentType = "" },
		"content type control":  func(in *evidence.NewItem) { in.ContentType = "text/plain\r\nX-A: b" },
		"content type too long": func(in *evidence.NewItem) { in.ContentType = strings.Repeat("a", 300) },
		"operation hash length": func(in *evidence.NewItem) { in.OperationHash = []byte{1} },
		"tool key with a space": func(in *evidence.NewItem) { in.ToolKey = "http x" },
		"too many arguments":    func(in *evidence.NewItem) { in.RedactedArgs = make([]string, 300) },
		"an argument too long":  func(in *evidence.NewItem) { in.RedactedArgs = []string{strings.Repeat("a", 5000)} },
	} {
		in := valid()
		mod(&in)
		if _, err := evidence.New(in); !errors.Is(err, evidence.ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestErrorsNameTheFieldAndNeverTheValue(t *testing.T) {
	in := valid()
	in.Kind = "secret-value-here"
	_, err := evidence.New(in)
	if err == nil || !strings.Contains(err.Error(), "kind") || strings.Contains(err.Error(), "secret-value-here") {
		t.Fatalf("err = %v", err)
	}
}

func TestAnItemCannotBeChangedAfterItIsBuilt(t *testing.T) {
	in := valid()
	in.OperationHash = make([]byte, 32)
	in.RedactedArgs = []string{"-u", "x"}
	it, err := evidence.New(in)
	if err != nil {
		t.Fatal(err)
	}
	in.OperationHash[0] = 9
	in.RedactedArgs[0] = "changed"
	it.OperationHash()[1] = 9
	it.RedactedArgs()[1] = "changed"
	if it.OperationHash()[0] != 0 || it.OperationHash()[1] != 0 || it.RedactedArgs()[0] != "-u" || it.RedactedArgs()[1] != "x" {
		t.Fatalf("the item changed: %v %v", it.OperationHash(), it.RedactedArgs())
	}
}

func TestDigest(t *testing.T) {
	d := evidence.DigestOf([]byte("abc"))
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if d.String() != want {
		t.Fatalf("digest = %s", d)
	}
	back, err := evidence.ParseDigest(want)
	if err != nil || back != d || !back.Equal(d) {
		t.Fatalf("round trip = %v, %v", back, err)
	}
	if d.Equal(evidence.DigestOf([]byte("abd"))) {
		t.Fatal("different content compared equal")
	}
	for _, bad := range []string{"", "abc", strings.Repeat("z", 64), strings.ToUpper(want), want + "0"} {
		if _, err := evidence.ParseDigest(bad); !errors.Is(err, evidence.ErrInvalid) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if b := d.Bytes(); len(b) != 32 {
		t.Fatalf("%d bytes", len(b))
	}
	if evidence.DigestFromBytes(d.Bytes()) != d {
		t.Fatal("bytes round trip")
	}
}

func TestACheckResultFollowsWhatWasObserved(t *testing.T) {
	it, _ := evidence.New(valid())
	same := evidence.Observation{Found: true, Safe: true, Size: 1, Digest: evidence.DigestOf([]byte("a"))}
	cases := map[string]struct {
		o    evidence.Observation
		want evidence.CheckResult
	}{
		"the same bytes":        {same, evidence.ResultMatch},
		"another hash":          {evidence.Observation{Found: true, Safe: true, Size: 1, Digest: evidence.DigestOf([]byte("b"))}, evidence.ResultHashMismatch},
		"another size":          {evidence.Observation{Found: true, Safe: true, Size: 2, Digest: evidence.DigestOf([]byte("a"))}, evidence.ResultSizeMismatch},
		"no file":               {evidence.Observation{}, evidence.ResultMissing},
		"a file that is unsafe": {evidence.Observation{Found: true, Safe: false, Size: 1, Digest: evidence.DigestOf([]byte("a"))}, evidence.ResultMetadataInconsistent},
	}
	for name, c := range cases {
		if got := evidence.Check(it, c.o); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

// A size that differs wins over a hash that differs, because the size is known
// before the file is read to the end.
func TestTheSizeIsComparedBeforeTheHash(t *testing.T) {
	it, _ := evidence.New(valid())
	o := evidence.Observation{Found: true, Safe: true, Size: 5, Digest: evidence.DigestOf([]byte("zzz"))}
	if got := evidence.Check(it, o); got != evidence.ResultSizeMismatch {
		t.Fatalf("got %s", got)
	}
}

func TestTheIntegrityStateOnlyGetsWorseUntilAHumanActs(t *testing.T) {
	type step struct {
		from   evidence.IntegrityState
		result evidence.CheckResult
		want   evidence.IntegrityState
	}
	for _, s := range []step{
		{evidence.Unchecked, evidence.ResultMatch, evidence.IntegrityOK},
		{evidence.IntegrityOK, evidence.ResultMatch, evidence.IntegrityOK},
		{evidence.IntegrityOK, evidence.ResultHashMismatch, evidence.Compromised},
		{evidence.IntegrityOK, evidence.ResultSizeMismatch, evidence.Compromised},
		{evidence.IntegrityOK, evidence.ResultMetadataInconsistent, evidence.Compromised},
		{evidence.IntegrityOK, evidence.ResultMissing, evidence.Missing},
		{evidence.Missing, evidence.ResultMatch, evidence.Missing},
		{evidence.Missing, evidence.ResultMissing, evidence.Missing},
		{evidence.Missing, evidence.ResultHashMismatch, evidence.Compromised},
		{evidence.Compromised, evidence.ResultMatch, evidence.Compromised},
		{evidence.Compromised, evidence.ResultMissing, evidence.Compromised},
		{evidence.Compromised, evidence.ResultHashMismatch, evidence.Compromised},
	} {
		if got := evidence.NextState(s.from, s.result); got != s.want {
			t.Errorf("%s after %s: %s, want %s", s.from, s.result, got, s.want)
		}
	}
}

func TestOnlyAnItemThatPassedItsCheckCanSupportVerification(t *testing.T) {
	for state, want := range map[evidence.IntegrityState]bool{
		evidence.Unchecked: false, evidence.IntegrityOK: true, evidence.Compromised: false, evidence.Missing: false,
	} {
		if got := state.SupportsVerification(); got != want {
			t.Errorf("%s: %v", state, got)
		}
	}
	if evidence.IntegrityState("").SupportsVerification() || evidence.IntegrityState("fine").SupportsVerification() {
		t.Fatal("an unknown state supports verification")
	}
}

func TestContentFollowsTheRedactionDecision(t *testing.T) {
	tok := strings.Repeat("Qz", 12)
	raw := []byte("Authorization: Bearer " + tok + "\n")
	out, state, err := evidence.Prepare(raw, evidence.PrepareOptions{})
	if err != nil || state != evidence.Redacted || strings.Contains(string(out), tok) {
		t.Fatalf("default = %q, %s, %v", out, state, err)
	}
	if _, _, err := evidence.Prepare(raw, evidence.PrepareOptions{Raw: true}); !errors.Is(err, evidence.ErrRawNotAllowed) {
		t.Fatalf("raw without permission: %v", err)
	}
	out, state, err = evidence.Prepare(raw, evidence.PrepareOptions{Raw: true, RawAllowed: true})
	if err != nil || state != evidence.RawRetained || string(out) != string(raw) {
		t.Fatalf("raw with permission = %q, %s, %v", out, state, err)
	}
	// Permission alone does not turn raw capture on.
	out, state, err = evidence.Prepare(raw, evidence.PrepareOptions{RawAllowed: true})
	if err != nil || state != evidence.Redacted || string(out) == string(raw) {
		t.Fatalf("permission without a request = %q, %s, %v", out, state, err)
	}
}
