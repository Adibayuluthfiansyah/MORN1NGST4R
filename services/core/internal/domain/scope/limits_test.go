package scope_test

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

// F-9 of the M1 review. The input limits are not new policy. Each one is the
// longest text that any valid input can have, so a longer input is invalid
// whatever it contains and is refused before anything is allocated:
//
//   - host: 253 octets (S1.1 step 4) and exactly one trailing dot (step 2): 254
//   - address: the longest literal is an IPv6 address with a dotted tail,
//     0000:0000:0000:0000:0000:ffff:255.255.255.255: 45
//   - prefix: an address and "/128": 49
//   - ports: the list of every port, "1,2,...,65535": 382103. A longer list has
//     a duplicate and is invalid.
//
// These tests check the numbers by building the longest valid input.

func TestTheLongestValidInputsAreAccepted(t *testing.T) {
	if got := len(fullPortList()); got != 382103 {
		t.Fatalf("the full port list is %d bytes, the documented limit is 382103", got)
	}
	if _, err := scope.ParsePorts(fullPortList()); err != nil {
		t.Errorf("the longest valid port list was refused: %v", err)
	}
	if h, err := scope.ParseHost(name253() + "."); err != nil || len(h) != 253 {
		t.Errorf("the longest valid host (253 octets and a dot, 254 bytes) = %q, %v", h, err)
	}
	const longestAddr = "0000:0000:0000:0000:0000:ffff:255.255.255.255"
	if len(longestAddr) != 45 {
		t.Fatalf("test constant is %d bytes, want 45", len(longestAddr))
	}
	if a, err := scope.ParseAddr(longestAddr); err != nil || a.String() != "255.255.255.255" {
		t.Errorf("the longest valid address literal = %v, %v", a, err)
	}
	if p, err := scope.ParseInclusionPrefix(longestAddr+"/128", scope.DefaultPrefixMin()); err != nil || p.String() != "255.255.255.255/32" {
		t.Errorf("the longest valid prefix = %v, %v", p, err)
	}
	if _, err := scope.ParseExclusionPrefix(longestAddr + "/128"); err != nil {
		t.Errorf("the longest valid prefix as an exclusion: %v", err)
	}
	// 2001:db8 with eight full groups is 39 bytes and is valid too.
	if _, err := scope.ParseAddr("2001:0db8:0000:0000:0000:0000:0000:0001"); err != nil {
		t.Errorf("eight full groups: %v", err)
	}
}

