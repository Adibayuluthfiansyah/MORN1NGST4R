package evidence

import (
	"context"
	"errors"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

// ErrPurgeUnavailable: the service was built without the retention role.
var ErrPurgeUnavailable = errors.New("evidence: the purge needs the retention role")

// Purge removes the content of an item and keeps its metadata and its hash
// (D-77, data-lifecycle.md). The order makes every crash safe to resume:
//
//  1. the item moves to purge_pending, with its audit event, in one transaction;
//  2. the file is removed;
//  3. the item moves to purged, with the person who decided and its audit event,
//     in one transaction.
//
// A second call finishes what a crash left. A purge is final, and nothing but the
// retention role can write the state (migration 00017). Ownership is checked
// first, with the role of the server.
func (s *Service) Purge(ctx context.Context, p inbound.Principal, requestID, id string) error {
	if s.Retention == nil || s.RetentionTx == nil || s.RetentionAudit == nil {
		return ErrPurgeUnavailable
	}
	owner, who, rec, err := s.lookup(ctx, p, id)
	if err != nil {
		return err
	}
	it := rec.Item
	if rec.Status.Retention == evidence.RetentionPurged {
		return inbound.ErrEvidencePurged
	}
	if rec.Status.Retention == evidence.RetentionActive {
		err = s.RetentionTx.InTx(ctx, func(ctx context.Context) error {
			ok, err := s.Retention.BeginPurge(ctx, it.ID())
			if err != nil {
				return err
			}
			if !ok {
				return inbound.ErrConflictState
			}
			return s.requireWith(ctx, s.RetentionAudit, who, "evidence.purge_requested", requestID, it, map[string]any{"sha256": it.SHA256().String(), "sizeBytes": it.Size()})
		})
		if err != nil {
			return mapError(err)
		}
	}
	if err := s.Store.Remove(ctx, it.StorageKey()); err != nil {
		return err
	}
	err = s.RetentionTx.InTx(ctx, func(ctx context.Context) error {
		ok, err := s.Retention.CompletePurge(ctx, it.ID(), owner)
		if err != nil {
			return err
		}
		if !ok {
			return inbound.ErrConflictState
		}
		return s.requireWith(ctx, s.RetentionAudit, who, "evidence.purged", requestID, it, map[string]any{"sha256": it.SHA256().String(), "sizeBytes": it.Size()})
	})
	return mapError(err)
}
