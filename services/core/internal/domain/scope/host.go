package scope

import (
	"errors"
	"fmt"
	"strings"
)

// ErrAmbiguous means that an input has no single exact reading, or is outside
// the grammar. It is denied, never repaired (D-34, D-37, D-38).
var ErrAmbiguous = errors.New("ambiguous target")

// ambiguous wraps ErrAmbiguous with a fixed reason. The reason never contains
// the input.
func ambiguous(reason string) error { return fmt.Errorf("%w: %s", ErrAmbiguous, reason) }

const (
	maxLabelLen = 63
	maxNameLen  = 253

	// maxHostInput is the longest valid input: a name of 253 octets and exactly
	// one trailing dot (S1.1 steps 2 and 4). A longer input is invalid whatever
	// it contains, so it is refused before any scan or allocation (F-9).
	maxHostInput = maxNameLen + 1

	// forbiddenHostBytes are refused anywhere in a host name (S1.1 step 1).
	forbiddenHostBytes = `/\@?#%:[]*,`
)

// Host is a canonical host name: lower-case ASCII, no trailing dot, labels of
// letters, digits and hyphen. A numeric spelling of an address is not a Host.
type Host string

// ParseHost returns the canonical form of a host name, or an error that wraps
// ErrAmbiguous. It strips exactly one trailing dot and lower-cases. It refuses
// everything else that is not already canonical.
//
// Names are ASCII only. International names are converted in the application
// layer (M1b). A non-ASCII byte here is ambiguous (D-73).
func ParseHost(s string) (Host, error) {
	if s == "" {
		return "", ambiguous("empty name")
	}
	if len(s) > maxHostInput {
		return "", ambiguous("longer than any host name")
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 0x80:
			return "", ambiguous("non-ASCII byte")
		case c <= 0x20 || c == 0x7f:
			return "", ambiguous("whitespace or control character")
		case strings.IndexByte(forbiddenHostBytes, c) >= 0:
			return "", ambiguous("forbidden character")
		}
	}

	// Exactly one trailing dot is removed. Anything left that is empty or still
	// ends in a dot is ambiguous.
	s = strings.TrimSuffix(s, ".")
	if s == "" || strings.HasSuffix(s, ".") {
		return "", ambiguous("empty label")
	}
	s = strings.ToLower(s) // the string is ASCII here
	if len(s) > maxNameLen {
		return "", ambiguous("name longer than 253 octets")
	}

	labels := strings.Split(s, ".")
	for _, l := range labels {
		if l == "" {
			return "", ambiguous("empty label")
		}
		if len(l) > maxLabelLen {
			return "", ambiguous("label longer than 63 octets")
		}
		if l[0] == '-' || l[len(l)-1] == '-' {
			return "", ambiguous("label starts or ends with a hyphen")
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", ambiguous("character outside letters, digits and hyphen")
			}
		}
	}

	// A name whose last label is a number is never a domain (S1.1 step 6). An
	// all-digit label, and 0x followed by any number of hex digits, are numeric
	// spellings of an address. Canonical four-part IPv4 belongs to ParseAddr.
	if isNumericLabel(labels[len(labels)-1]) {
		return "", ambiguous("numeric host")
	}
	return Host(s), nil
}

func isNumericLabel(l string) bool {
	if isAllDigits(l) {
		return true
	}
	if rest, ok := strings.CutPrefix(l, "0x"); ok {
		for i := 0; i < len(rest); i++ {
			c := rest[i]
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
		return true
	}
	return false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
