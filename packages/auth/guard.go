package auth

import (
	"context"
	"strings"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

// ctxKey is the request-context key carrying verified Claims.
type ctxKey struct{}

// AuthGuard enforces `Authorization: Bearer <jwt>` on routes. Verified
// claims are attached to the request context for handlers to read with
// CurrentUser. Use it per controller or endpoint:
//
//	guard := auth.AuthGuard(jwtService)
//	users := grove.ControllerDef{Prefix: "/users", Middleware: []router.Middleware{guard}, ...}
func AuthGuard(svc *Service) router.Middleware {
	return grove.UseGuard(grove.GuardFunc(func(c router.Context) error {
		token, ok := bearerToken(c.Header("Authorization"))
		if !ok {
			return grove.Unauthorized("missing bearer token")
		}
		claims, err := svc.Verify(token)
		if err != nil {
			return grove.Unauthorized("invalid token")
		}
		// Attach claims without mutating the incoming request pointer:
		// handlers reading c.Request() see the enriched context.
		*c.Request() = *c.Request().WithContext(context.WithValue(c.Request().Context(), ctxKey{}, claims))
		return nil
	}))
}

// CurrentUser returns the verified claims for the request, or false when
// the route is not behind AuthGuard.
func CurrentUser(c router.Context) (Claims, bool) {
	claims, ok := c.Request().Context().Value(ctxKey{}).(Claims)
	return claims, ok
}

// RequireRole builds a guard that layers on top of AuthGuard: the request
// must carry role in claims Extra["roles"].
//
// Roles accept the shapes JWT round-trips produce: []any (encoding/json),
// []string (hand-built claims), or a single string. Anything else denies
// with 403 instead of failing open or panicking.
func RequireRole(role string) router.Middleware {
	return grove.UseGuard(grove.GuardFunc(func(c router.Context) error {
		claims, ok := CurrentUser(c)
		if !ok {
			return grove.Unauthorized("missing bearer token")
		}
		for _, r := range claimRoles(claims) {
			if r == role {
				return nil
			}
		}
		return grove.Forbidden("insufficient role")
	}))
}

// RequireAnyRole passes when the caller carries at least one of roles.
func RequireAnyRole(roles ...string) router.Middleware {
	return grove.UseGuard(grove.GuardFunc(func(c router.Context) error {
		claims, ok := CurrentUser(c)
		if !ok {
			return grove.Unauthorized("missing bearer token")
		}
		have := claimRoles(claims)
		for _, want := range roles {
			for _, h := range have {
				if h == want {
					return nil
				}
			}
		}
		return grove.Forbidden("insufficient role")
	}))
}

// claimRoles normalizes Extra["roles"] across JSON and hand-built shapes.
func claimRoles(claims Claims) []string {
	raw, ok := claims.Extra["roles"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, r := range v {
			if s, ok := r.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// bearerToken splits an Authorization header into its token.
func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}
