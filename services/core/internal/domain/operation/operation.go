package operation

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strings"
	"unicode/utf8"
)

// ParamHTTPMethod is the reserved parameter name that carries an HTTP method. A
// method that is not safe raises the class to Intrusive (S1.6 item 8, T-22).
const ParamHTTPMethod = "http_method"

const (
	maxIDLen       = 64
	maxTargetLen   = 255
	maxParams      = 32
	maxParamName   = 48
	maxParamValue  = 1024
	maxDestination = 16
	maxMethodLen   = 16
)

// Spec is everything that identifies an operation. There is no class field:
// the class is derived from the category and the parameters.
type Spec struct {
	AssessmentID string
	// ToolKey and Kind name the tool and the operation in the reviewed manifest.
	ToolKey  string
	Kind     string
	Category Category
	// Target is the canonical target string and Port its port.
	Target string
	Port   uint16
	// Params are typed, validated and redacted by the caller. They hold no secrets.
	Params map[string]string
	// Destinations are the declared non-target network destinations (D-44).
	Destinations []string
	Limits       Limits
	// ManifestDigest is the SHA-256 of the reviewed manifest version. A zero
	// digest means there is no reviewed manifest, and the operation is refused.
	ManifestDigest [32]byte
}

// Operation is one security operation in canonical form. It is immutable: New
// copies its input and the accessors return copies. The zero Operation is not
// usable and has the zero Hash.
type Operation struct {
	spec      Spec
	class     RiskClass
	canonical []byte
	hash      Hash
}

// New validates a Spec and builds the canonical Operation. Every refusal
// returns ErrInvalid and an unusable Operation.
func New(s Spec) (Operation, error) {
	if ClassOf(s.Category) == ClassUnknown {
		return Operation{}, ErrInvalid
	}
	for _, id := range []string{s.AssessmentID, s.ToolKey, s.Kind} {
		if !validID(id) {
			return Operation{}, ErrInvalid
		}
	}
	if !validPrintable(s.Target, maxTargetLen) || s.Port == 0 {
		return Operation{}, ErrInvalid
	}
	if !s.Limits.valid() || s.ManifestDigest == ([32]byte{}) {
		return Operation{}, ErrInvalid
	}
	params, err := copyParams(s.Params)
	if err != nil {
		return Operation{}, err
	}
	class := ClassOf(s.Category)
	if m, ok := params[ParamHTTPMethod]; ok {
		safe, err := safeMethod(m)
		if err != nil {
			return Operation{}, err
		}
		if !safe {
			class = ClassIntrusive
		}
	}
	dests, err := copyDestinations(s.Destinations)
	if err != nil {
		return Operation{}, err
	}

	o := Operation{spec: s, class: class}
	o.spec.Params, o.spec.Destinations = params, dests
	o.canonical = o.encode()
	o.hash = sha256.Sum256(o.canonical)
	return o, nil
}

func validID(s string) bool {
	if len(s) == 0 || len(s) > maxIDLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if i == 0 && !alnum {
			return false
		}
		if !alnum && c != '.' && c != '_' && c != ':' && c != '-' {
			return false
		}
	}
	return true
}

