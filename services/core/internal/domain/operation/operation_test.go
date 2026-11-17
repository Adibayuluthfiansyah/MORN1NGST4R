package operation_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/operation"
)

func baseSpec() operation.Spec {
	return operation.Spec{
		AssessmentID:   "asm-1",
		ToolKey:        "httpx",
		Kind:           "probe",
		Category:       operation.CategoryHTTPProbe,
		Target:         "app.example.com",
		Port:           443,
		Params:         map[string]string{operation.ParamHTTPMethod: "GET", "path_depth": "1"},
		Destinations:   []string{"app.example.com:443"},
		Limits:         operation.Limits{MaxConcurrency: 2, MaxDuration: 10 * time.Minute, RequestsPerMinute: 60},
		ManifestDigest: [32]byte{1},
	}
}

func mustNew(t *testing.T, s operation.Spec) operation.Operation {
	t.Helper()
	op, err := operation.New(s)
	if err != nil {
		t.Fatalf("New(%+v): %v", s, err)
	}
	return op
}

func TestZeroOperation(t *testing.T) {
	var op operation.Operation
	if !op.IsZero() || !op.Hash().IsZero() || op.Class() != operation.ClassUnknown || len(op.Canonical()) != 0 {
		t.Errorf("zero Operation: IsZero %v, Hash %s, Class %v, Canonical %d bytes", op.IsZero(), op.Hash(), op.Class(), len(op.Canonical()))
	}
	if good := mustNew(t, baseSpec()); good.IsZero() || good.Hash().IsZero() {
		t.Error("a built Operation reports zero")
	}
}

