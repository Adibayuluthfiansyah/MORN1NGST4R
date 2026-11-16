package scope_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

var defMin = scope.DefaultPrefixMin()

func TestNewRuleAccepts(t *testing.T) {
	tests := []struct {
		name      string
		kind      scope.RuleKind
		exclude   bool
		value     string
		ports     string
		wantHost  string
		wantAddr  string
		wantPfx   string
		wantPorts string
	}{
		{"domain inclusion, default ports", scope.KindDomain, false, "Example.COM.", "", "example.com", "", "", "80,443"},
		{"domain inclusion, listed ports", scope.KindDomain, false, "example.com", "8443,80", "example.com", "", "", "80,8443"},
		{"domain exclusion, all ports", scope.KindDomain, true, "example.com", "", "example.com", "", "", "1-65535"},
		{"domain exclusion, narrowed ports", scope.KindDomain, true, "example.com", "8443", "example.com", "", "", "8443"},
		{"wildcard exclusion", scope.KindWildcard, true, "*.internal.example.com", "", "internal.example.com", "", "", "1-65535"},
		{"wildcard exclusion is canonicalized", scope.KindWildcard, true, "*.Internal.Example.COM.", "", "internal.example.com", "", "", "1-65535"},
		{"wildcard exclusion over a public suffix is accepted, it only denies", scope.KindWildcard, true, "*.co.uk", "", "co.uk", "", "", "1-65535"},
		{"wildcard exclusion over a TLD", scope.KindWildcard, true, "*.com", "", "com", "", "", "1-65535"},
		{"wildcard exclusion with ports", scope.KindWildcard, true, "*.x.example.com", "22", "x.example.com", "", "", "22"},
		{"ip inclusion", scope.KindIP, false, "203.0.113.7", "", "", "203.0.113.7", "", "80,443"},
		{"ip inclusion, mapped form", scope.KindIP, false, "::ffff:203.0.113.7", "", "", "203.0.113.7", "", "80,443"},
		{"ip inclusion, IPv6", scope.KindIP, false, "2001:DB8::7", "8443", "", "2001:db8::7", "", "8443"},
		{"ip exclusion", scope.KindIP, true, "203.0.113.7", "", "", "203.0.113.7", "", "1-65535"},
		{"cidr inclusion /24", scope.KindCIDR, false, "203.0.113.0/24", "", "", "", "203.0.113.0/24", "80,443"},
		{"cidr inclusion /25", scope.KindCIDR, false, "203.0.113.0/25", "443", "", "", "203.0.113.0/25", "443"},
		{"cidr inclusion /48", scope.KindCIDR, false, "2001:db8:1::/48", "", "", "", "2001:db8:1::/48", "80,443"},
		{"cidr exclusion /16", scope.KindCIDR, true, "198.51.0.0/16", "", "", "", "198.51.0.0/16", "1-65535"},
		{"cidr exclusion /0", scope.KindCIDR, true, "0.0.0.0/0", "", "", "", "0.0.0.0/0", "1-65535"},
		{"cidr exclusion IPv6 /32", scope.KindCIDR, true, "2001:db8::/32", "", "", "", "2001:db8::/32", "1-65535"},
		{"cidr exclusion ::/0", scope.KindCIDR, true, "::/0", "", "", "", "::/0", "1-65535"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// An exclusion must be accepted whatever minimum the caller passes.
			for _, min := range []scope.PrefixMin{defMin, {}} {
				if !tc.exclude && !min.IsSet() {
					continue
				}
				r, err := scope.NewRule(tc.kind, tc.exclude, tc.value, tc.ports, min)
				if err != nil {
					t.Fatalf("NewRule error: %v", err)
				}
				if r.Kind() != tc.kind || r.Exclude() != tc.exclude {
					t.Errorf("kind/exclude = %v/%v, want %v/%v", r.Kind(), r.Exclude(), tc.kind, tc.exclude)
				}
				if string(r.Host()) != tc.wantHost {
					t.Errorf("Host = %q, want %q", r.Host(), tc.wantHost)
				}
				if tc.wantAddr == "" && r.Addr().IsValid() || tc.wantAddr != "" && r.Addr().String() != tc.wantAddr {
					t.Errorf("Addr = %v, want %q", r.Addr(), tc.wantAddr)
				}
				if tc.wantPfx == "" && r.Prefix().IsValid() || tc.wantPfx != "" && r.Prefix().String() != tc.wantPfx {
					t.Errorf("Prefix = %v, want %q", r.Prefix(), tc.wantPfx)
				}
				if r.Ports().String() != tc.wantPorts {
					t.Errorf("Ports = %q, want %q", r.Ports().String(), tc.wantPorts)
				}
			}
		})
	}
}

