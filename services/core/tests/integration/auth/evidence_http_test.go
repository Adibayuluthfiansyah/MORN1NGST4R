package auth_test

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/audit"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/infrastructure/ids"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

// evWorld is a signed-in researcher with one assessment, and a way to store evidence the way a later milestone will.
type evWorld struct {
	a          *app
	b          *browser
	userID     string
	assessment string
}

func newEvWorld(t *testing.T) *evWorld {
	t.Helper()
	_, a, b, _ := fresh(t)
	w := &evWorld{a: a, b: b, userID: a.scalar(`SELECT id FROM morningstar.users`)}
	w.assessment = w.seedAssessment(t, w.userID)
	return w
}

func (w *evWorld) seedAssessment(t *testing.T, owner string) string {
	t.Helper()
	conn := w.a.c.Owner(t)
	var program, assessment string
	if err := conn.QueryRow(t.Context(), `INSERT INTO morningstar.programs (id, owner_user_id, name) VALUES (uuidv7(), $1, 'p') RETURNING id`, owner).Scan(&program); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(t.Context(), `INSERT INTO morningstar.assessments (id, program_id, name, created_by) VALUES (uuidv7(), $1, 'a', $2) RETURNING id`, program, owner).Scan(&assessment); err != nil {
		t.Fatal(err)
	}
	return assessment
}