func TestInputsOverTheLimitAreRefused(t *testing.T) {
	over := map[string]func() error{
		"host of 255 bytes": func() error { _, err := scope.ParseHost(strings.Repeat("a", 255)); return err },
		"host, one dot over": func() error {
			_, err := scope.ParseHost(name253() + "..")
			return err
		},
		"address of 46 bytes": func() error {
			_, err := scope.ParseAddr("0000:0000:0000:0000:0000:ffff:255.255.255.255" + "0")
			return err
		},
		"inclusion prefix of 50 bytes": func() error {
			_, err := scope.ParseInclusionPrefix("0000:0000:0000:0000:0000:ffff:255.255.255.255/1280", scope.DefaultPrefixMin())
			return err
		},
		"exclusion prefix of 50 bytes": func() error {
			_, err := scope.ParseExclusionPrefix("0000:0000:0000:0000:0000:ffff:255.255.255.255/1280")
			return err
		},
		"port list one byte over": func() error { _, err := scope.ParsePorts(fullPortList() + ","); return err },
		"target of 255 bytes":     func() error { _, err := scope.ParseTarget(strings.Repeat("a", 255), 443); return err },
	}
	for name, f := range over {
		if err := f(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// The oversized input must be refused without copying it. Each call below gets
// ten megabytes. The package may allocate for its error and nothing more.
func TestHugeInputsAreRefusedWithoutAllocatingForThem(t *testing.T) {
	const size = 10 << 20
	const allowed = 16 << 10 // a 64 KiB scratch table for the ports parser would already fail this
	huge := map[string]struct {
		input string
		call  func(string) error
	}{
		"host, upper case":      {strings.Repeat("A", size), func(s string) error { _, err := scope.ParseHost(s); return err }},
		"host, dots":            {strings.Repeat("a.", size/2), func(s string) error { _, err := scope.ParseHost(s); return err }},
		"address":               {strings.Repeat("1", size), func(s string) error { _, err := scope.ParseAddr(s); return err }},
		"inclusion prefix":      {strings.Repeat("1", size) + "/24", func(s string) error { _, err := scope.ParseInclusionPrefix(s, scope.DefaultPrefixMin()); return err }},
		"exclusion prefix":      {strings.Repeat("1", size) + "/24", func(s string) error { _, err := scope.ParseExclusionPrefix(s); return err }},
		"ports, commas":         {strings.Repeat(",", size), func(s string) error { _, err := scope.ParsePorts(s); return err }},
		"ports, numbers":        {strings.Repeat("80,", size/3), func(s string) error { _, err := scope.ParsePorts(s); return err }},
		"target, host":          {strings.Repeat("A", size), func(s string) error { _, err := scope.ParseTarget(s, 443); return err }},
		"target, colon":         {strings.Repeat(":", size), func(s string) error { _, err := scope.ParseTarget(s, 443); return err }},
		"rule value, domain":    {strings.Repeat("a", size), func(s string) error { _, err := scope.NewRule(D, false, s, "", defMin); return err }},
		"rule value, wildcard":  {"*." + strings.Repeat("a", size), func(s string) error { _, err := scope.NewRule(W, true, s, "", defMin); return err }},
		"rule value, cidr":      {strings.Repeat("1", size), func(s string) error { _, err := scope.NewRule(C, true, s, "", defMin); return err }},
		"rule ports":            {strings.Repeat(",", size), func(s string) error { _, err := scope.NewRule(D, false, "example.com", s, defMin); return err }},
		"rule value as an ip":   {strings.Repeat("1", size), func(s string) error { _, err := scope.NewRule(I, false, s, "", defMin); return err }},
		"exclusion error input": {strings.Repeat("a", size), func(s string) error { _, err := scope.NewRule(D, true, s, "", defMin); return err }},
	}
	for name, tc := range huge {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		err := tc.call(tc.input)
		runtime.ReadMemStats(&after)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if used := after.TotalAlloc - before.TotalAlloc; used > allowed {
			t.Errorf("%s: allocated %d bytes for a %d byte input, want at most %d", name, used, len(tc.input), allowed)
		}
	}
}

// The limit must not hide the reason: an over-long input still reports the
// same error classes as before.
func TestOverLongInputsKeepTheirErrorClass(t *testing.T) {
	if _, err := scope.ParseHost(strings.Repeat("a", 300)); !errors.Is(err, scope.ErrAmbiguous) {
		t.Errorf("host: %v", err)
	}
	if _, err := scope.ParseAddr(strings.Repeat("1", 300)); !errors.Is(err, scope.ErrAmbiguous) {
		t.Errorf("address: %v", err)
	}
	if _, err := scope.ParsePorts(strings.Repeat("1,", 300000)); !errors.Is(err, scope.ErrInvalidPorts) {
		t.Errorf("ports: %v", err)
	}
	var ue *scope.UnsupportedExclusionError
	if _, err := scope.NewRule(D, true, strings.Repeat("a", 300), "", defMin); !errors.As(err, &ue) || len(ue.Input) != 300 {
		t.Errorf("exclusion: %v", err)
	}
}

// The limits are visible in the error: an input over the limit says so, an
// input at the limit is judged on its content. This keeps every cap tested
// without timing.
func TestOverTheLimitSaysSo(t *testing.T) {
	const over = "longer than"
	atLimitAddr := "0000:0000:0000:0000:0000:ffff:255.255.255.255" // 45 bytes
	cases := []struct {
		name string
		err  error
	}{
		{"host, 255 bytes", func() error { _, err := scope.ParseHost(strings.Repeat("a", 255)); return err }()},
		{"address, 46 bytes", func() error { _, err := scope.ParseAddr(atLimitAddr + "0"); return err }()},
		{"inclusion prefix, 50 bytes", func() error {
			_, err := scope.ParseInclusionPrefix(atLimitAddr+"/1280", scope.DefaultPrefixMin())
			return err
		}()},
		{"exclusion prefix, 50 bytes", func() error { _, err := scope.ParseExclusionPrefix(atLimitAddr + "/1280"); return err }()},
		{"port list, one element over", func() error { _, err := scope.ParsePorts(fullPortList() + ",1"); return err }()},
	}
	for _, tc := range cases {
		if tc.err == nil || !strings.Contains(tc.err.Error(), over) {
			t.Errorf("%s: error %v does not report the limit", tc.name, tc.err)
		}
	}
	// At the limit the content decides, not the length.
	for name, err := range map[string]error{
		"host, 254 bytes":   func() error { _, err := scope.ParseHost(name253() + "."); return err }(),
		"address, 45 bytes": func() error { _, err := scope.ParseAddr(atLimitAddr); return err }(),
		"inclusion prefix, 49 bytes": func() error {
			_, err := scope.ParseInclusionPrefix(atLimitAddr+"/128", scope.DefaultPrefixMin())
			return err
		}(),
		"exclusion prefix, 49 bytes": func() error { _, err := scope.ParseExclusionPrefix(atLimitAddr + "/128"); return err }(),
		"port list, 382103 bytes":    func() error { _, err := scope.ParsePorts(fullPortList()); return err }(),
	} {
		if err != nil {
			t.Errorf("%s: refused at the limit: %v", name, err)
		}
	}
}
