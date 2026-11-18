// Package response serializes HTTP responses, including the single error
// envelope of docs/design/api/error-model.md.
package response

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/request"
)

const (
	CodeHostDenied           = "host_denied"
	CodeNotFound             = "not_found"
	CodeMethodNotAllowed     = "method_not_allowed"
	CodeInternal             = "internal_error"
	CodeInvalidRequest       = "invalid_request"
	CodeUnauthenticated      = "unauthenticated"
	CodeSessionExpired       = "session_expired"
	CodeInvalidCredentials   = "invalid_credentials"
	CodeBootstrapInvalid     = "bootstrap_secret_invalid"
	CodeRecoveryInvalid      = "recovery_invalid"
	CodeCSRFInvalid          = "csrf_invalid"
	CodeOriginDenied         = "origin_denied"
	CodeReauthRequired       = "reauth_required"
	CodeReauthInvalid        = "reauth_invalid"
	CodeReauthSubjectInvalid = "reauth_subject_invalid"
	CodeConflictState        = "conflict_state"
	CodeAccountExists        = "account_exists"
	CodeSetupUnavailable     = "setup_unavailable"
	CodeRecoveryNotRequired  = "recovery_not_required"
	CodePayloadTooLarge      = "payload_too_large"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeValidationFailed     = "validation_failed"
	CodeRateLimited          = "rate_limited"
	CodeRecoveryRequired     = "recovery_required"
	CodeDatabaseUnavailable  = "database_unavailable"
	CodeEvidencePurged       = "evidence_purged"
	CodeEvidenceIntegrity    = "evidence_integrity_failed"
	CodeProgramArchived      = "program_archived"
)

var messages = map[string]string{
	CodeHostDenied:           "The Host header is not allowed.",
	CodeNotFound:             "The resource does not exist or is not visible to this session.",
	CodeMethodNotAllowed:     "The method is not allowed for this resource.",
	CodeInternal:             "An unexpected failure. Nothing internal is exposed.",
	CodeInvalidRequest:       "The request could not be parsed or a header is malformed.",
	CodeUnauthenticated:      "No valid session.",
	CodeSessionExpired:       "The session expired or was revoked.",
	CodeInvalidCredentials:   "Credentials were not accepted.",
	CodeBootstrapInvalid:     "The bootstrap secret was not accepted.",
	CodeRecoveryInvalid:      "The recovery code was not accepted.",
	CodeCSRFInvalid:          "The CSRF token is missing or wrong.",
	CodeOriginDenied:         "The Origin header is missing or not allowed.",
	CodeReauthRequired:       "A fresh password entry bound to this action is required.",
	CodeReauthInvalid:        "The re-authentication grant is expired, consumed or bound to another action.",
	CodeReauthSubjectInvalid: "The subject of the re-authentication cannot be used.",
	CodeConflictState:        "The resource is not in a state that allows this transition.",
	CodeAccountExists:        "An account already exists.",
	CodeSetupUnavailable:     "Account setup is not available.",
	CodeRecoveryNotRequired:  "Recovery is not required.",
	CodePayloadTooLarge:      "The body exceeds the route limit.",
	CodeUnsupportedMediaType: "Only application/json is accepted on this route.",
	CodeValidationFailed:     "One or more fields are invalid.",
	CodeRateLimited:          "Too many requests.",
	CodeRecoveryRequired:     "The installation needs recovery after a detected restore. Only GET /session and the reconcile route work.",
	CodeDatabaseUnavailable:  "The database is unavailable. No operation was started.",
	CodeEvidencePurged:       "The evidence content was purged.",
	CodeEvidenceIntegrity:    "The evidence hash check failed or the content is missing.",
	CodeProgramArchived:      "The program is archived and read-only.",
}

var retryable = map[string]bool{CodeRateLimited: true, CodeDatabaseUnavailable: true}

type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorBody struct {
	Code        string       `json:"code"`
	Message     string       `json:"message"`
	RequestID   string       `json:"requestId"`
	Retryable   bool         `json:"retryable"`
	FieldErrors []FieldError `json:"fieldErrors,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// Error aborts the request with the error envelope. The message comes from the
// code, never from the caller.
func Error(c *gin.Context, status int, code string) { Detailed(c, status, code, nil, 0) }

// Detailed adds field errors for validation_failed and Retry-After for a 429 or
// 503. Field messages are fixed text per field code and never echo input.
func Detailed(c *gin.Context, status int, code string, fields []FieldError, retryAfter time.Duration) {
	msg, ok := messages[code]
	if !ok {
		status, code, msg = http.StatusInternalServerError, CodeInternal, messages[CodeInternal]
	}
	if retryAfter > 0 || status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
		secs := int(retryAfter.Round(time.Second) / time.Second)
		if secs < 1 {
			secs = 5
		}
		c.Header("Retry-After", strconv.Itoa(secs))
	}
	c.AbortWithStatusJSON(status, errorEnvelope{Error: errorBody{
		Code: code, Message: msg, RequestID: request.ID(c), Retryable: retryable[code], FieldErrors: fields,
	}})
}

type statusBody struct {
	Status string `json:"status"`
}

func Status(c *gin.Context, httpStatus int, value string) {
	c.JSON(httpStatus, statusBody{Status: value})
}
