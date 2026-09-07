package grove

import (
	"github.com/brandonbert8/grove/packages/router"
)

// CanActivate is Grove's guard contract, the Go equivalent of a NestJS
// CanActivate. Return nil to let the request proceed or an error (usually
// an *HttpError like Unauthorized or Forbidden) to reject it.
type CanActivate interface {
	// CanActivate decides whether the request proceeds.
	CanActivate(c router.Context) error
}

// GuardFunc adapts a plain function into a CanActivate.
type GuardFunc func(c router.Context) error

// CanActivate implements CanActivate.
func (f GuardFunc) CanActivate(c router.Context) error { return f(c) }

// UseGuard converts a CanActivate into route middleware. Attach it
// globally (app.Router.Use), per controller (ControllerDef.Middleware),
// or per endpoint (Endpoint.Middleware).
func UseGuard(g CanActivate) router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			if err := g.CanActivate(c); err != nil {
				return err
			}
			return next(c)
		}
	}
}

// Interceptor observes or wraps handler execution (tracing, metrics,
// response mapping), the Go equivalent of a NestJS interceptor. Unlike
// a guard it always calls next and may act on the result or error.
//
// Because interceptors share the middleware signature, plain
// router.Middleware values already are interceptors; this named type
// exists for documentation and future codegen.
type Interceptor = router.Middleware

// InterceptorFunc adapts a handler-wrapping function into an Interceptor.
func InterceptorFunc(fn func(next router.HandlerFunc) router.HandlerFunc) Interceptor {
	return router.Middleware(fn)
}

// Chain composes middleware into one unit for reuse across routes:
//
//	authenticated := grove.Chain(authGuard, rateLimit, timeout)
//	grove.GET("/profile", h.Profile, authenticated)
func Chain(mw ...router.Middleware) router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		for i := len(mw) - 1; i >= 0; i-- {
			next = mw[i](next)
		}
		return next
	}
}
