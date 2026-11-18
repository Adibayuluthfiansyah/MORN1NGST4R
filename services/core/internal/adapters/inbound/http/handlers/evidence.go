package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/middleware"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/response"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

// Evidence serves EVD-01 to EVD-05.
type Evidence struct{ uc inbound.Evidence }

func NewEvidence(uc inbound.Evidence) *Evidence { return &Evidence{uc: uc} }

type evidenceJSON struct {
	ID             string     `json:"id"`
	AssessmentID   string     `json:"assessmentId"`
	JobID          *string    `json:"jobId"`
	Kind           string     `json:"kind"`
	SHA256         string     `json:"sha256"`
	SizeBytes      int64      `json:"sizeBytes"`
	ContentType    string     `json:"contentType"`
	RedactionState string     `json:"redactionState"`
	Sensitivity    string     `json:"sensitivity"`
	CapturedAt     time.Time  `json:"capturedAt"`
	Origin         string     `json:"origin"`
	ToolKey        *string    `json:"toolKey"`
	ToolVersion    *string    `json:"toolVersion"`
	IntegrityState string     `json:"integrityState"`
	RetentionState string     `json:"retentionState"`
	LastCheckedAt  *time.Time `json:"lastCheckedAt"`
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func evidenceBody(r evidence.Record) evidenceJSON {
	it := r.Item
	return evidenceJSON{
		ID: it.ID(), AssessmentID: it.AssessmentID(), JobID: strPtr(it.JobID()), Kind: string(it.Kind()), SHA256: it.SHA256().String(),
		SizeBytes: it.Size(), ContentType: it.ContentType(), RedactionState: string(it.Redaction()), Sensitivity: string(it.Sensitivity()),
		CapturedAt: it.CapturedAt().UTC(), Origin: string(it.Origin()), ToolKey: strPtr(it.ToolKey()), ToolVersion: strPtr(it.ToolVersion()),
		IntegrityState: string(r.Status.Integrity), RetentionState: string(r.Status.Retention), LastCheckedAt: r.Status.LastCheckedAt,
	}
}

type checkJSON struct {
	ID         string    `json:"id"`
	EvidenceID string    `json:"evidenceId"`
	CheckedAt  time.Time `json:"checkedAt"`
	Trigger    string    `json:"trigger"`
	Result     string    `json:"result"`
	ActorKind  string    `json:"actorKind"`
}

func checkBody(c evidence.CheckRecord) checkJSON {
	return checkJSON{ID: c.ID, EvidenceID: c.EvidenceID, CheckedAt: c.CheckedAt.UTC(), Trigger: string(c.Trigger), Result: string(c.Result), ActorKind: string(c.Actor.Kind)}
}

func pageBody(p inbound.PageInfo) gin.H {
	var next any
	if p.NextCursor != "" {
		next = p.NextCursor
	}
	return gin.H{"limit": p.Limit, "nextCursor": next, "hasMore": p.HasMore}
}

func invalidQuery(c *gin.Context, field, code string) {
	response.Detailed(c, http.StatusUnprocessableEntity, response.CodeValidationFailed, []response.FieldError{{Field: field, Code: code, Message: "The field is invalid."}}, 0)
}

// onlyQuery refuses a parameter that the route does not define, and a parameter
// that is given twice, so no filter is silently ignored (API-07).
func onlyQuery(c *gin.Context, allowed ...string) bool {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	for k, v := range c.Request.URL.Query() {
		if !ok[k] || len(v) != 1 {
			response.Error(c, http.StatusBadRequest, response.CodeInvalidRequest)
			return false
		}
	}
	return true
}

// pageInput reads limit, cursor and sort. The default order is newest first.
func pageInput(c *gin.Context) (inbound.PageInput, bool) {
	in := inbound.PageInput{Cursor: c.Query("cursor"), Descending: true}
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			invalidQuery(c, "limit", "out_of_range")
			return in, false
		}
		in.Limit = n
	}
	switch c.Query("sort") {
	case "", "-createdAt":
	case "createdAt":
		in.Descending = false
	default:
		invalidQuery(c, "sort", "invalid")
		return in, false
	}
	return in, true
}

// List answers EVD-01.
func (h *Evidence) List(c *gin.Context) {
	if !onlyQuery(c, "limit", "cursor", "sort", "kind", "integrityState", "jobId") {
		return
	}
	page, ok := pageInput(c)
	if !ok {
		return
	}
	res, err := h.uc.List(c.Request.Context(), middleware.PrincipalOf(c), inbound.EvidenceListInput{
		AssessmentID: c.Param("assessmentId"), Kind: c.Query("kind"), IntegrityState: c.Query("integrityState"), JobID: c.Query("jobId"), Page: page,
	})
	if err != nil {
		fail(c, err)
		return
	}
	items := make([]evidenceJSON, 0, len(res.Items))
	for _, r := range res.Items {
		items = append(items, evidenceBody(r))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": pageBody(res.Page)})
}

// Get answers EVD-02.
func (h *Evidence) Get(c *gin.Context) {
	if !onlyQuery(c) {
		return
	}
	r, err := h.uc.Get(c.Request.Context(), middleware.PrincipalOf(c), c.Param("evidenceId"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, evidenceBody(r))
}

// Check answers EVD-03.
func (h *Evidence) Check(c *gin.Context) {
	if !onlyQuery(c) {
		return
	}
	res, err := h.uc.Check(c.Request.Context(), middleware.PrincipalOf(c), requestID(c), c.Param("evidenceId"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, checkBody(res))
}

// Checks answers EVD-04.
func (h *Evidence) Checks(c *gin.Context) {
	if !onlyQuery(c, "limit", "cursor", "sort") {
		return
	}
	page, ok := pageInput(c)
	if !ok {
		return
	}
	res, err := h.uc.Checks(c.Request.Context(), middleware.PrincipalOf(c), c.Param("evidenceId"), page)
	if err != nil {
		fail(c, err)
		return
	}
	items := make([]checkJSON, 0, len(res.Items))
	for _, r := range res.Items {
		items = append(items, checkBody(r))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": pageBody(res.Page)})
}

// Content answers EVD-05. The bytes are untrusted: they are an attachment of an
// opaque type, and the global headers add nosniff, no-store and a CSP of none.
func (h *Evidence) Content(c *gin.Context) {
	if !onlyQuery(c, "acknowledgeIntegrity") {
		return
	}
	acknowledge := false
	switch c.Query("acknowledgeIntegrity") {
	case "", "false":
	case "true":
		acknowledge = true
	default:
		invalidQuery(c, "acknowledgeIntegrity", "invalid")
		return
	}
	res, err := h.uc.Open(c.Request.Context(), middleware.PrincipalOf(c), requestID(c), c.Param("evidenceId"), acknowledge)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="evidence-`+res.ID+`.bin"`)
	c.Data(http.StatusOK, "application/octet-stream", res.Data)
}