func TestNewRefusesBadSpecs(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	manyParams := map[string]string{}
	for i := 0; i < 33; i++ {
		manyParams[fmt.Sprintf("p%d", i)] = "v"
	}
	manyDests := []string{}
	for i := 0; i < 17; i++ {
		manyDests = append(manyDests, fmt.Sprintf("d%d.example.com:443", i))
	}
	cases := []struct {
		name string
		edit func(*operation.Spec)
	}{
		{"unknown category", func(s *operation.Spec) { s.Category = operation.CategoryUnknown }},
		{"category out of range", func(s *operation.Spec) { s.Category = operation.Category(77) }},
		{"negative category", func(s *operation.Spec) { s.Category = operation.Category(-1) }},
		{"empty assessment", func(s *operation.Spec) { s.AssessmentID = "" }},
		{"assessment with a space", func(s *operation.Spec) { s.AssessmentID = "asm 1" }},
		{"assessment too long", func(s *operation.Spec) { s.AssessmentID = long(65) }},
		{"empty tool key", func(s *operation.Spec) { s.ToolKey = "" }},
		{"tool key with a slash", func(s *operation.Spec) { s.ToolKey = "a/b" }},
		{"empty kind", func(s *operation.Spec) { s.Kind = "" }},
		{"kind non-ASCII", func(s *operation.Spec) { s.Kind = "prøbe" }},
		{"empty target", func(s *operation.Spec) { s.Target = "" }},
		{"target with a space", func(s *operation.Spec) { s.Target = "app example.com" }},
		{"target with a tab", func(s *operation.Spec) { s.Target = "app\texample.com" }},
		{"target with a newline", func(s *operation.Spec) { s.Target = "app.example.com\n" }},
		{"target with NUL", func(s *operation.Spec) { s.Target = "app\x00.com" }},
		{"target non-ASCII", func(s *operation.Spec) { s.Target = "exämple.com" }},
		{"target 256 bytes", func(s *operation.Spec) { s.Target = long(256) }},
		{"port zero", func(s *operation.Spec) { s.Port = 0 }},
		{"33 parameters", func(s *operation.Spec) { s.Params = manyParams }},
		{"parameter name upper case", func(s *operation.Spec) { s.Params = map[string]string{"Name": "v"} }},
		{"parameter name starts with a digit", func(s *operation.Spec) { s.Params = map[string]string{"1x": "v"} }},
		{"parameter name with a dash", func(s *operation.Spec) { s.Params = map[string]string{"a-b": "v"} }},
		{"empty parameter name", func(s *operation.Spec) { s.Params = map[string]string{"": "v"} }},
		{"parameter name 49 bytes", func(s *operation.Spec) { s.Params = map[string]string{"a" + long(48): "v"} }},
		{"parameter value 1025 bytes", func(s *operation.Spec) { s.Params = map[string]string{"x": long(1025)} }},
		{"parameter value with a newline", func(s *operation.Spec) { s.Params = map[string]string{"x": "a\nb"} }},
		{"parameter value with DEL", func(s *operation.Spec) { s.Params = map[string]string{"x": "a\x7fb"} }},
		{"parameter value not UTF-8", func(s *operation.Spec) { s.Params = map[string]string{"x": "a\xffb"} }},
		{"17 destinations", func(s *operation.Spec) { s.Destinations = manyDests }},
		{"empty destination", func(s *operation.Spec) { s.Destinations = []string{""} }},
		{"destination with a space", func(s *operation.Spec) { s.Destinations = []string{"a b:443"} }},
		{"destination 256 bytes", func(s *operation.Spec) { s.Destinations = []string{long(256)} }},
		{"duplicate destination", func(s *operation.Spec) { s.Destinations = []string{"a.example.com:443", "a.example.com:443"} }},
		{"negative concurrency", func(s *operation.Spec) { s.Limits.MaxConcurrency = -1 }},
		{"negative duration", func(s *operation.Spec) { s.Limits.MaxDuration = -1 }},
		{"negative rate", func(s *operation.Spec) { s.Limits.RequestsPerMinute = -1 }},
		{"zero manifest digest", func(s *operation.Spec) { s.ManifestDigest = [32]byte{} }},
		{"CONNECT method", func(s *operation.Spec) { s.Params[operation.ParamHTTPMethod] = "CONNECT" }},
		{"method with a space", func(s *operation.Spec) { s.Params[operation.ParamHTTPMethod] = "GE T" }},
		{"empty method", func(s *operation.Spec) { s.Params[operation.ParamHTTPMethod] = "" }},
		{"method with a digit", func(s *operation.Spec) { s.Params[operation.ParamHTTPMethod] = "GET1" }},
		{"method 17 letters", func(s *operation.Spec) { s.Params[operation.ParamHTTPMethod] = long(17) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := baseSpec()
			tc.edit(&s)
			op, err := operation.New(s)
			if !errors.Is(err, operation.ErrInvalid) {
				t.Errorf("New = %v, want ErrInvalid", err)
			}
			if !op.IsZero() {
				t.Error("an error came with a usable Operation")
			}
		})
	}
	// The boundaries themselves are accepted.
	s := baseSpec()
	s.Target = long(255)
	s.AssessmentID = long(64)
	s.Params = map[string]string{"x": long(1024)}
	s.Limits = operation.Limits{} // active, but not established: New accepts it, policy denies it
	mustNew(t, s)
}

func TestNewAcceptsLimitsThatAreNotEstablished(t *testing.T) {
	// A plan item can describe an operation before the numbers exist (D-71).
	// Approval and the decision function refuse it. New does not.
	s := baseSpec()
	s.Limits = operation.Limits{}
	op := mustNew(t, s)
	if op.Limits().Established() {
		t.Error("zero limits report as established")
	}
}

