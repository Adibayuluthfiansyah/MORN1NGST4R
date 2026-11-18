// Package evidence is the evidence model (FR-EVIDENCE-001 and 002, D-10, D-77,
// P-37). An Item is the immutable metadata of one stored artifact. The package
// holds no file and no clock: the store keeps the bytes, and the database keeps
// the metadata. What it decides is what may be stored (redaction), what a file
// check means (Check, NextState) and what may serve as proof.
package evidence

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// ErrInvalid is the category of every refusal. The text names the field and never
// the value.
var ErrInvalid = errors.New("invalid evidence")

type invalidError struct{ field string }

func (e *invalidError) Error() string        { return "invalid evidence: " + e.field }
func (e *invalidError) Is(target error) bool { return target == ErrInvalid }

func invalid(field string) error { return &invalidError{field} }

// StoreBackend is the only store of the MVP (D-62, D-77).
const StoreBackend = "local_fs"

type Kind string

const (
	KindRequest         Kind = "request"
	KindResponse        Kind = "response"
	KindRequestResponse Kind = "request_response"
	KindToolOutput      Kind = "tool_output"
	KindNote            Kind = "note"
	KindReport          Kind = "report"
)

type Origin string

const (
	Observed     Origin = "observed"
	AIInferred   Origin = "ai_inferred"
	HumanEntered Origin = "human_entered"
)

// Redaction says what the stored bytes are. Redacted is the evidence of record
// (D-77). RawRetained is a capture that a human policy setting allowed.
type Redaction string

const (
	Redacted    Redaction = "redacted"
	RawRetained Redaction = "raw_retained"
)

type Sensitivity string

const (
	Normal    Sensitivity = "normal"
	Sensitive Sensitivity = "sensitive"
)

var (
	uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	toolPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)
)

func isUUID(s string) bool { return uuidPattern.MatchString(s) }

func validKind(k Kind) bool {
	switch k {
	case KindRequest, KindResponse, KindRequestResponse, KindToolOutput, KindNote, KindReport:
		return true
	}
	return false
}

// Digest is a SHA-256.
type Digest [32]byte

func DigestOf(b []byte) Digest { return sha256.Sum256(b) }

// DigestFromBytes copies the first 32 bytes of b. A shorter slice gives a digest
// that no content has.
func DigestFromBytes(b []byte) Digest {
	var d Digest
	copy(d[:], b)
	return d
}

func ParseDigest(s string) (Digest, error) {
	var d Digest
	if len(s) != 64 || s != strings.ToLower(s) {
		return d, invalid("sha256")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return d, invalid("sha256")
	}
	copy(d[:], b)
	return d, nil
}

func (d Digest) String() string { return hex.EncodeToString(d[:]) }

// Bytes returns a copy.
func (d Digest) Bytes() []byte { return append([]byte(nil), d[:]...) }

// Equal compares in constant time.
func (d Digest) Equal(o Digest) bool { return subtle.ConstantTimeCompare(d[:], o[:]) == 1 }

const (
	maxContentType = 255
	maxArgs        = 64
	maxArgLen      = 1024
)

// NewItem is what a caller supplies. Sensitivity defaults to Sensitive.
type NewItem struct {
	ID, ProgramID, AssessmentID, JobID, AttemptID, TargetID, DecisionID string
	Kind                                                                Kind
	ContentType                                                         string
	Origin                                                              Origin
	Redaction                                                           Redaction
	Sensitivity                                                         Sensitivity
	SHA256                                                              Digest
	Size                                                                int64
	CapturedAt                                                          time.Time
	ToolKey, ToolVersion                                                string
	ToolBinarySHA256, OperationHash                                     []byte
	RedactedArgs                                                        []string
}

// Item is immutable metadata. Its accessors return copies.
type Item struct {
	id, programID, assessmentID, jobID, attemptID, targetID, decisionID string
	kind                                                                Kind
	contentType                                                         string
	origin                                                              Origin
	redaction                                                           Redaction
	sensitivity                                                         Sensitivity
	sha256                                                              Digest
	size                                                                int64
	capturedAt                                                          time.Time
	toolKey, toolVersion                                                string
	toolBinarySHA256, operationHash                                     []byte
	redactedArgs                                                        []string
}

