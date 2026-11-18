package evidence

import (
	"bytes"
	"errors"
	"regexp"
)

// ErrRawNotAllowed is returned when raw capture is asked for and the assessment
// does not allow it (D-77).
var ErrRawNotAllowed = errors.New("raw capture is not allowed for this assessment")

// PrepareOptions says how a capture is stored. Raw asks to keep the bytes as
// they came. It works only when RawAllowed is true, which is the per-assessment
// policy setting that a human turned on. RawAllowed alone changes nothing.
type PrepareOptions struct {
	Raw        bool
	RawAllowed bool
}

// Prepare returns the bytes to store and what they are. By default they are
// redacted (D-77). A raw capture without the permission is refused, so a caller
// cannot keep secrets on disk by accident.
func Prepare(content []byte, o PrepareOptions) ([]byte, Redaction, error) {
	if o.Raw {
		if !o.RawAllowed {
			return nil, "", ErrRawNotAllowed
		}
		return append([]byte(nil), content...), RawRetained, nil
	}
	out, _ := Redact(content)
	return out, Redacted, nil
}

const mask = "[REDACTED]"

type pass struct {
	re    *regexp.Regexp
	build func(g [][]byte) []byte
}

// A match that the pass would rewrite to the same bytes is not counted, so
// redacting redacted text changes and counts nothing.
func (p pass) apply(b []byte) ([]byte, int) {
	n := 0
	out := p.re.ReplaceAllFunc(b, func(m []byte) []byte {
		r := p.build(p.re.FindSubmatch(m))
		if !bytes.Equal(r, m) {
			n++
		}
		return r
	})
	return out, n
}

func fixed(s string) func([][]byte) []byte { return func([][]byte) []byte { return []byte(s) } }

// keep returns group 1, a space or a separator, and the mask.
func keepAnd(suffix string) func([][]byte) []byte {
	return func(g [][]byte) []byte { return append(append([]byte(nil), g[1]...), suffix...) }
}

const secretKey = `(?:password|passwd|pwd|secret|api[_-]?key|token|private[_-]?key|session[_-]?id|credential)`

const headerName = `(?:authorization|proxy[-_]authorization|cookie|set[-_]cookie|x[-_]api[-_]key|api[-_]key|x[-_]auth[-_]token|x[-_]access[-_]token|x[-_]amz[-_]security[-_]token)`

var passes = []pass{
	// A private key block, and one that was cut off before its end line.
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), fixed("[REDACTED PRIVATE KEY]")},
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*`), fixed("[REDACTED PRIVATE KEY]")},
	// Headers that carry a credential. The name stays, the value goes. The value
	// runs to the end of the line and takes quoted parts whole, so a header value
	// that holds quotes (a cookie, a digest) goes entirely and a header inside a
	// quoted command argument is caught too. A header may lose more than its own
	// value when quotes follow on the same line: that is the safe side.
	{regexp.MustCompile(`(?i)(\b` + headerName + `[ \t]*:)[ \t]*(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\r\n"'])*`), keepAnd(" " + mask)},
	// The same headers as the keys of a JSON or Python object, such as the header
	// map of a tool that prints JSON lines.
	{regexp.MustCompile(`(?i)(["']` + headerName + `["'][ \t]*:[ \t]*)(?:"(?:[^"\\\r\n]|\\.)*"|'(?:[^'\\\r\n]|\\.)*')`), keepAnd(`"` + mask + `"`)},
	// Credentials in a URL.
	{regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s:@]+:[^/\s@]+@`), func(g [][]byte) []byte { return append(append([]byte(nil), g[1]...), mask+"@"...) }},
	// A secret value after a key, in a form, a query, a header-like line or JSON:
	// first a quoted value, which may hold spaces, then an unquoted one.
	{regexp.MustCompile(`(?i)(["']?[a-z0-9_.-]*` + secretKey + `[a-z0-9_.-]*["']?[ \t]*[:=][ \t]*)"(?:[^"\\\r\n]|\\.)*"`), keepAnd(`"` + mask + `"`)},
	{regexp.MustCompile(`(?i)(["']?[a-z0-9_.-]*` + secretKey + `[a-z0-9_.-]*["']?[ \t]*[:=][ \t]*)'(?:[^'\\\r\n]|\\.)*'`), keepAnd(`'` + mask + `'`)},
	{regexp.MustCompile(`(?i)(["']?[a-z0-9_.-]*` + secretKey + `[a-z0-9_.-]*["']?[ \t]*[:=][ \t]*)[^"'&\s;,}][^&\s;,}]*`), keepAnd(mask)},
	// Bearer and basic credentials anywhere.
	{regexp.MustCompile(`(?i)\b(bearer|basic)[ \t]+[A-Za-z0-9._~+/=-]{8,}`), func(g [][]byte) []byte { return append(append([]byte(nil), g[1]...), " "+mask...) }},
	// Well-known token shapes.
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), fixed(mask)},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`), fixed(mask)},
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), fixed(mask)},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), fixed(mask)},
	{regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), fixed(mask)},
	{regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`), fixed(mask)},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9]{32,}`), fixed(mask)},
}

// Redact removes the credentials that D-77 names from content and returns the
// result with the number of values it removed. It works on any bytes: binary
// content is kept as it is except for the secrets in it. It cannot know a secret
// format that it was not given (T-49), so the redacted bytes are the evidence of
// record and are not a guarantee.
func Redact(content []byte) ([]byte, int) {
	total := 0
	out := content
	for _, p := range passes {
		var n int
		out, n = p.apply(out)
		total += n
	}
	if total == 0 {
		return append([]byte(nil), content...), 0
	}
	return out, total
}
