package scope_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

func fullPortList() string {
	parts := make([]string, 0, 65535)
	for i := 1; i <= 65535; i++ {
		parts = append(parts, strconv.Itoa(i))
	}
	return strings.Join(parts, ",")
}

// F-2 of the M1 review: the canonical text of a port set must read back to the
// same set, and every spelling of "all ports" must be one set.
func TestAllPortsHaveOneCanonicalForm(t *testing.T) {
	want := scope.AllPorts()
	for name, in := range map[string]string{
		"range token":          "1-65535",
		"every port, listed":   fullPortList(),
		"every port, reversed": reverseList(fullPortList()),
	} {
		got, err := scope.ParsePorts(in)
		if err != nil {
			t.Errorf("%s: error %v", name, err)
			continue
		}
		if !got.All() || got.String() != "1-65535" || got.String() != want.String() {
			t.Errorf("%s: All=%v String=%q, want the all-ports set", name, got.All(), got.String())
		}
		for _, p := range []uint16{1, 22, 65535} {
			if !got.Contains(p) {
				t.Errorf("%s: does not contain %d", name, p)
			}
		}
		if got.Contains(0) {
			t.Errorf("%s: contains port 0", name)
		}
		if got.Ports() != nil {
			t.Errorf("%s: an all-ports set has no list", name)
		}
	}
}

func reverseList(s string) string {
	parts := strings.Split(s, ",")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ",")
}

// One port short of all ports is an ordinary list.
func TestAlmostAllPortsStayAList(t *testing.T) {
	parts := strings.Split(fullPortList(), ",")
	almost := strings.Join(parts[:len(parts)-1], ",") // 1..65534
	got, err := scope.ParsePorts(almost)
	if err != nil {
		t.Fatal(err)
	}
	if got.All() || got.Contains(65535) || !got.Contains(65534) {
		t.Errorf("1..65534: All=%v contains65535=%v", got.All(), got.Contains(65535))
	}
}

func TestOnlyTheExactRangeTokenIsAccepted(t *testing.T) {
	for _, in := range []string{
		"1-65534", "0-65535", "1-65536", "2-65535", "1-65535,80", "80,1-65535", "1 - 65535", " 1-65535", "1-65535 ",
		"01-65535", "1-065535", "1-", "-65535", "1--65535", "1–65535", "1-65535-1", "80-90", "1-2", "*", "all",
	} {
		if ps, err := scope.ParsePorts(in); !errors.Is(err, scope.ErrInvalidPorts) {
			t.Errorf("ParsePorts(%q) = %v, %v, want ErrInvalidPorts", in, ps, err)
		}
	}
}

func TestPortSetStringReadsBack(t *testing.T) {
	for _, in := range []string{"80", "443,80", "1,65535", "22,8443,80", "1-65535", "1,2,3"} {
		ps, err := scope.ParsePorts(in)
		if err != nil {
			t.Fatalf("ParsePorts(%q): %v", in, err)
		}
		again, err := scope.ParsePorts(ps.String())
		if err != nil || again.String() != ps.String() || again.All() != ps.All() {
			t.Errorf("%q -> %q does not read back: %v, %v", in, ps.String(), again.String(), err)
		}
	}
	for _, ps := range []scope.PortSet{scope.AllPorts(), scope.DefaultInScopePorts()} {
		again, err := scope.ParsePorts(ps.String())
		if err != nil || again.String() != ps.String() {
			t.Errorf("default set %q does not read back: %v", ps.String(), err)
		}
	}
}
