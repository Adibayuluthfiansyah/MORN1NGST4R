package scope

import (
	"errors"
	"net/netip"
	"strings"
)

// RuleKind is the kind of a scope rule. Only these four exist (D-34). The zero
// value is not a valid kind.
type RuleKind int

const (
	KindDomain   RuleKind = iota + 1 // example.com: exactly that host
	KindWildcard                     // *.example.com: one or more labels below the base, never the base
	KindIP                           // 203.0.113.7: exactly that address
	KindCIDR                         // 203.0.113.0/24: any address in the range
)

func (k RuleKind) String() string {
	switch k {
	case KindDomain:
		return "domain"
	case KindWildcard:
		return "wildcard"
	case KindIP:
		return "ip"
	case KindCIDR:
		return "cidr"
	}
	return "unknown"
}

var (
	// ErrWildcardInclusion means that a wildcard inclusion was refused. They
	// stay refused for the whole MVP, with no confirmation and no exception
	// (D-35, D-73). A wildcard exclusion is accepted.
	ErrWildcardInclusion = errors.New("wildcard inclusion rules are not supported")

	// ErrUnknownKind means that the rule kind is not one of the four kinds.
	ErrUnknownKind = errors.New("unknown rule kind")

	// ErrMinimumUnset means that an in-scope CIDR was offered without a set
	// PrefixMin (the zero value, or one that NewPrefixMin refused). A missing
	// minimum never means no minimum.
	ErrMinimumUnset = errors.New("in-scope cidr rule needs a set prefix minimum")

	// ErrUnsupportedExclusion matches every *UnsupportedExclusionError through
	// errors.Is.
	ErrUnsupportedExclusion = errors.New("unsupported exclusion rule")
)

// UnsupportedExclusionError reports an exclusion that the grammar refuses. An
// exclusion is never dropped silently (choice 10 of record 0002). The caller
// records the error with the original text and keeps the assessment from
// becoming ready until a human acts on it.
//
// An error for a refused inclusion is never of this type, so errors.As tells
// the two apart.
type UnsupportedExclusionError struct {
	Kind  RuleKind
	Input string // the original rule text, exactly as given
	Ports string // the original port text, exactly as given
	Err   error  // why it was refused
}

// Error returns a static text. It does not contain Input.
func (e *UnsupportedExclusionError) Error() string {
	return "unsupported exclusion rule: " + e.Err.Error()
}

// Unwrap exposes the cause, for example ErrAmbiguous or ErrInvalidPorts.
func (e *UnsupportedExclusionError) Unwrap() error { return e.Err }

// Is makes errors.Is(err, ErrUnsupportedExclusion) true.
func (e *UnsupportedExclusionError) Is(target error) bool { return target == ErrUnsupportedExclusion }

// Rule is one validated scope rule. A Rule can be built only by NewRule, so
// every Rule is canonical and complete. The zero Rule has no kind and is
// invalid: a RuleSet that holds one denies everything.
type Rule struct {
	kind    RuleKind
	exclude bool
	host    Host
	addr    netip.Addr
	prefix  netip.Prefix
	ports   PortSet
}

// Kind returns the rule kind.
func (r Rule) Kind() RuleKind { return r.kind }

// Exclude reports whether the rule is an exclusion.
func (r Rule) Exclude() bool { return r.exclude }

// Host returns the canonical host of a domain rule, or the canonical base of a
// wildcard rule (without the leading "*."). It is empty for other kinds.
func (r Rule) Host() Host { return r.host }

// Addr returns the address of an ip rule. It is invalid for other kinds.
func (r Rule) Addr() netip.Addr { return r.addr }

// Prefix returns the prefix of a cidr rule. It is invalid for other kinds.
func (r Rule) Prefix() netip.Prefix { return r.prefix }

// Ports returns the ports that the rule covers. Ports().String() is the
// canonical port text and NewRule reads it back.
func (r Rule) Ports() PortSet { return r.ports }

// Value returns the canonical rule text, in the form that NewRule reads back:
// the host for a domain rule, "*." and the base for a wildcard rule, the
// address for an ip rule and the prefix for a cidr rule. It is the
// canonical_value of the data dictionary. With Ports().String() it rebuilds the
// same rule: NewRule(r.Kind(), r.Exclude(), r.Value(), r.Ports().String(), min)
// returns an equal rule. An inclusion CIDR is read back under the minimum that
// is in force then, so it is refused if the minimum has been raised since. The
// zero Rule has no value.
func (r Rule) Value() string {
	switch r.kind {
	case KindDomain:
		return string(r.host)
	case KindWildcard:
		return "*." + string(r.host)
	case KindIP:
		return r.addr.String()
	case KindCIDR:
		return r.prefix.String()
	}
	return ""
}

// valid reports whether the rule is fully formed.
func (r Rule) valid() bool {
	if r.ports.IsEmpty() {
		return false
	}
	switch r.kind {
	case KindDomain:
		return r.host != ""
	case KindWildcard:
		return r.host != "" && r.exclude // a wildcard inclusion never exists
	case KindIP:
		return r.addr.IsValid()
	case KindCIDR:
		return r.prefix.IsValid()
	}
	return false
}

// NewRule validates and canonicalizes one rule.
//
// value is the rule text: a host name, "*." and a base for a wildcard, an
// address, or a prefix. ports is a port list as ParsePorts reads it, or "" for
// the default: 80 and 443 for an inclusion, every port for an exclusion.
//
// min is the minimum prefix length for an in-scope CIDR and must be fully set
// (ErrMinimumUnset otherwise). It is ignored for exclusions, which are exempt
// from it (choice 1).
//
// Any failure of an exclusion is returned as *UnsupportedExclusionError.
func NewRule(kind RuleKind, exclude bool, value, ports string, min PrefixMin) (Rule, error) {
	r, err := newRule(kind, exclude, value, ports, min)
	if err != nil && exclude {
		return Rule{}, &UnsupportedExclusionError{Kind: kind, Input: value, Ports: ports, Err: err}
	}
	return r, err
}

func newRule(kind RuleKind, exclude bool, value, ports string, min PrefixMin) (Rule, error) {
	r := Rule{kind: kind, exclude: exclude}

	switch kind {
	case KindDomain:
		h, err := ParseHost(value)
		if err != nil {
			return Rule{}, err
		}
		r.host = h
	case KindWildcard:
		if !exclude {
			return Rule{}, ErrWildcardInclusion
		}
		base, ok := strings.CutPrefix(value, "*.")
		if !ok {
			return Rule{}, ambiguous("wildcard rule must start with \"*.\"")
		}
		h, err := ParseHost(base) // refuses any further "*"
		if err != nil {
			return Rule{}, err
		}
		r.host = h
	case KindIP:
		a, err := ParseAddr(value)
		if err != nil {
			return Rule{}, err
		}
		r.addr = a
	case KindCIDR:
		// The two modes are separate functions, so a zero or partial minimum
		// can never turn an inclusion into an exclusion. An exclusion has no
		// minimum and ignores min.
		var p netip.Prefix
		var err error
		if exclude {
			p, err = ParseExclusionPrefix(value)
		} else {
			p, err = ParseInclusionPrefix(value, min)
		}
		if err != nil {
			return Rule{}, err
		}
		r.prefix = p
	default:
		return Rule{}, ErrUnknownKind
	}

	switch {
	case ports != "":
		ps, err := ParsePorts(ports)
		if err != nil {
			return Rule{}, err
		}
		r.ports = ps
	case exclude:
		r.ports = AllPorts()
	default:
		r.ports = DefaultInScopePorts()
	}
	return r, nil
}