func TestClassIsDerivedAndNeverSupplied(t *testing.T) {
	for cat, want := range map[operation.Category]operation.RiskClass{
		operation.CategoryPassiveRecon:        operation.ClassPassive,
		operation.CategoryHTTPProbe:           operation.ClassLowImpact,
		operation.CategorySafeReplay:          operation.ClassLowImpact,
		operation.CategoryStateChangingReplay: operation.ClassIntrusive,
		operation.CategoryTemplateCheck:       operation.ClassIntrusive,
	} {
		s := baseSpec()
		s.Category = cat
		op := mustNew(t, s)
		if op.Class() != want || op.Category() != cat {
			t.Errorf("category %v: class %v, want %v", cat, op.Class(), want)
		}
	}
	// A "class" parameter is just a parameter. It has no effect (FR-APPROVAL-004 AC5).
	s := baseSpec()
	s.Params = map[string]string{"class": "passive", "risk_class": "passive", "risk": "passive"}
	s.Category = operation.CategoryTemplateCheck
	if got := mustNew(t, s).Class(); got != operation.ClassIntrusive {
		t.Errorf("a class given as a parameter changed the class to %v", got)
	}
}

// S1.6 item 8 and T-22: a parameter can raise the class and never lowers it.
func TestAStateChangingMethodRaisesTheClass(t *testing.T) {
	for _, cat := range []operation.Category{operation.CategoryHTTPProbe, operation.CategorySafeReplay, operation.CategoryPassiveRecon} {
		for _, m := range []string{"POST", "put", "DELETE", "PATCH", "Patch", "TRACE", "PROPFIND"} {
			s := baseSpec()
			s.Category = cat
			s.Params = map[string]string{operation.ParamHTTPMethod: m}
			if got := mustNew(t, s).Class(); got != operation.ClassIntrusive {
				t.Errorf("category %v with method %q: class %v, want intrusive", cat, m, got)
			}
		}
		for _, m := range []string{"GET", "head", "OPTIONS", "Get"} {
			s := baseSpec()
			s.Category = cat
			s.Params = map[string]string{operation.ParamHTTPMethod: m}
			if got := mustNew(t, s).Class(); got != operation.ClassOf(cat) {
				t.Errorf("category %v with safe method %q: class %v, want %v", cat, m, got, operation.ClassOf(cat))
			}
		}
	}
	// Nothing lowers an intrusive category.
	s := baseSpec()
	s.Category = operation.CategoryStateChangingReplay
	s.Params = map[string]string{operation.ParamHTTPMethod: "GET"}
	if got := mustNew(t, s).Class(); got != operation.ClassIntrusive {
		t.Errorf("a GET lowered an intrusive category to %v", got)
	}
}

func TestSameFieldsGiveTheSameHashWhateverTheOrder(t *testing.T) {
	a := baseSpec()
	a.Params = map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"}
	a.Destinations = []string{"z.example.com:443", "a.example.com:443", "m.example.com:443"}
	want := mustNew(t, a).Hash()
	for i := 0; i < 50; i++ { // map iteration order differs between runs
		b := baseSpec()
		b.Params = map[string]string{"d": "4", "c": "3", "b": "2", "a": "1"}
		b.Destinations = []string{"m.example.com:443", "z.example.com:443", "a.example.com:443"}
		if got := mustNew(t, b).Hash(); got != want {
			t.Fatalf("hash depends on order: %s != %s", got, want)
		}
	}
}

