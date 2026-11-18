package identity

import (
	"crypto/sha256"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Purpose is the action class that a re-authentication grant allows. The names
// are the CHECK list of reauth_grants.purpose.
type Purpose string

const (
	PurposeApprovalIntrusive        Purpose = "approval_intrusive"
	PurposePolicyWidening           Purpose = "policy_widening"
	PurposeScopeWidening            Purpose = "scope_widening"
	PurposeLabRegistration          Purpose = "lab_registration"
	PurposeExclusionAcknowledgement Purpose = "exclusion_acknowledgement"
	PurposeRawEvidenceCapture       Purpose = "raw_evidence_capture"
	PurposeProviderEnablement       Purpose = "provider_enablement"
	PurposeToolRegistration         Purpose = "tool_registration"
	PurposePassiveSourceEnablement  Purpose = "passive_source_enablement"
	PurposeVerification             Purpose = "verification"
	PurposePasswordChange           Purpose = "password_change"
)

var purposes = map[Purpose]bool{
	PurposeApprovalIntrusive: true, PurposePolicyWidening: true, PurposeScopeWidening: true, PurposeLabRegistration: true,
	PurposeExclusionAcknowledgement: true, PurposeRawEvidenceCapture: true, PurposeProviderEnablement: true,
	PurposeToolRegistration: true, PurposePassiveSourceEnablement: true, PurposeVerification: true, PurposePasswordChange: true,
}

func ParsePurpose(s string) (Purpose, bool) {
	p := Purpose(s)
	return p, purposes[p]
}

const (
	maxSubjectKeys   = 16
	maxSubjectString = 512
	maxKeyLen        = 32
)

// SubjectHash canonicalizes the subject of a guarded action and hashes it. The
// canonical form is a flat JSON object with sorted keys and no spaces. Values are
// strings, booleans or integers. A later milestone that guards an action builds
// the same map from its own record, so the two hashes agree only when the action
// is exactly the one the researcher entered the password for. The purpose is part
// of the hash input so one subject cannot serve two purposes.
func SubjectHash(purpose Purpose, subject map[string]any) ([32]byte, error) {
	if !purposes[purpose] || len(subject) > maxSubjectKeys {
		return [32]byte{}, ErrInvalid
	}
	if purpose == PurposePasswordChange && len(subject) != 0 {
		return [32]byte{}, ErrInvalid
	}
	keys := make([]string, 0, len(subject))
	for k := range subject {
		if !validKey(k) {
			return [32]byte{}, ErrInvalid
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(string(purpose))
	b.WriteByte('\n')
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(k))
		b.WriteByte(':')
		switch v := subject[k].(type) {
		case string:
			if !utf8.ValidString(v) || len(v) > maxSubjectString || strings.ContainsRune(v, 0) {
				return [32]byte{}, ErrInvalid
			}
			b.WriteString(strconv.Quote(v))
		case bool:
			b.WriteString(strconv.FormatBool(v))
		case int:
			b.WriteString(strconv.Itoa(v))
		case int64:
			b.WriteString(strconv.FormatInt(v, 10))
		case float64:
			if v != float64(int64(v)) || v > 1<<53 || v < -(1<<53) {
				return [32]byte{}, ErrInvalid
			}
			b.WriteString(strconv.FormatInt(int64(v), 10))
		default:
			return [32]byte{}, ErrInvalid
		}
	}
	b.WriteByte('}')
	return sha256.Sum256([]byte(b.String())), nil
}

func validKey(k string) bool {
	if len(k) == 0 || len(k) > maxKeyLen || k[0] < 'a' || k[0] > 'z' {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}
