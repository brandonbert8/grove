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
