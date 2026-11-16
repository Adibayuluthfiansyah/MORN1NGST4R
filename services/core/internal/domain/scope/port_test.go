package scope_test

import (
	"errors"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

func TestParsePortsAccepts(t *testing.T) {
	tests := []struct{ in, want string }{
		{"80", "80"},
		{"8443", "8443"},
		{"1", "1"},
		{"65535", "65535"},
		{"443,80", "80,443"}, // canonical form is sorted
		{"8443,80,443", "80,443,8443"},
	}
	for _, tc := range tests {
		ps, err := scope.ParsePorts(tc.in)
		if err != nil {
			t.Errorf("ParsePorts(%q) error: %v", tc.in, err)
			continue
		}
		if ps.String() != tc.want {
			t.Errorf("ParsePorts(%q) = %q, want %q", tc.in, ps.String(), tc.want)
		}
	}
}

func TestParsePortsRefuses(t *testing.T) {
	tests := []struct{ name, in string }{
		{"zero", "0"},
		{"65536", "65536"},
		{"leading zero", "08080"},
		{"leading zeros on a valid port", "0080"},
		{"duplicate", "80,80"},
		{"duplicate out of order", "80,443,80"},
		{"empty", ""},
		{"trailing comma", "80,"},
		{"leading comma", ",80"},
		{"empty element", "80,,443"},
		{"leading space", " 80"},
		{"trailing space", "80 "},
		{"space after comma", "80, 443"},
		{"letters", "8o"},
		{"negative", "-1"},
		{"plus sign", "+80"},
		{"range", "80-90"},
		{"hex", "0x50"},
		{"exponent", "1e3"},
		{"overflow", "99999999999999999999"},
		{"non-ASCII digits", "٨٠"},
		{"wildcard", "*"},
		{"all", "all"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps, err := scope.ParsePorts(tc.in)
			if !errors.Is(err, scope.ErrInvalidPorts) {
				t.Fatalf("ParsePorts(%q) = %v, %v, want ErrInvalidPorts", tc.in, ps, err)
			}
			if ps.Contains(80) || ps.Contains(443) {
				t.Errorf("a refused port set still contains ports: %v", ps)
			}
		})
	}
}

func TestDefaultPortSets(t *testing.T) {
	in := scope.DefaultInScopePorts()
	for _, p := range []uint16{80, 443} {
		if !in.Contains(p) {
			t.Errorf("default in-scope ports do not contain %d", p)
		}
	}
	for _, p := range []uint16{0, 1, 22, 81, 8080, 8443, 65535} {
		if in.Contains(p) {
			t.Errorf("default in-scope ports contain %d, want only 80 and 443", p)
		}
	}

	all := scope.AllPorts()
	for _, p := range []uint16{1, 22, 80, 443, 8443, 65535} {
		if !all.Contains(p) {
			t.Errorf("AllPorts does not contain %d", p)
		}
	}
	if all.Contains(0) {
		t.Error("AllPorts contains port 0")
	}
}

// A zero PortSet matches nothing. A rule built without ports must therefore
// never silently cover everything, and an exclusion with an empty set would
// cover nothing, so the rule layer refuses to build one.
func TestZeroPortSetContainsNothing(t *testing.T) {
	var zero scope.PortSet
	for _, p := range []uint16{0, 1, 80, 443, 65535} {
		if zero.Contains(p) {
			t.Errorf("zero PortSet contains %d", p)
		}
	}
	if !zero.IsEmpty() {
		t.Error("zero PortSet is not empty")
	}
	if scope.AllPorts().IsEmpty() || scope.DefaultInScopePorts().IsEmpty() {
		t.Error("a default port set is empty")
	}
}

func TestPortSetCannotBeChangedFromOutside(t *testing.T) {
	ps, err := scope.ParsePorts("80,443")
	if err != nil {
		t.Fatal(err)
	}
	list := ps.Ports()
	list[0] = 22
	if ps.Contains(22) || !ps.Contains(80) {
		t.Errorf("changing the returned slice changed the set: %v", ps)
	}
	d := scope.DefaultInScopePorts()
	_ = d.Ports()
	d2 := scope.DefaultInScopePorts()
	if d2.Contains(22) {
		t.Error("a second DefaultInScopePorts call is affected by an earlier one")
	}
}

func TestParsePortsIsIdempotentThroughString(t *testing.T) {
	ps, err := scope.ParsePorts("8443,80,443")
	if err != nil {
		t.Fatal(err)
	}
	again, err := scope.ParsePorts(ps.String())
	if err != nil || again.String() != ps.String() {
		t.Errorf("re-parse of %q = %q, %v", ps.String(), again.String(), err)
	}
}

func FuzzParsePorts(f *testing.F) {
	for _, s := range []string{"80", "80,443", "0", "65536", "08080", "80,80", "", ",", "1-2", "99999999999"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		ps, err := scope.ParsePorts(s) // must not panic
		if err != nil {
			if !errors.Is(err, scope.ErrInvalidPorts) || !ps.IsEmpty() {
				t.Fatalf("ParsePorts(%q) = %v, %v", s, ps, err)
			}
			return
		}
		if ps.IsEmpty() || ps.Contains(0) {
			t.Fatalf("ParsePorts(%q) produced an empty set or contains 0", s)
		}
		again, err := scope.ParsePorts(ps.String())
		if err != nil || again.String() != ps.String() {
			t.Fatalf("not idempotent: %q -> %q -> %q, %v", s, ps.String(), again.String(), err)
		}
	})
}
