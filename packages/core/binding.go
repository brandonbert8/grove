package grove

import (
	"github.com/brandonbert8/grove/packages/httperr"
	"github.com/brandonbert8/grove/packages/router"
	"github.com/brandonbert8/grove/packages/validate"
)

// BodyHandler is a declarative DTO handler: receives a validated body of
// type T plus the request context, returns a payload (written as JSON
// 200/201) or an error. It is the Go equivalent of NestJS `@Body() dto`
// with the ValidationPipe applied — no pipes.Body boilerplate:
//
//	grove.POST("", grove.HandleBody(svc.Create, 201),
//	    grove.WithSummary("Create user"),
//	    openapi.WithBody[CreateUser]())
type BodyHandler[T any] func(c router.Context, body T) (any, error)

// HandleBody wraps fn with decode + strict tag validation + dispatch.
// Malformed JSON is 400, rule violations are 422 with field details —
// the same contract as pipes.Body, without the per-handler call.
//
// Status defaults to 200; pass e.g. 201 for creates.
func HandleBody[T any](fn BodyHandler[T], status int) router.HandlerFunc {
	if status == 0 {
		status = 200
	}
	return func(c router.Context) error {
		var v T
		if err := decodeStrictBody(c, &v); err != nil {
			return err
		}
		out, err := fn(c, v)
		if err != nil {
			return err
		}
		if out == nil {
			return c.NoContent(status)
		}
		return c.JSON(status, out)
	}
}

// HandleBodyCreate wraps fn with decode + strict tag validation + 201
// dispatch. It is the canonical create shorthand so callers never
// hardcode the status in two places (handler + docs):
//
//	grove.POST("", grove.HandleBodyCreate(svc.Create),
//	    grove.WithSummary("Create user"))
//
// Prefer HandleBody/HandleBodyCreate over calling pipes.Body inside a
// plain router.HandlerFunc: both share the same 400/422 contract, but
// HandleBody* removes the per-handler decode boilerplate and keeps
// the status in one place.
func HandleBodyCreate[T any](fn BodyHandler[T]) router.HandlerFunc {
	return HandleBody(fn, 201)
}

// HandleBodyOK wraps fn with decode + strict tag validation + 200
// dispatch (update/replace shorthand, symmetric with HandleBodyCreate).
func HandleBodyOK[T any](fn BodyHandler[T]) router.HandlerFunc {
	return HandleBody(fn, 200)
}
// decodeStrictBody is the shared DTO pipeline over the leaf validate
// package (core cannot import pipes without a cycle): JSON decode with
// the default cap, then strict tag validation.
func decodeStrictBody(c router.Context, v any) error {
	if err := c.Body(v); err != nil {
		return httperr.BadRequest("invalid request body: " + err.Error())
	}
	if ferrs := validate.ValidateStrict(v); len(ferrs) > 0 {
		return httperr.Unprocessable("", ferrs)
	}
	return nil
}
