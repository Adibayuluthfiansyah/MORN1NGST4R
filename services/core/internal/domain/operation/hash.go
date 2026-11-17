package operation

import "encoding/hex"

// Hash is the SHA-256 of the canonical form of an operation. The zero Hash
// belongs to no operation, so a zero value cannot match anything.
type Hash [32]byte

// String returns 64 lower-case hexadecimal characters.
func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// IsZero reports whether the hash is all zero bytes.
func (h Hash) IsZero() bool { return h == Hash{} }

// ParseHash reads the text form. It accepts exactly 64 lower-case hexadecimal
// characters.
func ParseHash(s string) (Hash, error) {
	if len(s) != 64 {
		return Hash{}, ErrInvalid
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return Hash{}, ErrInvalid
		}
	}
	var h Hash
	if _, err := hex.Decode(h[:], []byte(s)); err != nil {
		return Hash{}, ErrInvalid
	}
	return h, nil
}
