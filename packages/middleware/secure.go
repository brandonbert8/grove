package middleware

import (
	"github.com/brandonbert8/grove/packages/router"
)

// SecureHeaders sets baseline hardening headers without breaking JSON
// APIs (no CSP: that policy is app-specific and belongs in the app).
//
// Always set: X-Content-Type-Options=nosniff, X-Frame-Options=DENY,
// Referrer-Policy=no-referrer, Permissions-Policy with camera/
// microphone/geolocation disabled. Strict-Transport-Security is added
// only on TLS requests, so plain-HTTP dev never gets pinned.
func SecureHeaders() router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			h := c.ResponseWriter().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			if c.Request().TLS != nil {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			return next(c)
		}
	}
}
