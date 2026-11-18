package outbound

import (
	"context"
	"errors"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
)

var (
	// ErrEvidenceExists: the store already holds a file under the key. The store
	// never replaces a file (P-37).
	ErrEvidenceExists = errors.New("an evidence file with this key exists")
	// ErrEvidenceTooLarge: the content is larger than the limit of the store.
	ErrEvidenceTooLarge = errors.New("the evidence is larger than the limit")
	// ErrEvidenceNotFound: no item with this id is visible to the user. It does not say whether another user's item exists.
	ErrEvidenceNotFound = errors.New("the evidence does not exist or is not visible")
	// ErrEvidencePurged: the content of the item was purged.
	ErrEvidencePurged = errors.New("the evidence content was purged")
)

// EvidenceWritten is what the store measured while it wrote the file.
type EvidenceWritten struct {
	Size   int64
	Digest evidence.Digest
}

// EvidenceStore keeps the bytes of evidence (D-77, P-37). A key is the id of an
// item, never a name or a path from a caller. A store refuses a key that is not
// a lower-case UUID.
//
// Put publishes a new file and never replaces one. Observe and Read describe
// the file as it is now: they never trust the metadata. A missing file is a
// finding (Observation.Found false) and not an error. An error means that the
// store could not look, and the caller must not treat it as a match.
type EvidenceStore interface {
	// MaxBytes is the largest content that Put accepts and Read returns.
	MaxBytes() int64
	Put(ctx context.Context, key string, content []byte) (EvidenceWritten, error)
	// Observe hashes the file without keeping its bytes.
	Observe(ctx context.Context, key string) (evidence.Observation, error)
	// Read returns the observation and the bytes that were hashed. The bytes are
	// nil when the file is missing or unsafe, or when it is larger than limit
	// (the observation then carries the size but no hash, and Check reports a
	// size mismatch). limit above MaxBytes is ErrEvidenceTooLarge.
	Read(ctx context.Context, key string, limit int64) (evidence.Observation, []byte, error)
	// Remove deletes the file. A missing file is not an error. It is for the
	// orphan of a capture whose metadata was not stored.
	Remove(ctx context.Context, key string) error
}

// EvidenceCursor is the position after the last row of a page.
type EvidenceCursor struct {
	At time.Time
	ID string
}

// EvidencePageQuery asks for the rows after the cursor. The repository returns
// at most Limit rows. The caller passes one more than it will show, to learn
// whether another page exists.
type EvidencePageQuery struct {
	Limit      int
	After      *EvidenceCursor
	Descending bool
}

// EvidenceFilter holds the exact-match filters of the list. A zero field does not filter.
type EvidenceFilter struct {
	Kind      evidence.Kind
	Integrity evidence.IntegrityState
	JobID     string
}

// EvidenceAssessment is what capture needs to know about an assessment. The
// program is the one of the assessment, never one that a caller names.
type EvidenceAssessment struct {
	ProgramID       string
	RawCapture      bool
	ProgramArchived bool
}

// EvidenceRepository persists the metadata of evidence. Every read that serves a
// user takes the owner and returns ErrEvidenceNotFound for a row of another
// owner, so a guessed id reveals nothing (BOLA). Inside Transactor.InTx every
// call joins the transaction of the context.
type EvidenceRepository interface {
	// Assessment returns the capture facts of an assessment, or ErrEvidenceNotFound.
	Assessment(ctx context.Context, assessmentID string) (EvidenceAssessment, error)
	// Insert stores the item and its initial status (unchecked, active).
	Insert(ctx context.Context, it evidence.Item) error
	Get(ctx context.Context, ownerID, id string) (evidence.Record, error)
	// List returns ErrEvidenceNotFound when the assessment is not the owner's.
	List(ctx context.Context, ownerID, assessmentID string, f EvidenceFilter, q EvidencePageQuery) ([]evidence.Record, error)
	// RecordCheck appends the check and moves the integrity state with
	// evidence.NextState, in the transaction of the context, holding a row lock on
	// the status. It returns ErrEvidencePurged for a purged item. It returns the
	// stored check and the status after it.
	RecordCheck(ctx context.Context, c evidence.CheckRecord) (evidence.CheckRecord, evidence.Status, error)
	// ListChecks returns ErrEvidenceNotFound when the item is not the owner's.
	ListChecks(ctx context.Context, ownerID, evidenceID string, q EvidencePageQuery) ([]evidence.CheckRecord, error)
}
