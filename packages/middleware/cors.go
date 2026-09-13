package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/brandonbert8/grove/packages/router"
)

// CORSConfig tunes the CORS middleware.
type CORSConfig struct {
	// AllowOrigins lists permitted Origin values. "*" allows all.
	AllowOrigins []string
	// AllowMethods lists permitted methods for preflight.
	AllowMethods []string
	// AllowHeaders lists permitted request headers for preflight.
	AllowHeaders []string
	// AllowCredentials emits Access-Control-Allow-Credentials.
	AllowCredentials bool
	// ExposeHeaders lists response headers browsers may read
	// (Access-Control-Expose-Headers), e.g. X-Request-ID, X-Total-Count.
	ExposeHeaders []string
	// MaxAge caches preflight results for this many seconds.
	MaxAge int
}

// DefaultCORSConfig is a permissive development default.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodDelete, http.MethodPatch, http.MethodOptions,
		},
		AllowHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:       86400,
	}
}

// CORS enforces a simple, explicit cross-origin policy: matching origins
// pass, others get no CORS headers; OPTIONS preflights short-circuit
// with 204 on match.
func CORS(cfg CORSConfig) router.Middleware {
	if len(cfg.AllowOrigins) == 0 {
		cfg.AllowOrigins = []string{"*"}
	}
	if len(cfg.AllowMethods) == 0 {
		cfg.AllowMethods = []string{http.MethodGet, http.MethodPost, http.MethodOptions}
	}
	allowAll := len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*"

	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			origin := c.Request().Header.Get("Origin")
			allowed := allowAll || originAllowed(origin, cfg.AllowOrigins)

			if allowed && origin != "" {
				h := c.ResponseWriter().Header()
				if allowAll && !cfg.AllowCredentials {
					h.Set("Access-Control-Allow-Origin", "*")
				} else {
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Vary", "Origin")
				}
				if cfg.AllowCredentials {
					h.Set("Access-Control-Allow-Credentials", "true")
				}
				if len(cfg.ExposeHeaders) > 0 {
					h.Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
				}
			}

			if c.Request().Method == http.MethodOptions {
				h := c.ResponseWriter().Header()
				h.Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
				if len(cfg.AllowHeaders) > 0 {
					h.Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
				}
				if cfg.MaxAge > 0 {
					h.Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
				}
				if allowed {
					return c.NoContent(http.StatusNoContent)
				}
				// Fall through for disallowed origins so the route (or
				// 404) still decides; no CORS headers are emitted.
				return next(c)
			}

			return next(c)
		}
	}
}

func originAllowed(origin string, allow []string) bool {
	if origin == "" {
		return false
	}
	for _, a := range allow {
		if a == origin {
			return true
		}
	}
	return false
}