// D-35 and D-73: wildcard inclusion stays refused. No parameter, no
// confirmation and no value makes one valid.
func TestNewRuleRefusesWildcardInclusion(t *testing.T) {
	for _, v := range []string{"*.example.com", "*.a.b.c.d.example.com", "*.co.uk", "*.com", "*.EXAMPLE.com.", "*.example.com "} {
		for _, ports := range []string{"", "443", "80,443,8443"} {
			r, err := scope.NewRule(scope.KindWildcard, false, v, ports, defMin)
			if !errors.Is(err, scope.ErrWildcardInclusion) {
				t.Errorf("NewRule(wildcard inclusion %q, ports %q) = %+v, %v, want ErrWildcardInclusion", v, ports, r, err)
			}
			var ue *scope.UnsupportedExclusionError
			if errors.As(err, &ue) {
				t.Errorf("an inclusion error must not be an UnsupportedExclusionError: %v", err)
			}
		}
	}
	// A wildcard written under another kind is not a way around it.
	for _, kind := range []scope.RuleKind{scope.KindDomain, scope.KindIP, scope.KindCIDR} {
		if r, err := scope.NewRule(kind, false, "*.example.com", "", defMin); err == nil {
			t.Errorf("NewRule(%v, \"*.example.com\") = %+v, want an error", kind, r)
		}
	}
}

func TestNewRuleRefusesMalformedInclusions(t *testing.T) {
	tests := []struct {
		name  string
		kind  scope.RuleKind
		value string
		ports string
		want  error
	}{
		// Constructs outside the grammar are refused, never widened (D-34).
		{"URL", scope.KindDomain, "https://example.com", "", scope.ErrAmbiguous},
		{"URL path", scope.KindDomain, "example.com/admin", "", scope.ErrAmbiguous},
		{"trailing slash", scope.KindDomain, "example.com/", "", scope.ErrAmbiguous},
		{"scheme without slashes", scope.KindDomain, "http:example.com", "", scope.ErrAmbiguous},
		{"host and port in the value", scope.KindDomain, "example.com:443", "", scope.ErrAmbiguous},
		{"regular expression", scope.KindDomain, `^example\.com$`, "", scope.ErrAmbiguous},
		{"alternation", scope.KindDomain, "(a|b).example.com", "", scope.ErrAmbiguous},
		{"character class", scope.KindDomain, "[ab].example.com", "", scope.ErrAmbiguous},
		{"question mark", scope.KindDomain, "exampl?.com", "", scope.ErrAmbiguous},
		{"userinfo", scope.KindDomain, "user@example.com", "", scope.ErrAmbiguous},
		{"underscore", scope.KindDomain, "a_b.example.com", "", scope.ErrAmbiguous},
		{"non-ASCII", scope.KindDomain, "exämple.com", "", scope.ErrAmbiguous},
		{"empty", scope.KindDomain, "", "", scope.ErrAmbiguous},
		{"numeric host as a domain", scope.KindDomain, "127.1", "", scope.ErrAmbiguous},
		{"IP literal as a domain", scope.KindDomain, "203.0.113.7", "", scope.ErrAmbiguous},
		{"hostname as an ip", scope.KindIP, "example.com", "", scope.ErrAmbiguous},
		{"short IPv4 as an ip", scope.KindIP, "127.1", "", scope.ErrAmbiguous},
		{"zone as an ip", scope.KindIP, "::1%lo", "", scope.ErrAmbiguous},
		{"bracketed ip", scope.KindIP, "[::1]", "", scope.ErrAmbiguous},
		{"NAT64 as an ip", scope.KindIP, "64:ff9b::cb00:7107", "", scope.ErrAmbiguous},
		{"cidr as an ip", scope.KindIP, "203.0.113.0/24", "", scope.ErrAmbiguous},
		{"ip as a cidr", scope.KindCIDR, "203.0.113.7", "", scope.ErrAmbiguous},
		{"cidr /16 inclusion", scope.KindCIDR, "198.51.0.0/16", "", scope.ErrPrefixTooShort},
		{"cidr /0 inclusion", scope.KindCIDR, "0.0.0.0/0", "", scope.ErrPrefixTooShort},
		{"cidr IPv6 /32 inclusion", scope.KindCIDR, "2001:db8::/32", "", scope.ErrPrefixTooShort},
		{"cidr host bits", scope.KindCIDR, "203.0.113.5/24", "", scope.ErrAmbiguous},
		{"cidr shorthand", scope.KindCIDR, "10/8", "", scope.ErrAmbiguous},
		{"cidr in a translation range", scope.KindCIDR, "64:ff9b::/96", "", scope.ErrAmbiguous},
		{"ports zero", scope.KindDomain, "example.com", "0", scope.ErrInvalidPorts},
		{"ports 65536", scope.KindDomain, "example.com", "65536", scope.ErrInvalidPorts},
		{"ports leading zero", scope.KindIP, "203.0.113.7", "08080", scope.ErrInvalidPorts},
		{"ports duplicate", scope.KindCIDR, "203.0.113.0/24", "80,80", scope.ErrInvalidPorts},
		{"ports with a space", scope.KindDomain, "example.com", "80, 443", scope.ErrInvalidPorts},
		{"ports range", scope.KindDomain, "example.com", "80-90", scope.ErrInvalidPorts},
		{"unknown kind", scope.RuleKind(0), "example.com", "", scope.ErrUnknownKind},
		{"out of range kind", scope.RuleKind(99), "example.com", "", scope.ErrUnknownKind},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := scope.NewRule(tc.kind, false, tc.value, tc.ports, defMin)
			if !errors.Is(err, tc.want) {
				t.Fatalf("NewRule = %+v, %v, want %v", r, err, tc.want)
			}
			var ue *scope.UnsupportedExclusionError
			if errors.As(err, &ue) {
				t.Errorf("an inclusion error is an UnsupportedExclusionError: %v", err)
			}
		})
	}
}

