package router

import (
	"context"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

// stubEvidence counts its calls in the counter of stubAuth, so the guard tests
// of the route table see a handler that was reached.
type stubEvidence struct{ auth *stubAuth }

func (s *stubEvidence) List(context.Context, inbound.Principal, inbound.EvidenceListInput) (inbound.EvidencePage, error) {
	return inbound.EvidencePage{}, s.auth.hit()
}
func (s *stubEvidence) Get(context.Context, inbound.Principal, string) (evidence.Record, error) {
	return evidence.Record{}, s.auth.hit()
}
func (s *stubEvidence) Check(context.Context, inbound.Principal, string, string) (evidence.CheckRecord, error) {
	return evidence.CheckRecord{}, s.auth.hit()
}
func (s *stubEvidence) Checks(context.Context, inbound.Principal, string, inbound.PageInput) (inbound.EvidenceChecksPage, error) {
	return inbound.EvidenceChecksPage{}, s.auth.hit()
}
func (s *stubEvidence) Open(context.Context, inbound.Principal, string, string, bool) (inbound.EvidenceContent, error) {
	return inbound.EvidenceContent{}, s.auth.hit()
}
