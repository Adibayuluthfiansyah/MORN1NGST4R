package scope

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ErrInvalidPorts means that a port list is outside the grammar: port 0, 65536
// or more, a leading zero, a duplicate, a range other than the all-ports token,
// or any stray character (D-36).
var ErrInvalidPorts = errors.New("invalid port set")

func invalidPorts(reason string) error { return fmt.Errorf("%w: %s", ErrInvalidPorts, reason) }

const (
	maxPort = 65535

	// allPortsText is the canonical text of every port from 1 to 65535. It is
	// what PortSet.String returns for the full set and the one range that
	// ParsePorts reads (data dictionary: ports_canonical, for example
	// "80,443" or "1-65535").
	allPortsText = "1-65535"

	// The list of every port, "1,2,...,65535", is the longest valid list: it
	// has 316569 digits and 65534 commas. A longer list has a duplicate and is
	// invalid, so it is refused by length. TestTheLongestValidInputsAreAccepted
	// checks the number.
	maxPortsInput = 382103
)

// PortSet is a set of TCP ports from 1 to 65535. The zero value is empty and
// contains no port, so a forgotten set never matches.
type PortSet struct {
	all  bool
	list []uint16 // sorted, unique, never changed after construction
}

// DefaultInScopePorts is the port set of an inclusion without ports: 80 and
// 443 only (D-36).
func DefaultInScopePorts() PortSet { return PortSet{list: []uint16{80, 443}} }

// AllPorts is the port set of an exclusion without ports: 1 to 65535 (D-36).
func AllPorts() PortSet { return PortSet{all: true} }

// ParsePorts parses a comma separated list of decimal ports without spaces,
// for example "80,8443", or the token "1-65535" for every port. The canonical
// form is sorted. A list that names every port is the same set as the token.
// It refuses 0, 65536, leading zeros, duplicates, empty elements, any other
// range and anything else.
func ParsePorts(s string) (PortSet, error) {
	if s == allPortsText {
		return AllPorts(), nil
	}
	if s == "" {
		return PortSet{}, invalidPorts("empty list")
	}
	if len(s) > maxPortsInput {
		return PortSet{}, invalidPorts("longer than any valid port list")
	}

	seen := make([]bool, maxPort+1)
	var list []uint16
	for rest := s; ; {
		el, tail, more := strings.Cut(rest, ",")
		if el == "" {
			return PortSet{}, invalidPorts("empty element")
		}
		if len(el) > 5 || !isAllDigits(el) {
			return PortSet{}, invalidPorts("not a decimal number")
		}
		if len(el) > 1 && el[0] == '0' {
			return PortSet{}, invalidPorts("leading zero")
		}
		n, err := strconv.Atoi(el)
		if err != nil || n < 1 || n > maxPort {
			return PortSet{}, invalidPorts("port outside 1 to 65535")
		}
		if seen[n] {
			return PortSet{}, invalidPorts("duplicate port")
		}
		seen[n] = true
		list = append(list, uint16(n))
		if !more {
			break
		}
		rest = tail
	}
	if len(list) == maxPort {
		return AllPorts(), nil // the same set, so it has one canonical form
	}
	sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
	return PortSet{list: list}, nil
}

// Contains reports whether the set holds the port. Port 0 is never in a set.
func (p PortSet) Contains(port uint16) bool {
	if port == 0 {
		return false
	}
	if p.all {
		return true
	}
	for _, q := range p.list {
		if q == port {
			return true
		}
	}
	return false
}

// IsEmpty reports whether the set contains no port.
func (p PortSet) IsEmpty() bool { return !p.all && len(p.list) == 0 }

// All reports whether the set is every port from 1 to 65535.
func (p PortSet) All() bool { return p.all }

// Ports returns a copy of the listed ports. It is nil for AllPorts, which has
// no list. Changing the copy does not change the set.
func (p PortSet) Ports() []uint16 {
	if p.all || len(p.list) == 0 {
		return nil
	}
	out := make([]uint16, len(p.list))
	copy(out, p.list)
	return out
}

// String returns the canonical text that ParsePorts reads back to the same
// set: "1-65535" for every port, otherwise the sorted comma separated list.
func (p PortSet) String() string {
	if p.all {
		return allPortsText
	}
	parts := make([]string, len(p.list))
	for i, q := range p.list {
		parts[i] = strconv.Itoa(int(q))
	}
	return strings.Join(parts, ",")
}
