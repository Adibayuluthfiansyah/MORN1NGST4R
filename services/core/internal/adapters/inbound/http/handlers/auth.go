package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/middleware"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/request"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/inbound/http/response"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/inbound"
)

type Auth struct{ uc inbound.Auth }

func NewAuth(uc inbound.Auth) *Auth { return &Auth{uc: uc} }

type userJSON struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"displayName"`
}

func user(u inbound.UserSummary) userJSON {
	out := userJSON{ID: u.ID, Username: u.Username}
	if u.DisplayName != "" {
		out.DisplayName = &u.DisplayName
	}
	return out
}

type recoveryJSON struct {
	Reason             string    `json:"reason"`
	MarkerGeneration   *uint64   `json:"markerGeneration"`
	DatabaseGeneration *uint64   `json:"databaseGeneration"`
	DetectedAt         time.Time `json:"detectedAt"`
}

type stateJSON struct {
	State             string        `json:"state"`
	CSRFToken         string        `json:"csrfToken"`
	User              *userJSON     `json:"user"`
	IdleExpiresAt     *time.Time    `json:"idleExpiresAt"`
	AbsoluteExpiresAt *time.Time    `json:"absoluteExpiresAt"`
	SessionID         *string       `json:"sessionId"`
	Recovery          *recoveryJSON `json:"recovery"`
}

func stateBody(s inbound.SessionState) stateJSON {
	out := stateJSON{State: string(s.Kind), CSRFToken: s.CSRFToken}
	if s.User != nil {
		u := user(*s.User)
		out.User = &u
	}
	if s.SessionID != "" {
		out.SessionID = &s.SessionID
		idle, abs := s.IdleExpiresAt.UTC(), s.AbsoluteExpiresAt.UTC()
		out.IdleExpiresAt, out.AbsoluteExpiresAt = &idle, &abs
	}
	if s.Recovery != nil {
		r := &recoveryJSON{Reason: s.Recovery.Reason, DetectedAt: s.Recovery.DetectedAt.UTC()}
		if s.Recovery.MarkerGeneration != 0 {
			r.MarkerGeneration = &s.Recovery.MarkerGeneration
		}
		if s.Recovery.DatabaseGeneration != 0 {
			r.DatabaseGeneration = &s.Recovery.DatabaseGeneration
		}
		out.Recovery = r
	}
	return out
}

func setCookie(c *gin.Context, name, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: c.Request.TLS != nil,
	})
}

func setSession(c *gin.Context, s inbound.IssuedSession) {
	maxAge := int(time.Until(s.ExpiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	setCookie(c, request.CookieSession, s.Token, maxAge)
}

func clearSession(c *gin.Context) { setCookie(c, request.CookieSession, "", -1) }

func requestID(c *gin.Context) string { return request.ID(c) }

func bind(c *gin.Context, dst any) bool {
	switch request.DecodeJSON(c, dst) {
	case request.BodyOK:
		return true
	case request.BodyTooLarge:
		response.Error(c, http.StatusRequestEntityTooLarge, response.CodePayloadTooLarge)
	default:
		response.Error(c, http.StatusBadRequest, response.CodeInvalidRequest)
	}
	return false
}

// fail maps a use case error to the error envelope. An error it does not know
// is a 500 and nothing of it reaches the response.
func fail(c *gin.Context, err error) {
	var vErr *inbound.ValidationError
	var rErr *inbound.RateLimitedError
	switch {
	case errors.As(err, &vErr):
		fields := make([]response.FieldError, 0, len(vErr.Fields))
		for _, f := range vErr.Fields {
			fields = append(fields, response.FieldError{Field: f.Field, Code: f.Code, Message: "The field is invalid."})
		}
		response.Detailed(c, http.StatusUnprocessableEntity, response.CodeValidationFailed, fields, 0)
	case errors.As(err, &rErr):
		response.Detailed(c, http.StatusTooManyRequests, response.CodeRateLimited, nil, rErr.RetryAfter)
	case errors.Is(err, inbound.ErrInvalidCredentials):
		response.Error(c, http.StatusUnauthorized, response.CodeInvalidCredentials)
	case errors.Is(err, inbound.ErrBootstrapSecretInvalid):
		response.Error(c, http.StatusUnauthorized, response.CodeBootstrapInvalid)
	case errors.Is(err, inbound.ErrRecoveryInvalid):
		response.Error(c, http.StatusUnauthorized, response.CodeRecoveryInvalid)
	case errors.Is(err, inbound.ErrUnauthenticated):
		response.Error(c, http.StatusUnauthorized, response.CodeUnauthenticated)
	case errors.Is(err, inbound.ErrSessionExpired):
		response.Error(c, http.StatusUnauthorized, response.CodeSessionExpired)
	case errors.Is(err, inbound.ErrReauthRequired):
		response.Error(c, http.StatusForbidden, response.CodeReauthRequired)
	case errors.Is(err, inbound.ErrReauthInvalid):
		response.Error(c, http.StatusForbidden, response.CodeReauthInvalid)
	case errors.Is(err, inbound.ErrNotFound):
		response.Error(c, http.StatusNotFound, response.CodeNotFound)
	case errors.Is(err, inbound.ErrAccountExists):
		response.Error(c, http.StatusConflict, response.CodeAccountExists)
	case errors.Is(err, inbound.ErrSetupUnavailable):
		response.Error(c, http.StatusConflict, response.CodeSetupUnavailable)
	case errors.Is(err, inbound.ErrConflictState):
		response.Error(c, http.StatusConflict, response.CodeConflictState)
	case errors.Is(err, inbound.ErrRecoveryNotRequired):
		response.Error(c, http.StatusConflict, response.CodeRecoveryNotRequired)
	case errors.Is(err, inbound.ErrReauthSubjectInvalid):
		response.Error(c, http.StatusUnprocessableEntity, response.CodeReauthSubjectInvalid)
	case errors.Is(err, inbound.ErrDatabaseUnavailable):
		response.Error(c, http.StatusServiceUnavailable, response.CodeDatabaseUnavailable)
	default:
		response.Error(c, http.StatusInternalServerError, response.CodeInternal)
	}
}

func (h *Auth) cookies(c *gin.Context) inbound.Cookies {
	s, _ := c.Cookie(request.CookieSession)
	a, _ := c.Cookie(request.CookieAnon)
	return inbound.Cookies{Session: s, Anon: a}
}

// State answers GET /session.
func (h *Auth) State(c *gin.Context) {
	cookies := h.cookies(c)
	st, err := h.uc.SessionState(c.Request.Context(), cookies)
	if err != nil {
		fail(c, err)
		return
	}
	if st.NewAnonToken != "" {
		setCookie(c, request.CookieAnon, st.NewAnonToken, int((15 * time.Minute).Seconds()))
	}
	if cookies.Session != "" && st.Kind != inbound.StateAuthenticated {
		clearSession(c)
	}
	c.JSON(http.StatusOK, stateBody(st))
}

type setupRequest struct {
	BootstrapSecret string `json:"bootstrapSecret"`
	Username        string `json:"username"`
	Password        string `json:"password"`
}

func (h *Auth) Setup(c *gin.Context) {
	var in setupRequest
	if !bind(c, &in) {
		return
	}
	res, err := h.uc.Setup(c.Request.Context(), inbound.SetupInput{
		BootstrapSecret: in.BootstrapSecret, Username: in.Username, Password: in.Password,
		ClientKey: request.ClientKey(c), UserAgent: c.GetHeader("User-Agent"), RequestID: requestID(c),
	})
	if err != nil {
		fail(c, err)
		return
	}
	setSession(c, res.Session)
	c.JSON(http.StatusCreated, gin.H{"user": user(res.User), "recoveryCode": res.RecoveryCode, "csrfToken": res.Session.CSRFToken})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Auth) Login(c *gin.Context) {
	var in loginRequest
	if !bind(c, &in) {
		return
	}
	res, err := h.uc.Login(c.Request.Context(), inbound.LoginInput{
		Username: in.Username, Password: in.Password, ClientKey: request.ClientKey(c), UserAgent: c.GetHeader("User-Agent"), RequestID: requestID(c),
	})
	if err != nil {
		fail(c, err)
		return
	}
	setSession(c, res.Session)
	c.JSON(http.StatusOK, stateBody(res.State))
}

func (h *Auth) Logout(c *gin.Context) {
	if err := h.uc.Logout(c.Request.Context(), middleware.PrincipalOf(c), requestID(c)); err != nil {
		fail(c, err)
		return
	}
	clearSession(c)
	c.Status(http.StatusNoContent)
}

type sessionJSON struct {
	ID                string    `json:"id"`
	CreatedAt         time.Time `json:"createdAt"`
	LastSeenAt        time.Time `json:"lastSeenAt"`
	IdleExpiresAt     time.Time `json:"idleExpiresAt"`
	AbsoluteExpiresAt time.Time `json:"absoluteExpiresAt"`
	Current           bool      `json:"current"`
	UserAgent         *string   `json:"userAgent"`
}

func sessionBody(v inbound.SessionView) sessionJSON {
	out := sessionJSON{ID: v.ID, CreatedAt: v.CreatedAt.UTC(), LastSeenAt: v.LastSeenAt.UTC(), IdleExpiresAt: v.IdleExpiresAt.UTC(), AbsoluteExpiresAt: v.AbsoluteExpiresAt.UTC(), Current: v.Current}
	if v.UserAgent != "" {
		out.UserAgent = &v.UserAgent
	}
	return out
}

func (h *Auth) List(c *gin.Context) {
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			response.Detailed(c, http.StatusUnprocessableEntity, response.CodeValidationFailed, []response.FieldError{{Field: "limit", Code: "out_of_range", Message: "The field is invalid."}}, 0)
			return
		}
		limit = n
	}
	views, err := h.uc.ListSessions(c.Request.Context(), middleware.PrincipalOf(c), limit)
	if err != nil {
		fail(c, err)
		return
	}
	items := make([]sessionJSON, 0, len(views))
	for _, v := range views {
		items = append(items, sessionBody(v))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "page": gin.H{"limit": limit, "nextCursor": nil, "hasMore": false}})
}

func (h *Auth) Revoke(c *gin.Context) {
	v, err := h.uc.RevokeSession(c.Request.Context(), middleware.PrincipalOf(c), c.Param("sessionId"), requestID(c))
	if err != nil {
		fail(c, err)
		return
	}
	if v.Current {
		clearSession(c)
	}
	c.JSON(http.StatusOK, sessionBody(v))
}

type reauthRequest struct {
	Password string         `json:"password"`
	Purpose  string         `json:"purpose"`
	Subject  map[string]any `json:"subject"`
}

func (h *Auth) Reauthenticate(c *gin.Context) {
	var in reauthRequest
	if !bind(c, &in) {
		return
	}
	if in.Subject == nil {
		fail(c, &inbound.ValidationError{Fields: []inbound.FieldError{{Field: "subject", Code: "required"}}})
		return
	}
	g, err := h.uc.Reauthenticate(c.Request.Context(), middleware.PrincipalOf(c), inbound.ReauthInput{
		Password: in.Password, Purpose: in.Purpose, Subject: in.Subject, ClientKey: request.ClientKey(c), RequestID: requestID(c),
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"grantId": g.ID, "purpose": g.Purpose, "expiresAt": g.ExpiresAt.UTC()})
}

type resetRequest struct {
	RecoveryCode string `json:"recoveryCode"`
	NewPassword  string `json:"newPassword"`
}

func (h *Auth) ResetPassword(c *gin.Context) {
	var in resetRequest
	if !bind(c, &in) {
		return
	}
	res, err := h.uc.ResetPassword(c.Request.Context(), inbound.RecoveryResetInput{
		RecoveryCode: in.RecoveryCode, NewPassword: in.NewPassword, ClientKey: request.ClientKey(c), UserAgent: c.GetHeader("User-Agent"), RequestID: requestID(c),
	})
	if err != nil {
		fail(c, err)
		return
	}
	setSession(c, res.Session)
	c.JSON(http.StatusOK, gin.H{"recoveryCode": res.RecoveryCode})
}

type reconcileRequest struct {
	Password          string `json:"password"`
	NewPassword       string `json:"newPassword"`
	ConfirmRevocation bool   `json:"confirmRevocation"`
}

func (h *Auth) Reconcile(c *gin.Context) {
	var in reconcileRequest
	if !bind(c, &in) {
		return
	}
	res, err := h.uc.Reconcile(c.Request.Context(), inbound.ReconcileInput{
		Password: in.Password, NewPassword: in.NewPassword, ConfirmRevocation: in.ConfirmRevocation, ClientKey: request.ClientKey(c), RequestID: requestID(c),
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"generation": res.Generation, "revokedSessions": res.RevokedSessions, "revokedApprovals": res.RevokedApprovals,
		"staleValidations": res.StaleValidations, "revokedLabs": res.RevokedLabs, "cancelledJobs": res.CancelledJobs,
		"recoveryCode": res.RecoveryCode,
	})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
	ReauthGrantID   string `json:"reauthGrantId"`
}

func (h *Auth) ChangePassword(c *gin.Context) {
	var in changePasswordRequest
	if !bind(c, &in) {
		return
	}
	s, err := h.uc.ChangePassword(c.Request.Context(), middleware.PrincipalOf(c), inbound.ChangePasswordInput{
		CurrentPassword: in.CurrentPassword, NewPassword: in.NewPassword, ReauthGrantID: in.ReauthGrantID,
		ClientKey: request.ClientKey(c), UserAgent: c.GetHeader("User-Agent"), RequestID: requestID(c),
	})
	if err != nil {
		fail(c, err)
		return
	}
	setSession(c, s)
	c.Status(http.StatusNoContent)
}
