// Package request holds transport-level helpers for reading request data.
package request

import (
	"crypto/rand"
	"fmt"
	"regexp"

	"github.com/gin-gonic/gin"
)

const idKey = "morningstar.request_id"

// HeaderID is the request identifier header (API-13).
const HeaderID = "X-Request-Id"

// UnknownID is the nil UUID. Recovery uses it when a request panicked before
// the request-id middleware assigned an identifier, so that the response and
// the log still carry a well-formed id.
const UnknownID = "00000000-0000-0000-0000-000000000000"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidID reports whether s is a canonical UUID string. A caller supplied
// request id that is not one is replaced, never echoed.
func ValidID(s string) bool { return uuidPattern.MatchString(s) }

// NewID returns a random (version 4) UUID.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on supported platforms. Failing closed
		// here is better than issuing a predictable identifier.
		panic("request: no randomness available")
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// SetID stores the request id on the Gin context.
func SetID(c *gin.Context, id string) { c.Set(idKey, id) }

// ID returns the request id stored by the request-id middleware, or "".
func ID(c *gin.Context) string {
	if v, ok := c.Get(idKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