// Choice 10 of record 0002: a refused exclusion is never dropped silently.
// The error carries the original text and is distinguishable from the error of
// a refused inclusion, so the application can record it and block readiness.
func TestNewRuleReportsRefusedExclusions(t *testing.T) {
	tests := []struct {
		name  string
		kind  scope.RuleKind
		value string
		ports string
		cause error
	}{
		{"URL", scope.KindDomain, "https://example.com/admin", "", scope.ErrAmbiguous},
		{"regular expression", scope.KindDomain, `.*\.example\.com`, "", scope.ErrAmbiguous},
		{"non-ASCII wildcard exclusion", scope.KindWildcard, "*.exämple.com", "", scope.ErrAmbiguous},
		{"wildcard without a star", scope.KindWildcard, "example.com", "", scope.ErrAmbiguous},
		{"star in the middle", scope.KindWildcard, "a.*.example.com", "", scope.ErrAmbiguous},
		{"double star", scope.KindWildcard, "**.example.com", "", scope.ErrAmbiguous},
		{"two stars", scope.KindWildcard, "*.*.example.com", "", scope.ErrAmbiguous},
		{"bare star", scope.KindWildcard, "*", "", scope.ErrAmbiguous},
		{"star and dot only", scope.KindWildcard, "*.", "", scope.ErrAmbiguous},
		{"star without a dot", scope.KindWildcard, "*example.com", "", scope.ErrAmbiguous},
		{"numeric base", scope.KindWildcard, "*.127.1", "", scope.ErrAmbiguous},
		{"ip exclusion of a hostname", scope.KindIP, "example.com", "", scope.ErrAmbiguous},
		{"cidr host bits", scope.KindCIDR, "203.0.113.5/24", "", scope.ErrAmbiguous},
		{"cidr shorthand", scope.KindCIDR, "10/8", "", scope.ErrAmbiguous},
		{"cidr in a translation range", scope.KindCIDR, "2002::/16", "", scope.ErrAmbiguous},
		{"invalid ports", scope.KindDomain, "example.com", "65536", scope.ErrInvalidPorts},
		{"duplicate ports", scope.KindDomain, "example.com", "22,22", scope.ErrInvalidPorts},
		{"unknown kind", scope.RuleKind(7), "example.com", "", scope.ErrUnknownKind},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := scope.NewRule(tc.kind, true, tc.value, tc.ports, defMin)
			var ue *scope.UnsupportedExclusionError
			if !errors.As(err, &ue) {
				t.Fatalf("NewRule = %+v, %v, want an *UnsupportedExclusionError", r, err)
			}
			if ue.Input != tc.value {
				t.Errorf("Input = %q, want the original text %q", ue.Input, tc.value)
			}
			if ue.Ports != tc.ports {
				t.Errorf("Ports = %q, want %q", ue.Ports, tc.ports)
			}
			if !errors.Is(err, scope.ErrUnsupportedExclusion) {
				t.Errorf("errors.Is(err, ErrUnsupportedExclusion) is false for %v", err)
			}
			if !errors.Is(err, tc.cause) {
				t.Errorf("the cause %v is not reachable through errors.Is: %v", tc.cause, err)
			}
			if strings.Contains(err.Error(), tc.value) && len(tc.value) > 4 {
				t.Errorf("the error text echoes the input: %v", err)
			}
		})
	}
}

