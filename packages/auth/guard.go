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
// must carry role in claims Extra["roles"] ([]any of strings).
func RequireRole(role string) router.Middleware {
	return grove.UseGuard(grove.GuardFunc(func(c router.Context) error {
		claims, ok := CurrentUser(c)
		if !ok {
			return grove.Unauthorized("missing bearer token")
		}
		roles, _ := claims.Extra["roles"].([]any)
		for _, r := range roles {
			if s, _ := r.(string); s == role {
				return nil
			}
		}
		return grove.Forbidden("insufficient role")
	}))
}

// bearerToken splits an Authorization header into its token.
func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}
