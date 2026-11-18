package request

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"

	"github.com/gin-gonic/gin"
)

const (
	CookieSession = "ms_session"
	CookieAnon    = "ms_anon"
	HeaderCSRF    = "X-CSRF-Token"

	AuthBodyLimit = 16 << 10
)

type BodyError int

const (
	BodyOK BodyError = iota
	BodyMalformed
	BodyTooLarge
	BodyWrongMediaType
)

// LimitJSON bounds the body and checks the media type before any handler reads
// it. It does not read the body.
func LimitJSON(c *gin.Context, limit int64) BodyError {
	if c.Request.ContentLength > limit {
		return BodyTooLarge
	}
	mt, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mt != "application/json" {
		return BodyWrongMediaType
	}
	if cs, ok := params["charset"]; ok && cs != "utf-8" && cs != "UTF-8" {
		return BodyWrongMediaType
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	return BodyOK
}

// DecodeJSON reads exactly one JSON value into dst. An unknown field, a second
// value or trailing bytes is an error (API-23).
func DecodeJSON(c *gin.Context, dst any) BodyError {
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return classify(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return classify(err)
		}
		return BodyMalformed
	}
	return BodyOK
}

func classify(err error) BodyError {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return BodyTooLarge
	}
	return BodyMalformed
}

// ClientKey identifies the caller for rate limiting. The listener is loopback
// only, so every caller is "loopback", and nothing the caller controls (the
// source port, X-Forwarded-For, the Host header) can pick another bucket.
func ClientKey(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err == nil {
		if ip, err := netip.ParseAddr(host); err == nil && ip.Unmap().IsLoopback() {
			return "loopback"
		}
	}
	return "other"
}
