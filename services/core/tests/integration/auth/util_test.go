package auth_test

import (
	"encoding/base64"
	"testing"
)

func decode(t *testing.T, token string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token %q: %v", token, err)
	}
	return b
}
