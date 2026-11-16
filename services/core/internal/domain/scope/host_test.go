package scope_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/scope"
)

func label(n int) string { return strings.Repeat("a", n) }

// name253 is a name of exactly 253 octets made of valid labels.
func name253() string {
	return strings.Join([]string{label(63), label(63), label(63), label(61)}, ".")
}

func TestParseHostAccepts(t *testing.T) {
	tests := []struct{ in, want string }{
		{"example.com", "example.com"},
		{"Example.COM.", "example.com"}, // case and exactly one trailing dot
		{"EXAMPLE.com", "example.com"},
		{"a.b.c.d.example.com", "a.b.c.d.example.com"},
		{"a-b.example.com", "a-b.example.com"},
		{"123.example.com", "123.example.com"}, // digits in a non-final label are fine
		{"localhost", "localhost"},
		{"xn--fu-hia.de", "xn--fu-hia.de"}, // an A-label is plain ASCII here. IDNA checks belong to M1b
		{"XN--FU-HIA.DE", "xn--fu-hia.de"},
		{"1a", "1a"},
		// F-8 of the M1 review: M1 does not validate A-labels or the "--" rule. IDNA
		// validation is milestone M1b. These pin that M1 only checks the ASCII syntax.
		{"xn--a", "xn--a"},
		{"xn--zzzzzzzz.example.com", "xn--zzzzzzzz.example.com"},
		{"ab--cd.example.com", "ab--cd.example.com"},
		{"foo.0xzz", "foo.0xzz"}, // not hexadecimal, so not a numeric host
		{"foo.1e3", "foo.1e3"},
		{label(63) + ".example.com", label(63) + ".example.com"},
		{name253(), name253()},
		{name253() + ".", name253()},
	}
	for _, tc := range tests {
		got, err := scope.ParseHost(tc.in)
		if err != nil {
			t.Errorf("ParseHost(%q) error: %v", tc.in, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("ParseHost(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseHostRefusesAsAmbiguous(t *testing.T) {
	tests := []struct{ name, in string }{
		// Empty and dot forms.
		{"empty", ""},
		{"only a dot", "."},
		{"two dots", ".."},
		{"double trailing dot", "example.com.."},
		{"leading dot", ".example.com"},
		{"empty label", "example..com"},
		// Forbidden and control characters.
		{"userinfo", "user@host"},
		{"userinfo and port", "user@host.example.com:80"},
		{"underscore", "a_b.example.com"},
		{"leading underscore", "_dmarc.example.com"},
		{"space inside", "ex ample.com"},
		{"leading space", " example.com"},
		{"trailing space", "example.com "},
		{"tab", "example.com\t"},
		{"newline", "example.com\n"},
		{"carriage return", "example.com\r"},
		{"NUL", "exa\x00mple.com"},
		{"DEL", "exa\x7fmple.com"},
		{"slash", "example.com/path"},
		{"backslash", "example.com\\x"},
		{"question mark", "example.com?x"},
		{"hash", "example.com#x"},
		{"percent", "exa%41mple.com"},
		{"colon", "example.com:443"},
		{"bracket", "[::1]"},
		{"bracket in name", "a[b].com"},
		{"comma", "a.com,b.com"},
		{"asterisk", "*.example.com"},
		{"asterisk inside", "a*.example.com"},
		{"scheme", "https://example.com"},
		{"punctuation", "exa!mple.com"},
		// Non-ASCII is refused in M1. Conversion is M1b (D-73).
		{"non-ASCII letter", "exämple.com"},
		{"CJK", "例え.jp"},
		{"ideographic full stop", "example。com"},
		{"fullwidth digits", "１２７.0.0.1"},
		{"soft hyphen", "exa\u00admple.com"},
		{"zero width space", "ex\u200bample.com"},
		{"right-to-left override", "\u202eexample.com"},
		{"BOM", "\ufeffexample.com"},
		{"lookalike Cyrillic a", "ex\u0430mple.com"},
		{"mixed script", "exаmple.рф"},
		{"invalid UTF-8", "exa\xffmple.com"},
		// Lengths.
		{"label of 64 octets", label(64) + ".example.com"},
		{"name of 254 octets", name253() + "a"},
		{"name of 254 octets with trailing dot", name253() + "a."},
		// Hyphens.
		{"leading hyphen", "-a.example.com"},
		{"trailing hyphen", "a-.example.com"},
		{"lone hyphen", "-"},
		// Numeric host spellings are never domains.
		{"short IPv4", "127.1"},
		{"decimal IPv4", "2130706433"},
		{"hex IPv4 with dots", "0x7f.0.0.1"},
		{"hex IPv4 single", "0x7f000001"},
		{"upper-case hex", "0X7F.0.0.1"},
		{"octal IPv4", "0177.0.0.1"},
		{"leading zeros", "127.000.000.001"},
		{"octet over 255", "1.2.3.256"},
		{"five parts", "1.2.3.4.5"},
		{"hex last label", "foo.0x7f"},
		{"bare 0x last label", "foo.0x"},
		{"digit last label", "foo.123"},
		{"canonical IPv4 is an Addr, not a Host", "1.2.3.4"},
		{"upper-case hex last label", "foo.0X7F"},
		{"upper-case bare 0X last label", "foo.0X"},
		{"over the 254-byte input limit", label(63) + "." + label(63) + "." + label(63) + "." + label(62) + ".."},
		{"canonical IPv4 with trailing dot", "1.2.3.4."},
		{"single digit", "1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scope.ParseHost(tc.in)
			if !errors.Is(err, scope.ErrAmbiguous) {
				t.Fatalf("ParseHost(%q) = %q, %v, want ErrAmbiguous", tc.in, got, err)
			}
			if got != "" {
				t.Errorf("ParseHost(%q) returned %q together with an error", tc.in, got)
			}
		})
	}
}

// The error text must not echo the input. It may be shown to the researcher
// next to the input, and it may be logged.
func TestParseHostErrorDoesNotEchoInput(t *testing.T) {
	const marker = "MARKER-INPUT-7f3a"
	_, err := scope.ParseHost("user@" + marker + ".example.com")
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), marker) {
		t.Errorf("error echoes the input: %v", err)
	}
}

func TestParseHostIsIdempotent(t *testing.T) {
	for _, in := range []string{"Example.COM.", "a.B.c", "xn--FU-hia.de", name253(), "LOCALHOST"} {
		h, err := scope.ParseHost(in)
		if err != nil {
			t.Fatalf("ParseHost(%q): %v", in, err)
		}
		again, err := scope.ParseHost(string(h))
		if err != nil || again != h {
			t.Errorf("ParseHost(ParseHost(%q)) = %q, %v, want %q", in, again, err, h)
		}
	}
}

// A second trailing dot is never repaired.
func TestParseHostStripsExactlyOneTrailingDot(t *testing.T) {
	if _, err := scope.ParseHost("example.com.."); !errors.Is(err, scope.ErrAmbiguous) {
		t.Errorf("two trailing dots: %v, want ErrAmbiguous", err)
	}
	if _, err := scope.ParseHost("example.com..."); !errors.Is(err, scope.ErrAmbiguous) {
		t.Errorf("three trailing dots: %v, want ErrAmbiguous", err)
	}
}

func FuzzParseHost(f *testing.F) {
	for _, s := range []string{"example.com", "Example.COM.", "a_b", "127.1", "0x7f.0.0.1", "exämple.com", "a..b", ".", "", "*.x.com", "a@b", strings.Repeat("a", 300)} {
		f.Add(s)
	}
	const forbidden = "/\\@?#%:[]*, \t\r\n\x00"
	f.Fuzz(func(t *testing.T, s string) {
		h, err := scope.ParseHost(s) // must not panic
		if err != nil {
			if h != "" {
				t.Fatalf("ParseHost(%q) returned %q with error %v", s, h, err)
			}
			if !errors.Is(err, scope.ErrAmbiguous) {
				t.Fatalf("ParseHost(%q) error %v is not ErrAmbiguous", s, err)
			}
			return
		}
		again, err := scope.ParseHost(string(h))
		if err != nil || again != h {
			t.Fatalf("not idempotent: %q -> %q -> %q, %v", s, h, again, err)
		}
		if len(h) == 0 || len(h) > 253 || strings.HasSuffix(string(h), ".") {
			t.Fatalf("bad canonical length or trailing dot: %q", h)
		}
		for i := 0; i < len(h); i++ {
			c := h[i]
			if c >= 0x80 || c < 0x20 || c == 0x7f || strings.IndexByte(forbidden, c) >= 0 || (c >= 'A' && c <= 'Z') {
				t.Fatalf("canonical host %q contains forbidden byte %#x", h, c)
			}
		}
	})
}