func (w *evWorld) capture(t *testing.T, assessment, content string) evidence.Item {
	t.Helper()
	who, err := audit.User(w.userID)
	if err != nil {
		t.Fatal(err)
	}
	it, err := w.a.srv.Evidence().Capture(t.Context(), inbound.CaptureInput{
		AssessmentID: assessment, Kind: evidence.KindResponse, ContentType: "text/html", Origin: evidence.Observed,
		Content: []byte(content), Actor: who, RequestID: ids.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func (w *evWorld) path(it evidence.Item) string { return filepath.Join(w.a.evidence, it.StorageKey()) }

func TestEvidenceRoutesNeedASession(t *testing.T) {
	w := newEvWorld(t)
	it := w.capture(t, w.assessment, "x")
	anon := newBrowser(w.a)
	anon.state()
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/assessments/" + w.assessment + "/evidence"},
		{"GET", "/api/v1/evidence/" + it.ID()},
		{"POST", "/api/v1/evidence/" + it.ID() + "/integrity-checks"},
		{"GET", "/api/v1/evidence/" + it.ID() + "/integrity-checks"},
		{"GET", "/api/v1/evidence/" + it.ID() + "/content"},
	} {
		if got := anon.do(r.method, r.path, nil); got.status != 401 || got.code() != "unauthenticated" {
			t.Errorf("%s %s = %d %s", r.method, r.path, got.status, got.code())
		}
	}
}

func TestEvidenceIsListedReadAndDownloadedAsAnAttachment(t *testing.T) {
	w := newEvWorld(t)
	secret := strings.Repeat("hunter", 6)
	first := w.capture(t, w.assessment, "<html><script>alert(1)</script>Authorization: Bearer "+secret+"</html>")
	second := w.capture(t, w.assessment, "second")

	page := w.b.do("GET", "/api/v1/assessments/"+w.assessment+"/evidence?limit=1", nil)
	if page.status != 200 {
		t.Fatalf("list = %d %s", page.status, page.body)
	}
	items, _ := page.json()["items"].([]any)
	pg, _ := page.json()["page"].(map[string]any)
	if len(items) != 1 || pg["hasMore"] != true || pg["limit"] != float64(1) || pg["nextCursor"] == nil {
		t.Fatalf("page = %s", page.body)
	}
	if got := items[0].(map[string]any)["id"]; got != second.ID() {
		t.Errorf("the default order is not newest first: %v", got)
	}
	next := w.b.do("GET", "/api/v1/assessments/"+w.assessment+"/evidence?limit=1&cursor="+pg["nextCursor"].(string), nil)
	nextItems, _ := next.json()["items"].([]any)
	nextPage, _ := next.json()["page"].(map[string]any)
	if len(nextItems) != 1 || nextItems[0].(map[string]any)["id"] != first.ID() || nextPage["hasMore"] != false || nextPage["nextCursor"] != nil {
		t.Fatalf("second page = %s", next.body)
	}
	asc := w.b.do("GET", "/api/v1/assessments/"+w.assessment+"/evidence?sort=createdAt", nil)
	if got := asc.json()["items"].([]any)[0].(map[string]any)["id"]; got != first.ID() {
		t.Errorf("sort=createdAt gave %v first", got)
	}

	get := w.b.do("GET", "/api/v1/evidence/"+first.ID(), nil)
	if get.status != 200 || get.str("sha256") != first.SHA256().String() || get.str("integrityState") != "unchecked" || get.str("retentionState") != "active" ||
		get.str("redactionState") != "redacted" || get.str("assessmentId") != w.assessment || get.json()["sizeBytes"] != float64(first.Size()) {
		t.Fatalf("get = %d %s", get.status, get.body)
	}
	if bytes.Contains(get.body, []byte("storageKey")) || bytes.Contains(get.body, []byte(w.a.evidence)) {
		t.Error("the response names the location of the file")
	}

	dl := w.b.do("GET", "/api/v1/evidence/"+first.ID()+"/content", nil)
	if dl.status != 200 {
		t.Fatalf("content = %d %s", dl.status, dl.body)
	}
	if got := dl.header.Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type = %q: stored evidence must never be served as a renderable type", got)
	}
	if got := dl.header.Get("Content-Disposition"); got != `attachment; filename="evidence-`+first.ID()+`.bin"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if dl.header.Get("X-Content-Type-Options") != "nosniff" || dl.header.Get("Cache-Control") != "no-store" || !strings.Contains(dl.header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Errorf("headers = %v", dl.header)
	}
	if bytes.Contains(dl.body, []byte(secret)) || !bytes.Contains(dl.body, []byte("<script>")) {
		t.Errorf("the download is not the redacted capture: %q", dl.body)
	}
	if evidence.DigestOf(dl.body) != first.SHA256() {
		t.Error("the downloaded bytes do not match the stored hash")
	}
	if n := w.a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'evidence.accessed' AND outcome = 'success' AND entity_id = $1`, first.ID()); n != "1" {
		t.Errorf("evidence.accessed events = %s", n)
	}
	if got := w.b.do("GET", "/api/v1/evidence/"+first.ID(), nil); got.str("integrityState") != "ok" {
		t.Errorf("a verified download did not record its check: %s", got.body)
	}
}

func TestAnIntegrityCheckIsAStateChangingRequest(t *testing.T) {
	w := newEvWorld(t)
	it := w.capture(t, w.assessment, "body")
	path := "/api/v1/evidence/" + it.ID() + "/integrity-checks"

	if r := w.b.do("POST", path, nil, noHeader("X-CSRF-Token")); r.status != 403 || r.code() != "csrf_invalid" {
		t.Errorf("without CSRF = %d %s", r.status, r.code())
	}
	if r := w.b.do("POST", path, nil, header("Origin", "http://evil.example")); r.status != 403 || r.code() != "origin_denied" {
		t.Errorf("foreign Origin = %d %s", r.status, r.code())
	}
	r := w.b.do("POST", path, nil)
	if r.status != 201 || r.str("result") != "match" || r.str("trigger") != "manual" || r.str("actorKind") != "user" || r.str("evidenceId") != it.ID() {
		t.Fatalf("check = %d %s", r.status, r.body)
	}
	h := w.b.do("GET", path, nil)
	items, _ := h.json()["items"].([]any)
	if h.status != 200 || len(items) != 1 {
		t.Fatalf("history = %d %s", h.status, h.body)
	}
	if got := w.b.do("GET", "/api/v1/evidence/"+it.ID(), nil); got.str("integrityState") != "ok" || got.str("lastCheckedAt") == "" {
		t.Errorf("state = %s", got.body)
	}
}

