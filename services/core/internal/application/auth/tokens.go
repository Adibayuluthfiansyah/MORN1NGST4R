package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
)

const (
	tokenBytes     = 32
	attemptWindow  = identity.AttemptWindow
	anonPayloadLen = 16 + 8
)

var enc = base64.RawURLEncoding

func newToken(random func(int) []byte) (token string, raw []byte) {
	raw = random(tokenBytes)
	return enc.EncodeToString(raw), raw
}

func decodeToken(s string) ([]byte, bool) {
	if len(s) != enc.EncodedLen(tokenBytes) {
		return nil, false
	}
	b, err := enc.DecodeString(s)
	if err != nil || len(b) != tokenBytes {
		return nil, false
	}
	return b, true
}

func tokenHash(raw []byte) []byte {
	h := sha256.Sum256(raw)
	return h[:]
}

// csrfFor derives the session CSRF token from the session token. The token is
// therefore never stored and can be returned again by GET /session, while a
// script that cannot read the HttpOnly cookie cannot compute it. Only its
// SHA-256 is stored, and requests are checked against that hash.
func csrfFor(sessionRaw []byte) (token string, hash []byte) {
	m := hmac.New(sha256.New, sessionRaw)
	m.Write([]byte("morningstar csrf v1"))
	raw := m.Sum(nil)
	return enc.EncodeToString(raw), tokenHash(raw)
}

func csrfMatches(presented string, storedHash []byte) bool {
	raw, ok := decodeToken(presented)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare(tokenHash(raw), storedHash) == 1
}

// The pre-session token (ms_anon) is signed and expires after 15 minutes. It
// carries no user data. Its CSRF token is derived from it with the same key.
func (s *Service) newAnonToken() string {
	payload := make([]byte, anonPayloadLen)
	copy(payload, s.Random(16))
	binary.BigEndian.PutUint64(payload[16:], uint64(s.Now().Add(identity.AnonTokenTTL).Unix()))
	return enc.EncodeToString(payload) + "." + enc.EncodeToString(s.mac("anon", payload))
}

func (s *Service) mac(label string, payload []byte) []byte {
	m := hmac.New(sha256.New, s.anonKey)
	m.Write([]byte(label))
	m.Write([]byte{0})
	m.Write(payload)
	return m.Sum(nil)
}

func (s *Service) parseAnon(token string) ([]byte, bool) {
	if len(token) > 128 {
		return nil, false
	}
	var p, m string
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			p, m = token[:i], token[i+1:]
			break
		}
	}
	payload, err1 := enc.DecodeString(p)
	sig, err2 := enc.DecodeString(m)
	if err1 != nil || err2 != nil || len(payload) != anonPayloadLen {
		return nil, false
	}
	if !hmac.Equal(sig, s.mac("anon", payload)) {
		return nil, false
	}
	if s.Now().Unix() >= int64(binary.BigEndian.Uint64(payload[16:])) {
		return nil, false
	}
	return payload, true
}

func (s *Service) anonCSRF(payload []byte) string {
	return enc.EncodeToString(s.mac("anon-csrf", payload))
}

func (s *Service) ValidAnonCSRF(anonToken, presented string) bool {
	payload, ok := s.parseAnon(anonToken)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.anonCSRF(payload))) == 1
}
