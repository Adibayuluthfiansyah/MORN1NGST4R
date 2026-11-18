// Package identity holds the pure rules of the local account: names, password
// policy, recovery codes, re-authentication purposes and the session and
// attempt limits (D-60, D-68, D-69). It reads no clock and touches no storage.
package identity

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid identity input")

const (
	minUsername = 3
	maxUsername = 64
	MinPassword = 12
	MaxPassword = 128
)

func NormalizeUsername(s string) (string, error) {
	s = strings.ToLower(s)
	if len(s) < minUsername || len(s) > maxUsername {
		return "", ErrInvalid
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		alnum := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if i == 0 && !alnum {
			return "", ErrInvalid
		}
		if !alnum && c != '.' && c != '_' && c != '-' {
			return "", ErrInvalid
		}
	}
	return s, nil
}

// ValidateNewPassword applies length rules only, counted in characters. There
// are no composition rules and no normalization: the bytes the user typed are
// the bytes that are hashed.
func ValidateNewPassword(username, password string) error {
	if !utf8.ValidString(password) {
		return ErrInvalid
	}
	n := utf8.RuneCountInString(password)
	if n < MinPassword || n > MaxPassword {
		return ErrInvalid
	}
	for _, r := range password {
		if unicode.IsControl(r) {
			return ErrInvalid
		}
	}
	if strings.EqualFold(password, username) {
		return ErrInvalid
	}
	return nil
}

// ValidatePasswordInput bounds a password that is only checked, never stored, so
// that an oversized value never reaches the hash function.
func ValidatePasswordInput(password string) error {
	if !utf8.ValidString(password) || len(password) == 0 || len(password) > 4*MaxPassword {
		return ErrInvalid
	}
	return nil
}