func TestATamperedFileIsWithheldOverHTTPUntilAPersonAcknowledges(t *testing.T) {
	w := newEvWorld(t)
	it := w.capture(t, w.assessment, "the original body")
	forged := "the forgery  body"
	if err := os.WriteFile(w.path(it), []byte(forged), 0o600); err != nil {
		t.Fatal(err)
	}
	content := "/api/v1/evidence/" + it.ID() + "/content"

	r := w.b.do("GET", content, nil)
	if r.status != 409 || r.code() != "evidence_integrity_failed" || bytes.Contains(r.body, []byte(forged)) {
		t.Fatalf("tampered download = %d %s", r.status, r.body)
	}
	if got := w.b.do("GET", "/api/v1/evidence/"+it.ID(), nil); got.str("integrityState") != "compromised" {
		t.Errorf("state = %s", got.body)
	}
	// The file is put back, and the item stays compromised.
	if err := os.WriteFile(w.path(it), []byte("the original body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := w.b.do("GET", content, nil); r.status != 409 || r.code() != "evidence_integrity_failed" {
		t.Errorf("a restored file of a compromised item = %d %s", r.status, r.code())
	}
	if r := w.b.do("GET", content+"?acknowledgeIntegrity=true", nil); r.status != 200 || string(r.body) != "the original body" {
		t.Errorf("acknowledged download = %d %q", r.status, r.body)
	}
	chk := w.b.do("POST", "/api/v1/evidence/"+it.ID()+"/integrity-checks", nil)
	if chk.status != 201 || chk.str("result") != "match" {
		t.Errorf("check = %d %s", chk.status, chk.body)
	}
	if got := w.b.do("GET", "/api/v1/evidence/"+it.ID(), nil); got.str("integrityState") != "compromised" {
		t.Errorf("a matching check cleared the compromise: %s", got.body)
	}
	if n := w.a.scalar(`SELECT count(*) FROM morningstar.audit_events WHERE action = 'evidence.integrity_failed' AND entity_id = $1`, it.ID()); n != "1" {
		t.Errorf("evidence.integrity_failed events = %s", n)
	}
}

func TestPurgedEvidenceKeepsItsMetadataAndLosesItsContent(t *testing.T) {
	w := newEvWorld(t)
	it := w.capture(t, w.assessment, "body")
	w.a.exec(`UPDATE morningstar.evidence_status SET retention_state = 'purged', purged_at = now(), purged_by = $2 WHERE evidence_id = $1`, it.ID(), w.userID)
	if r := w.b.do("GET", "/api/v1/evidence/"+it.ID()+"/content", nil); r.status != 410 || r.code() != "evidence_purged" {
		t.Errorf("content = %d %s", r.status, r.code())
	}
	if r := w.b.do("POST", "/api/v1/evidence/"+it.ID()+"/integrity-checks", nil); r.status != 410 || r.code() != "evidence_purged" {
		t.Errorf("check = %d %s", r.status, r.code())
	}
	if r := w.b.do("GET", "/api/v1/evidence/"+it.ID(), nil); r.status != 200 || r.str("retentionState") != "purged" {
		t.Errorf("metadata = %d %s", r.status, r.body)
	}
}

func TestEvidenceOfAnotherOwnerIsNotFound(t *testing.T) {
	w := newEvWorld(t)
	w.a.exec(`INSERT INTO morningstar.users (id, username, status) VALUES (uuidv7(), 'stranger', 'disabled')`)
	other := w.a.scalar(`SELECT id FROM morningstar.users WHERE username = 'stranger'`)
	theirs := w.seedAssessment(t, other)
	it := w.capture(t, theirs, "not yours")

	for _, p := range []string{
		"/api/v1/assessments/" + theirs + "/evidence",
		"/api/v1/evidence/" + it.ID(),
		"/api/v1/evidence/" + it.ID() + "/integrity-checks",
		"/api/v1/evidence/" + it.ID() + "/content",
		"/api/v1/evidence/" + it.ID() + "/content?acknowledgeIntegrity=true",
	} {
		if r := w.b.do("GET", p, nil); r.status != 404 || r.code() != "not_found" {
			t.Errorf("GET %s = %d %s", p, r.status, r.code())
		}
	}
	if r := w.b.do("POST", "/api/v1/evidence/"+it.ID()+"/integrity-checks", nil); r.status != 404 {
		t.Errorf("check = %d", r.status)
	}
	// The same answer for an id that does not exist at all.
	if r := w.b.do("GET", "/api/v1/evidence/"+ids.New(), nil); r.status != 404 || r.code() != "not_found" {
		t.Errorf("unknown id = %d %s", r.status, r.code())
	}
	if r := w.b.do("GET", "/api/v1/evidence/not-a-uuid", nil); r.status != 404 {
		t.Errorf("malformed id = %d", r.status)
	}
}

func TestEvidenceQueriesAreValidatedAtTheBoundary(t *testing.T) {
	w := newEvWorld(t)
	it := w.capture(t, w.assessment, "x")
	list := "/api/v1/assessments/" + w.assessment + "/evidence"
	for name, c := range map[string]struct {
		path   string
		status int
		code   string
	}{
		"limit zero":           {list + "?limit=0", 422, "validation_failed"},
		"limit over 200":       {list + "?limit=201", 422, "validation_failed"},
		"limit text":           {list + "?limit=ten", 422, "validation_failed"},
		"bad sort":             {list + "?sort=name", 422, "validation_failed"},
		"forged cursor":        {list + "?cursor=AAAA", 422, "validation_failed"},
		"unknown kind":         {list + "?kind=screenshot", 422, "validation_failed"},
		"unknown state":        {list + "?integrityState=fine", 422, "validation_failed"},
		"job not a uuid":       {list + "?jobId=1", 422, "validation_failed"},
		"unknown parameter":    {list + "?owner=me", 400, "invalid_request"},
		"repeated parameter":   {list + "?limit=1&limit=2", 400, "invalid_request"},
		"parameter on a get":   {"/api/v1/evidence/" + it.ID() + "?x=1", 400, "invalid_request"},
		"acknowledge text":     {"/api/v1/evidence/" + it.ID() + "/content?acknowledgeIntegrity=yes", 422, "validation_failed"},
		"acknowledge repeated": {"/api/v1/evidence/" + it.ID() + "/content?acknowledgeIntegrity=true&acknowledgeIntegrity=true", 400, "invalid_request"},
	} {
		if r := w.b.do("GET", c.path, nil); r.status != c.status || r.code() != c.code {
			t.Errorf("%s: %d %s, want %d %s", name, r.status, r.code(), c.status, c.code)
		}
	}
	if r := w.b.do("GET", list+"?kind=response&integrityState=unchecked", nil); r.status != 200 || len(r.json()["items"].([]any)) != 1 {
		t.Errorf("filters = %d %s", r.status, r.body)
	}
}

func TestDownloadsAreRateLimitedPerSession(t *testing.T) {
	w := newEvWorld(t)
	it := w.capture(t, w.assessment, "x")
	limited := 0
	for range 40 {
		if r := w.b.do("GET", "/api/v1/evidence/"+it.ID()+"/content", nil); r.status == http.StatusTooManyRequests {
			limited++
			if r.code() != "rate_limited" || r.header.Get("Retry-After") == "" {
				t.Errorf("429 = %s, Retry-After %q", r.code(), r.header.Get("Retry-After"))
			}
		}
	}
	if limited != 10 {
		t.Errorf("%d of 40 downloads were limited, want 10 (30 per minute)", limited)
	}
}
