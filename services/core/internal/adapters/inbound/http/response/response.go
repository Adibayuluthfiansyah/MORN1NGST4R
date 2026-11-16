// Package response serializes HTTP responses, including the single error
// envelope of docs/design/api/error-model.md.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/request"
)

// Error codes used by this adapter. Messages are fixed text per code. No user
// input is echoed.
const (
	CodeHostDenied       = "host_denied"
	CodeNotFound         = "not_found"
	CodeMethodNotAllowed = "method_not_allowed"
	CodeInternal         = "internal_error"
)

var messages = map[string]string{
	CodeHostDenied:       "The Host header is not allowed.",
	CodeNotFound:         "The resource does not exist or is not visible to this session.",
	CodeMethodNotAllowed: "The method is not allowed for this resource.",
	CodeInternal:         "An unexpected failure. Nothing internal is exposed.",
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	Retryable bool   `json:"retryable"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// Error aborts the request with the error envelope. The message comes from the
// code, never from the caller.
func Error(c *gin.Context, status int, code string) {
	msg, ok := messages[code]
	if !ok {
		// A code without a fixed message is a programming error. Do not
		// invent text.
		status, code, msg = http.StatusInternalServerError, CodeInternal, messages[CodeInternal]
	}
	c.AbortWithStatusJSON(status, errorEnvelope{Error: errorBody{
		Code:      code,
		Message:   msg,
		RequestID: request.ID(c),
	}})
}

type statusBody struct {
	Status string `json:"status"`
}

// Status writes {"status": value} with the given HTTP status.
func Status(c *gin.Context, httpStatus int, value string) {
	c.JSON(httpStatus, statusBody{Status: value})
}
