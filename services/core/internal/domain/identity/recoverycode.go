package identity

import "strings"

const (
	recoveryCodeBytes = 20
	recoveryCodeChars = 32
	base32Alphabet    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
)

// RecoveryCodeBytes is the entropy of a recovery code: 160 bits, above the 128
// bits that D-68 requires.
const RecoveryCodeBytes = recoveryCodeBytes

// FormatRecoveryCode renders 20 random bytes as eight groups of four base32
// characters. The canonical form that is hashed has no separators.
func FormatRecoveryCode(random []byte) (shown, canonical string, err error) {
	if len(random) != recoveryCodeBytes {
		return "", "", ErrInvalid
	}
	var b strings.Builder
	var buf, bits uint
	for _, x := range random {
		buf = buf<<8 | uint(x)
		bits += 8
		for bits >= 5 {
			bits -= 5
			b.WriteByte(base32Alphabet[(buf>>bits)&31])
		}
	}
	canonical = b.String()
	var g strings.Builder
	for i := 0; i < len(canonical); i += 4 {
		if i > 0 {
			g.WriteByte('-')
		}
		g.WriteString(canonical[i : i+4])
	}
	return g.String(), canonical, nil
}

// CanonicalRecoveryCode accepts the shown form in any case with hyphens and
// spaces and returns the canonical form, or false.
func CanonicalRecoveryCode(input string) (string, bool) {
	if len(input) > 2*recoveryCodeChars {
		return "", false
	}
	var b strings.Builder
	for _, r := range strings.ToUpper(input) {
		switch {
		case r == '-' || r == ' ':
		case r >= 'A' && r <= 'Z' || r >= '2' && r <= '7':
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	if b.Len() != recoveryCodeChars {
		return "", false
	}
	return b.String(), true
}
