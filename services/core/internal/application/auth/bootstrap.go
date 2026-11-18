package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

const secretHashPrefix = "sha256:"

func hashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return secretHashPrefix + hex.EncodeToString(h[:])
}

func secretMatches(secret, stored string) bool {
	return subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(stored)) == 1
}

// PrepareBootstrap runs after the recovery check passed. While no account
// exists it creates a new 256-bit secret, supersedes older ones, stores only the
// hash and writes the secret to the memory-backed file. When that location is
// unavailable no live secret remains, so setup is refused (D-68). When an
// account exists it removes any leftover secret and file.
func (s *Service) PrepareBootstrap(ctx context.Context) error {
	n, err := s.Accounts.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return s.retireBootstrap(ctx)
	}
	secret, raw := newToken(s.Random)
	_ = raw
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if _, err := s.Bootstraps.Supersede(ctx); err != nil {
			return err
		}
		return s.Bootstraps.Insert(ctx, s.NewID(), hashSecret(secret))
	})
	if err != nil {
		return err
	}
	if err := s.SecretFile.Write(secret); err != nil {
		s.Logger.Warn("auth: the bootstrap secret cannot be written to a memory-backed file, so account setup is unavailable")
		_, serr := s.Bootstraps.Supersede(ctx)
		return serr
	}
	s.Logger.Info("auth: a bootstrap secret was written to the runtime directory")
	return nil
}

func (s *Service) retireBootstrap(ctx context.Context) error {
	if _, err := s.Bootstraps.Supersede(ctx); err != nil {
		return err
	}
	return s.SecretFile.Remove()
}

// Close deletes the bootstrap secret file when the server stops.
func (s *Service) Close() { _ = s.SecretFile.Remove() }

func validSecretShape(s string) bool {
	return len(s) > 0 && len(s) <= 128 && !strings.ContainsAny(s, "\x00\r\n")
}

var _ inbound.Auth = (*Service)(nil)