// FR-APPROVAL-001 AC1 and AC2: a different target or parameter is a different
// operation and therefore a different hash.
func TestEveryFieldChangesTheHash(t *testing.T) {
	base := mustNew(t, baseSpec()).Hash()
	edits := map[string]func(*operation.Spec){
		"assessment":      func(s *operation.Spec) { s.AssessmentID = "asm-2" },
		"tool key":        func(s *operation.Spec) { s.ToolKey = "httpy" },
		"kind":            func(s *operation.Spec) { s.Kind = "probe2" },
		"category":        func(s *operation.Spec) { s.Category = operation.CategorySafeReplay },
		"target":          func(s *operation.Spec) { s.Target = "app.example.org" },
		"port":            func(s *operation.Spec) { s.Port = 8443 },
		"parameter value": func(s *operation.Spec) { s.Params["path_depth"] = "2" },
		"parameter name": func(s *operation.Spec) {
			s.Params["path_depth2"] = s.Params["path_depth"]
			delete(s.Params, "path_depth")
		},
		"extra parameter":         func(s *operation.Spec) { s.Params["extra"] = "x" },
		"removed parameter":       func(s *operation.Spec) { delete(s.Params, "path_depth") },
		"destination":             func(s *operation.Spec) { s.Destinations = []string{"app.example.com:8443"} },
		"extra destination":       func(s *operation.Spec) { s.Destinations = append(s.Destinations, "cdn.example.com:443") },
		"no destination":          func(s *operation.Spec) { s.Destinations = nil },
		"concurrency":             func(s *operation.Spec) { s.Limits.MaxConcurrency = 3 },
		"duration":                func(s *operation.Spec) { s.Limits.MaxDuration = 11 * time.Minute },
		"request rate":            func(s *operation.Spec) { s.Limits.RequestsPerMinute = 61 },
		"manifest digest":         func(s *operation.Spec) { s.ManifestDigest[31] = 9 },
		"method raises the class": func(s *operation.Spec) { s.Params[operation.ParamHTTPMethod] = "POST" },
	}
	for name, edit := range edits {
		s := baseSpec()
		edit(&s)
		if got := mustNew(t, s).Hash(); got == base {
			t.Errorf("changing the %s did not change the hash", name)
		}
	}
}

// A length-prefixed form has no field-boundary ambiguity.
func TestNoTwoFieldLayoutsCollide(t *testing.T) {
	pairs := []struct {
		name string
		a, b func(*operation.Spec)
	}{
		{"assessment and tool key", func(s *operation.Spec) { s.AssessmentID, s.ToolKey = "ab", "c" }, func(s *operation.Spec) { s.AssessmentID, s.ToolKey = "a", "bc" }},
		{"tool key and kind", func(s *operation.Spec) { s.ToolKey, s.Kind = "ab", "c" }, func(s *operation.Spec) { s.ToolKey, s.Kind = "a", "bc" }},
		{"parameter name and value", func(s *operation.Spec) { s.Params = map[string]string{"a": "bc"} }, func(s *operation.Spec) { s.Params = map[string]string{"ab": "c"} }},
		{"two parameters and one", func(s *operation.Spec) { s.Params = map[string]string{"a": "b", "c": "d"} }, func(s *operation.Spec) { s.Params = map[string]string{"a": "bcd"} }},
		{"destinations", func(s *operation.Spec) { s.Destinations = []string{"a", "b"} }, func(s *operation.Spec) { s.Destinations = []string{"ab"} }},
		{"parameters against destinations", func(s *operation.Spec) { s.Params = map[string]string{"a": "b"}; s.Destinations = nil }, func(s *operation.Spec) { s.Params = nil; s.Destinations = []string{"a", "b"} }},
	}
	for _, p := range pairs {
		a, b := baseSpec(), baseSpec()
		a.Params, a.Destinations, b.Params, b.Destinations = nil, nil, nil, nil
		p.a(&a)
		p.b(&b)
		if mustNew(t, a).Hash() == mustNew(t, b).Hash() {
			t.Errorf("%s: two different layouts have the same hash", p.name)
		}
	}
}

func TestCanonicalFormMatchesTheHash(t *testing.T) {
	op := mustNew(t, baseSpec())
	c := op.Canonical()
	if !bytes.HasPrefix(c, []byte("msop\x01")) {
		t.Errorf("canonical form starts with %q, want the msop version 1 prefix", c[:5])
	}
	if sha256.Sum256(c) != [32]byte(op.Hash()) {
		t.Error("the hash is not the SHA-256 of the canonical form")
	}
	// A caller cannot change the stored form through the returned slice.
	c[0] = 'X'
	if op.Canonical()[0] != 'm' {
		t.Error("Canonical returned the internal slice")
	}
}