// New validates in. Arguments are redacted again here as a second line of
// defense: the tool adapter redacts them first.
func New(in NewItem) (Item, error) {
	for field, id := range map[string]string{"id": in.ID, "programId": in.ProgramID, "assessmentId": in.AssessmentID} {
		if !isUUID(id) {
			return Item{}, invalid(field)
		}
	}
	for field, id := range map[string]string{"jobId": in.JobID, "attemptId": in.AttemptID, "targetId": in.TargetID, "decisionId": in.DecisionID} {
		if id != "" && !isUUID(id) {
			return Item{}, invalid(field)
		}
	}
	if !validKind(in.Kind) {
		return Item{}, invalid("kind")
	}
	switch in.Origin {
	case Observed, AIInferred, HumanEntered:
	default:
		return Item{}, invalid("origin")
	}
	switch in.Redaction {
	case Redacted, RawRetained:
	default:
		return Item{}, invalid("redaction")
	}
	sens := in.Sensitivity
	switch sens {
	case "":
		sens = Sensitive
	case Normal, Sensitive:
	default:
		return Item{}, invalid("sensitivity")
	}
	if in.Size < 0 {
		return Item{}, invalid("size")
	}
	if in.CapturedAt.IsZero() {
		return Item{}, invalid("capturedAt")
	}
	if n := len(in.ContentType); n == 0 || n > maxContentType {
		return Item{}, invalid("contentType")
	}
	for _, r := range in.ContentType {
		if r < 0x20 || r > 0x7e {
			return Item{}, invalid("contentType")
		}
	}
	for field, v := range map[string]string{"toolKey": in.ToolKey, "toolVersion": in.ToolVersion} {
		if v != "" && !toolPattern.MatchString(v) {
			return Item{}, invalid(field)
		}
	}
	for field, h := range map[string][]byte{"toolBinarySha256": in.ToolBinarySHA256, "operationHash": in.OperationHash} {
		if h != nil && len(h) != 32 {
			return Item{}, invalid(field)
		}
	}
	if len(in.RedactedArgs) > maxArgs {
		return Item{}, invalid("redactedArgs")
	}
	args := make([]string, len(in.RedactedArgs))
	for i, a := range in.RedactedArgs {
		if len(a) > maxArgLen {
			return Item{}, invalid("redactedArgs")
		}
		red, _ := Redact([]byte(clean(a)))
		args[i] = string(red)
	}
	return Item{
		id: in.ID, programID: in.ProgramID, assessmentID: in.AssessmentID, jobID: in.JobID, attemptID: in.AttemptID,
		targetID: in.TargetID, decisionID: in.DecisionID, kind: in.Kind, contentType: in.ContentType, origin: in.Origin,
		redaction: in.Redaction, sensitivity: sens, sha256: in.SHA256, size: in.Size, capturedAt: in.CapturedAt,
		toolKey: in.ToolKey, toolVersion: in.ToolVersion,
		toolBinarySHA256: append([]byte(nil), in.ToolBinarySHA256...), operationHash: append([]byte(nil), in.OperationHash...),
		redactedArgs: args,
	}, nil
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

func (i Item) ID() string               { return i.id }
func (i Item) ProgramID() string        { return i.programID }
func (i Item) AssessmentID() string     { return i.assessmentID }
func (i Item) JobID() string            { return i.jobID }
func (i Item) AttemptID() string        { return i.attemptID }
func (i Item) TargetID() string         { return i.targetID }
func (i Item) DecisionID() string       { return i.decisionID }
func (i Item) Kind() Kind               { return i.kind }
func (i Item) ContentType() string      { return i.contentType }
func (i Item) Origin() Origin           { return i.origin }
func (i Item) Redaction() Redaction     { return i.redaction }
func (i Item) Sensitivity() Sensitivity { return i.sensitivity }
func (i Item) SHA256() Digest           { return i.sha256 }
func (i Item) Size() int64              { return i.size }
func (i Item) CapturedAt() time.Time    { return i.capturedAt }
func (i Item) ToolKey() string          { return i.toolKey }
func (i Item) ToolVersion() string      { return i.toolVersion }
func (i Item) ToolBinarySHA256() []byte { return append([]byte(nil), i.toolBinarySHA256...) }
func (i Item) OperationHash() []byte    { return append([]byte(nil), i.operationHash...) }
func (i Item) RedactedArgs() []string   { return append([]string(nil), i.redactedArgs...) }

// StorageKey is the key of the file in the store. It is the evidence id and
// never a name or a path from a caller (D-77).
func (i Item) StorageKey() string { return i.id }

// Observation is what the store found when it looked at the file of an item.
type Observation struct {
	Found  bool
	Safe   bool // a regular file, owned by the user, closed to group and others, with one link
	Size   int64
	Digest Digest
}

type CheckResult string

const (
	ResultMatch                CheckResult = "match"
	ResultHashMismatch         CheckResult = "hash_mismatch"
	ResultSizeMismatch         CheckResult = "size_mismatch"
	ResultMissing              CheckResult = "missing"
	ResultMetadataInconsistent CheckResult = "metadata_inconsistent"
)

type CheckTrigger string

const (
	TriggerRead            CheckTrigger = "read"
	TriggerManual          CheckTrigger = "manual"
	TriggerSweep           CheckTrigger = "sweep"
	TriggerPreVerification CheckTrigger = "pre_verification"
)

// Check compares an observation with the item. A file that is not a plain,
// owner-only file is treated as tampered with, whatever its bytes are. The size
// is compared before the hash.
func Check(it Item, o Observation) CheckResult {
	switch {
	case !o.Found:
		return ResultMissing
	case !o.Safe:
		return ResultMetadataInconsistent
	case o.Size != it.size:
		return ResultSizeMismatch
	case !o.Digest.Equal(it.sha256):
		return ResultHashMismatch
	}
	return ResultMatch
}

type IntegrityState string

const (
	Unchecked   IntegrityState = "unchecked"
	IntegrityOK IntegrityState = "ok"
	Compromised IntegrityState = "compromised"
	Missing     IntegrityState = "missing"
)

// NextState is the state after a check (domain-model.md section 3: unchecked to
// ok, any to compromised, any to missing). A compromised item stays compromised
// whatever a later check finds, and a missing one does not return to ok: only a
// person can decide about them (P-37).
func NextState(from IntegrityState, r CheckResult) IntegrityState {
	switch {
	case from == Compromised:
		return Compromised
	case r == ResultMatch && from == Missing:
		return Missing
	case r == ResultMatch:
		return IntegrityOK
	case r == ResultMissing:
		return Missing
	}
	return Compromised
}

// SupportsVerification says whether an item may serve as proof for a Verified
// transition (FR-EVIDENCE-002 AC3). Only an item whose last check matched can.
func (s IntegrityState) SupportsVerification() bool { return s == IntegrityOK }