func TestExclusionErrorIsDistinctFromInclusionErrors(t *testing.T) {
	_, incl := scope.NewRule(scope.KindDomain, false, "https://example.com", "", defMin)
	_, excl := scope.NewRule(scope.KindDomain, true, "https://example.com", "", defMin)
	var ue *scope.UnsupportedExclusionError
	if errors.As(incl, &ue) || errors.Is(incl, scope.ErrUnsupportedExclusion) {
		t.Errorf("inclusion error looks like an exclusion error: %v", incl)
	}
	if !errors.As(excl, &ue) {
		t.Errorf("exclusion error is not an UnsupportedExclusionError: %v", excl)
	}
}

// A forgotten minimum must not mean "no minimum". Only an exclusion may pass a
// zero PrefixMin (choice 1 of record 0002).
func TestInclusionCIDRNeedsAnExplicitMinimum(t *testing.T) {
	var unset scope.PrefixMin
	for _, v := range []string{"203.0.113.0/24", "198.51.0.0/16", "0.0.0.0/0", "2001:db8:1::/48"} {
		r, err := scope.NewRule(scope.KindCIDR, false, v, "", unset)
		if !errors.Is(err, scope.ErrMinimumUnset) {
			t.Errorf("NewRule(cidr inclusion %q, unset minimum) = %+v, %v, want ErrMinimumUnset", v, r, err)
		}
	}
	// Domain and ip inclusions do not use the minimum.
	if _, err := scope.NewRule(scope.KindDomain, false, "example.com", "", unset); err != nil {
		t.Errorf("a domain inclusion needs no minimum: %v", err)
	}
	if _, err := scope.NewRule(scope.KindIP, false, "203.0.113.7", "", unset); err != nil {
		t.Errorf("an ip inclusion needs no minimum: %v", err)
	}
	// An explicit policy that raises the minimum is honoured.
	strict, _ := scope.NewPrefixMin(28, 56)
	if _, err := scope.NewRule(scope.KindCIDR, false, "203.0.113.0/24", "", strict); !errors.Is(err, scope.ErrPrefixTooShort) {
		t.Errorf("a /24 inclusion under a /28 minimum: %v, want ErrPrefixTooShort", err)
	}
}

func TestRuleCannotBeChangedFromOutside(t *testing.T) {
	r, err := scope.NewRule(scope.KindDomain, false, "example.com", "80,443", defMin)
	if err != nil {
		t.Fatal(err)
	}
	ports := r.Ports().Ports()
	ports[0] = 22
	if r.Ports().Contains(22) {
		t.Error("changing a returned slice changed the rule")
	}
	var zero scope.Rule
	if zero.Kind() != 0 {
		t.Errorf("zero Rule has kind %v, want the invalid kind", zero.Kind())
	}
}
