package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/brandonbert8/grove/packages/router"
)

// RequestIDHeader is the header carrying the request id, inbound and outbound.
const RequestIDHeader = "X-Request-ID"

// requestIDKey is the request-context key carrying the id.
type requestIDKey struct{}

// RequestID ensures every request carries an id: inbound
// X-Request-ID is reused, otherwise a 128-bit random hex id is minted.
// The id is echoed back as a response header and readable via
// GetRequestID, so logs and downstream services correlate.
//
// Place it innermost-global (first in Use) so Logging and handlers see it.
func RequestID() router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			id := c.Request().Header.Get(RequestIDHeader)
			if !validRequestID(id) {
				id = newRequestID()
			}
			if sr, ok := c.(interface{ SetRequest(*http.Request) }); ok {
				sr.SetRequest(c.Request().WithContext(
					context.WithValue(c.Request().Context(), requestIDKey{}, id)))
			} else {
				*c.Request() = *c.Request().WithContext(
					context.WithValue(c.Request().Context(), requestIDKey{}, id))
			}
			c.ResponseWriter().Header().Set(RequestIDHeader, id)
			return next(c)
		}
	}
}

// validRequestID accepts tight inbound ids (prevents log/header
// injection and giant echoes): 1-128 chars of [A-Za-z0-9-_].
func validRequestID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' ||
			b >= '0' && b <= '9' || b == '-' || b == '_' {
			continue
		}
		return false
	}
	return true
}

// GetRequestID returns the id attached by RequestID, falling back to
// the inbound header when the middleware did not run.
func GetRequestID(c router.Context) string {
	if v, ok := c.Request().Context().Value(requestIDKey{}).(string); ok && v != "" {
		return v
	}
	return c.Request().Header.Get(RequestIDHeader)
}

// newRequestID mints 128-bit hex. crypto/rand cannot fail on Linux in
// practice; the timestamp fallback keeps the contract panic-free.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000000")))
}
