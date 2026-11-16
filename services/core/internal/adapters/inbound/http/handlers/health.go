// Package handlers holds the thin Gin handlers of the inbound HTTP adapter.
// A handler parses transport input, calls a use case and maps the result.
package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/response"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

// retryAfterSeconds is sent with a 503 readiness answer.
const retryAfterSeconds = 5

// Health serves the liveness and readiness probes.
type Health struct {
	uc inbound.Health
}

// NewHealth returns the probe handlers backed by uc.
func NewHealth(uc inbound.Health) *Health { return &Health{uc: uc} }

// Live answers GET /api/v1/health/live.
func (h *Health) Live(c *gin.Context) {
	if !h.uc.Live() {
		response.Status(c, http.StatusServiceUnavailable, "not_live")
		return
	}
	response.Status(c, http.StatusOK, "ok")
}

// Ready answers GET /api/v1/health/ready.
func (h *Health) Ready(c *gin.Context) {
	if !h.uc.Ready(c.Request.Context()) {
		c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
		response.Status(c, http.StatusServiceUnavailable, "not_ready")
		return
	}
	response.Status(c, http.StatusOK, "ready")
}