// validPrintable accepts 1 to max bytes of printable ASCII without a space.
func validPrintable(s string, max int) bool {
	if len(s) == 0 || len(s) > max {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

func validParamName(s string) bool {
	if len(s) == 0 || len(s) > maxParamName || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func validParamValue(s string) bool {
	if len(s) > maxParamValue || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func copyParams(in map[string]string) (map[string]string, error) {
	if len(in) > maxParams {
		return nil, ErrInvalid
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if !validParamName(k) || !validParamValue(v) {
			return nil, ErrInvalid
		}
		out[k] = v
	}
	return out, nil
}

func copyDestinations(in []string) ([]string, error) {
	if len(in) > maxDestination {
		return nil, ErrInvalid
	}
	out := make([]string, 0, len(in))
	for _, d := range in {
		if !validPrintable(d, maxTargetLen) {
			return nil, ErrInvalid
		}
		out = append(out, d)
	}
	sort.Strings(out)
	for i := 1; i < len(out); i++ {
		if out[i] == out[i-1] {
			return nil, ErrInvalid
		}
	}
	return out, nil
}

// safeMethod classifies an HTTP method. Only GET, HEAD and OPTIONS are safe.
// CONNECT opens a tunnel and is not an HTTP request to a target, so it is
// refused. Any other well-formed method counts as state-changing.
func safeMethod(m string) (bool, error) {
	if len(m) == 0 || len(m) > maxMethodLen {
		return false, ErrInvalid
	}
	for i := 0; i < len(m); i++ {
		c := m[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false, ErrInvalid
		}
	}
	switch strings.ToUpper(m) {
	case "CONNECT":
		return false, ErrInvalid
	case "GET", "HEAD", "OPTIONS":
		return true, nil
	}
	return false, nil
}

// encode writes canonical form version 1. Every string carries its length, so
// no two field layouts can produce the same bytes. Parameters and destinations
// are sorted.
func (o Operation) encode() []byte {
	var b bytes.Buffer
	str := func(s string) {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(s)))
		b.Write(n[:])
		b.WriteString(s)
	}
	u64 := func(v uint64) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], v)
		b.Write(n[:])
	}
	b.WriteString("msop")
	b.WriteByte(1)
	str(o.spec.AssessmentID)
	str(o.spec.ToolKey)
	str(o.spec.Kind)
	b.WriteByte(byte(o.spec.Category))
	b.WriteByte(byte(o.class))
	str(o.spec.Target)
	var port [2]byte
	binary.BigEndian.PutUint16(port[:], o.spec.Port)
	b.Write(port[:])

	names := make([]string, 0, len(o.spec.Params))
	for k := range o.spec.Params {
		names = append(names, k)
	}
	sort.Strings(names)
	u64(uint64(len(names)))
	for _, k := range names {
		str(k)
		str(o.spec.Params[k])
	}
	u64(uint64(len(o.spec.Destinations)))
	for _, d := range o.spec.Destinations {
		str(d)
	}
	u64(uint64(o.spec.Limits.MaxConcurrency))
	u64(uint64(o.spec.Limits.MaxDuration))
	u64(uint64(o.spec.Limits.RequestsPerMinute))
	b.Write(o.spec.ManifestDigest[:])
	return b.Bytes()
}

// IsZero reports whether the Operation was never built by New.
func (o Operation) IsZero() bool { return o.hash.IsZero() }

// Hash returns the canonical hash. The zero Operation has the zero Hash.
func (o Operation) Hash() Hash { return o.hash }

// Canonical returns a copy of the canonical form that was hashed.
func (o Operation) Canonical() []byte { return append([]byte(nil), o.canonical...) }

// Class is the derived class: the category class, raised by a state-changing
// method. It is never lower than the category class.
func (o Operation) Class() RiskClass { return o.class }

func (o Operation) AssessmentID() string     { return o.spec.AssessmentID }
func (o Operation) ToolKey() string          { return o.spec.ToolKey }
func (o Operation) Kind() string             { return o.spec.Kind }
func (o Operation) Category() Category       { return o.spec.Category }
func (o Operation) Target() string           { return o.spec.Target }
func (o Operation) Port() uint16             { return o.spec.Port }
func (o Operation) Limits() Limits           { return o.spec.Limits }
func (o Operation) ManifestDigest() [32]byte { return o.spec.ManifestDigest }

// Param returns one parameter.
func (o Operation) Param(name string) (string, bool) {
	v, ok := o.spec.Params[name]
	return v, ok
}

// Params returns a copy of the parameters.
func (o Operation) Params() map[string]string {
	out := make(map[string]string, len(o.spec.Params))
	for k, v := range o.spec.Params {
		out[k] = v
	}
	return out
}

// Destinations returns a sorted copy of the declared destinations.
func (o Operation) Destinations() []string { return append([]string(nil), o.spec.Destinations...) }
