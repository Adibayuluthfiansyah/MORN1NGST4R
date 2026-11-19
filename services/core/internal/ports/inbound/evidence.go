package inbound

import (
	"context"
	"errors"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
)

var (
	// ErrEvidencePurged: the content was purged (HTTP 410).
	ErrEvidencePurged = errors.New("the evidence content was purged")
	// ErrEvidenceIntegrityFailed: the content is withheld because its integrity
	// state is compromised or missing, or the check just failed (HTTP 409).
	ErrEvidenceIntegrityFailed = errors.New("the evidence failed its integrity check")
	// ErrProgramArchived: the program is archived and read-only (HTTP 409).
	ErrProgramArchived = errors.New("the program is archived")
	// ErrRawCaptureNotAllowed: raw capture was asked for and the assessment does not allow it (D-77).
	ErrRawCaptureNotAllowed = errors.New("raw capture is not allowed for this assessment")
)

// PageInput is the paging of a list. Cursor is the opaque value of a previous
// page. A malformed cursor is a ValidationError on the field "cursor".
type PageInput struct {
	Limit      int
	Cursor     string
	Descending bool
}

// PageInfo is the page object of a list response.
type PageInfo struct {
	Limit      int
	NextCursor string
	HasMore    bool
}

type EvidenceListInput struct {
	AssessmentID   string
	Kind           string
	IntegrityState string
	JobID          string
	Page           PageInput
}

type EvidencePage struct {
	Items []evidence.Record
	Page  PageInfo
}

type EvidenceChecksPage struct {
	Items []evidence.CheckRecord
	Page  PageInfo
}

// EvidenceContent is the verified content of an item. The bytes are held in
// memory, hashed, before the first one is returned.
type EvidenceContent struct {
	ID   string
	Data []byte
}

// Evidence is the evidence use case for a signed-in user (EVD-01 to EVD-05).
// Every method resolves the item through the owner of its program, and an item
// of another owner is ErrNotFound.
type Evidence interface {
	List(ctx context.Context, p Principal, in EvidenceListInput) (EvidencePage, error)
	Get(ctx context.Context, p Principal, id string) (evidence.Record, error)
	// Check re-hashes the stored content, records the result and moves the
	// integrity state. ErrEvidencePurged, ErrProgramArchived.
	Check(ctx context.Context, p Principal, requestID, id string) (evidence.CheckRecord, error)
	Checks(ctx context.Context, p Principal, id string, page PageInput) (EvidenceChecksPage, error)
	// Open returns the content after a fresh check. It returns
	// ErrEvidenceIntegrityFailed unless the content is intact or the human
	// acknowledged the failure, and ErrEvidencePurged for purged content.
	Open(ctx context.Context, p Principal, requestID, id string, acknowledge bool) (EvidenceContent, error)
}

// CaptureInput is one artifact to store. The program is never an input: it is
// the program of the assessment.
type CaptureInput struct {
	AssessmentID string
	JobID        string
	AttemptID    string
	TargetID     string
	DecisionID   string

	Kind        evidence.Kind
	ContentType string
	Origin      evidence.Origin
	Sensitivity evidence.Sensitivity
	Content     []byte
	// Raw asks to keep the bytes unredacted. It works only for an assessment
	// whose raw capture setting a human turned on (D-77).
	Raw bool

	ToolKey, ToolVersion string
	ToolBinarySHA256     []byte
	OperationHash        []byte
	RedactedArgs         []string

	Actor     audit.Actor
	RequestID string
}

// EvidenceCapture stores new evidence. Other use cases call it. It is not an HTTP route.
type EvidenceCapture interface {
	Capture(ctx context.Context, in CaptureInput) (evidence.Item, error)
}

// EvidencePurge removes the content of an item and keeps its metadata and hash
// (D-77). It is a human action, run with the retention role. No route offers it
// in the MVP (domain-model.md section 3): a command or a screen of a later
// milestone calls it.
type EvidencePurge interface {
	Purge(ctx context.Context, p Principal, requestID, id string) error
}