func TestAnOperationIsImmutable(t *testing.T) {
	s := baseSpec()
	op := mustNew(t, s)
	want := op.Hash()
	// Changing the Spec after New does nothing.
	s.Params["path_depth"] = "99"
	s.Destinations[0] = "evil.example.com:443"
	if op.Hash() != want {
		t.Error("changing the Spec after New changed the Operation")
	}
	// Changing what an accessor returned does nothing either.
	op.Params()["path_depth"] = "99"
	op.Destinations()[0] = "evil.example.com:443"
	if v, _ := op.Param("path_depth"); v != "1" || op.Destinations()[0] != "app.example.com:443" {
		t.Error("an accessor returned internal state")
	}
	if op.Hash() != want {
		t.Error("the hash changed")
	}
	if _, ok := op.Param("missing"); ok {
		t.Error("Param reported a missing parameter")
	}
	if d := op.ManifestDigest(); d != [32]byte{1} {
		t.Errorf("ManifestDigest = %v", d)
	}
	if op.AssessmentID() != "asm-1" || op.ToolKey() != "httpx" || op.Kind() != "probe" || op.Target() != "app.example.com" || op.Port() != 443 {
		t.Error("accessors return other values than the Spec")
	}
}

func TestHashText(t *testing.T) {
	h := mustNew(t, baseSpec()).Hash()
	s := h.String()
	if len(s) != 64 || strings.ToLower(s) != s {
		t.Fatalf("Hash.String() = %q, want 64 lower-case hex characters", s)
	}
	back, err := operation.ParseHash(s)
	if err != nil || back != h {
		t.Errorf("ParseHash(%q) = %v, %v", s, back, err)
	}
	for _, in := range []string{"", "00", strings.Repeat("0", 63), strings.Repeat("0", 65), strings.ToUpper(s), strings.Repeat("g", 64), " " + s[1:]} {
		if got, err := operation.ParseHash(in); !errors.Is(err, operation.ErrInvalid) || !got.IsZero() {
			t.Errorf("ParseHash(%q) = %v, %v, want ErrInvalid", in, got, err)
		}
	}
	var z operation.Hash
	if !z.IsZero() || h.IsZero() {
		t.Error("IsZero is wrong")
	}
}

func FuzzOperationHash(f *testing.F) {
	f.Add("asm-1", "httpx", "app.example.com", "GET", uint16(443))
	f.Add("a", "b", "c", "POST", uint16(1))
	f.Add("", "", "", "", uint16(0))
	f.Add("asm-1", "ab", "x\x00y", "\xff", uint16(65535))
	f.Fuzz(func(t *testing.T, assessment, tool, target, method string, port uint16) {
		s := operation.Spec{
			AssessmentID: assessment, ToolKey: tool, Kind: "probe", Category: operation.CategoryHTTPProbe,
			Target: target, Port: port, Params: map[string]string{operation.ParamHTTPMethod: method, "n": "1"},
			Limits: operation.Limits{MaxConcurrency: 1, MaxDuration: time.Second, RequestsPerMinute: 1}, ManifestDigest: [32]byte{7},
		}
		op, err := operation.New(s) // must not panic
		if err != nil {
			if !errors.Is(err, operation.ErrInvalid) || !op.IsZero() {
				t.Fatalf("New returned %v with a usable value", err)
			}
			return
		}
		again, err := operation.New(s)
		if err != nil || again.Hash() != op.Hash() {
			t.Fatalf("not deterministic: %v, %v", err, again.Hash() == op.Hash())
		}
		if sha256.Sum256(op.Canonical()) != [32]byte(op.Hash()) {
			t.Fatal("hash is not the SHA-256 of the canonical form")
		}
		// A different target that is also valid is a different operation.
		s.Target = target + "x"
		if other, err := operation.New(s); err == nil && other.Hash() == op.Hash() {
			t.Fatalf("targets %q and %q have the same hash", target, target+"x")
		}
		// The class never goes below the category class.
		if op.Class() < operation.ClassOf(operation.CategoryHTTPProbe) {
			t.Fatalf("class %v is below the category class", op.Class())
		}
	})
}
