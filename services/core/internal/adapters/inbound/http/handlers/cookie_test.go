package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func cookieFor(t *testing.T, secure bool) *http.Cookie {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	if secure {
		c.Request.TLS = &tls.ConnectionState{}
	}
	setCookie(c, "ms_session", "value", 60)
	cs := w.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("%d cookies", len(cs))
	}
	return cs[0]
}

func TestCookieAttributes(t *testing.T) {
	for _, secure := range []bool{false, true} {
		c := cookieFor(t, secure)
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" || c.MaxAge != 60 {
			t.Fatalf("cookie = %+v", c)
		}
		if c.Secure != secure {
			t.Fatalf("Secure = %v with TLS %v", c.Secure, secure)
		}
	}
}
