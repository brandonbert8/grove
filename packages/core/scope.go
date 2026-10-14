package grove

import (
	"net/http"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

// RequestScope is middleware that creates one di.Scope per request
// (NestJS Scope.REQUEST equivalent). It stores the scope in the request
// context so handlers resolve request-scoped providers with InjectRequest:
//
//	app.Router.Use(grove.RequestScope(app))
//
// The scope is closed at the end of the request, releasing any scoped
// values implementing io.Closer (e.g. per-request transactions) in
// reverse order. Scoped values must not retain the request or writer.
func RequestScope(app *App) router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			scope := app.Container.Scope(c.Request().Context())
			r := c.Request().WithContext(di.WithScope(c.Request().Context(), scope))
			if s, ok := c.(interface{ SetRequest(*http.Request) }); ok {
				s.SetRequest(r)
			}
			defer func() { _ = scope.Close() }()
			return next(c)
		}
	}
}

// InjectRequest resolves the request-scoped (or any visible) provider T
// from the Scope bound to c by RequestScope middleware. Singleton and
// transient providers delegate to the root container; request-scoped
// ones are cached per request:
//
//	v, err := grove.InjectRequest[*ReqCtx](ctx, app)
//
// Without the middleware (unit tests, grovtest), an ad-hoc scope is
// created once and pinned to the request so repeated resolves in the
// same handler still share one instance.
func InjectRequest[T any](c router.Context, app *App) (T, error) {
	return di.ResolveScoped[T](ScopeOf(c, app))
}

// MustInjectRequest is like InjectRequest but panics on error.
func MustInjectRequest[T any](c router.Context, app *App) T {
	v, err := InjectRequest[T](c, app)
	if err != nil {
		panic(err)
	}
	return v
}

// ScopeOf returns the request Scope for c, creating an ad-hoc one when
// RequestScope middleware is not mounted (tests) and pinning it to the
// request so later InjectRequest calls share it. Handlers needing
// several scoped resolves should reuse the returned scope.
func ScopeOf(c router.Context, app *App) *di.Scope {
	if s, ok := di.ScopeFrom(c.Request().Context()); ok {
		return s
	}
	s := app.Container.Scope(c.Request().Context())
	if setter, ok := c.(interface{ SetRequest(*http.Request) }); ok {
		setter.SetRequest(c.Request().WithContext(di.WithScope(c.Request().Context(), s)))
	}
	return s
}
