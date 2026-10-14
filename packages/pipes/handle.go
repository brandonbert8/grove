package pipes

import (
	"github.com/brandonbert8/grove/packages/router"
)

// QueryHandler is a declarative query handler: receives bound + strictly
// validated query struct Q, returns a payload (written as JSON status)
// or an error. The Go equivalent of NestJS `@Query() q` with the
// ValidationPipe applied — no BindQuery boilerplate:
//
//	pipes.HandleQuery(svc.Search, 200)
//
//	type SearchQuery struct {
//	    Q     string `query:"q" validate:"required"`
//	    Limit int    `query:"limit" validate:"gte=1,lte=100"`
//	}
type QueryHandler[Q any] func(c router.Context, q Q) (any, error)

// PathHandler is a declarative path-param handler: receives bound +
// strictly validated path struct P (the `@Param()` equivalent).
type PathHandler[P any] func(c router.Context, p P) (any, error)

// BodyQueryHandler is a declarative body+query handler for endpoints
// needing both (e.g. POST with ?dry_run=1): body is decoded + strictly
// validated first, then the query.
type BodyQueryHandler[B any, Q any] func(c router.Context, body B, q Q) (any, error)

// HandleQuery wraps fn with query bind + strict tag validation +
// dispatch. Malformed values are 400, rule violations are 422 with
// field details — the same contract as BindQuery. Status defaults to
// 200 when 0; a nil payload answers NoContent(status).
//
// Prefer HandleQuery over calling BindQuery inside a plain handler:
// same contract, no per-handler boilerplate, status in one place.
func HandleQuery[Q any](fn QueryHandler[Q], status int) router.HandlerFunc {
	if status == 0 {
		status = 200
	}
	return func(c router.Context) error {
		var q Q
		if err := BindQuery(c, &q); err != nil {
			return err
		}
		out, err := fn(c, q)
		if err != nil {
			return err
		}
		if out == nil {
			return c.NoContent(status)
		}
		return c.JSON(status, out)
	}
}

// HandleQueryOK wraps fn with query bind + validation + 200 dispatch
// (the canonical read shorthand, symmetric with grove.HandleBodyCreate).
func HandleQueryOK[Q any](fn QueryHandler[Q]) router.HandlerFunc {
	return HandleQuery(fn, 200)
}

// HandlePath wraps fn with path-param bind + strict tag validation +
// dispatch. Missing params and unparsable values are 400, rule
// violations are 422 — the same contract as BindPath. Status defaults
// to 200 when 0; a nil payload answers NoContent(status).
func HandlePath[P any](fn PathHandler[P], status int) router.HandlerFunc {
	if status == 0 {
		status = 200
	}
	return func(c router.Context) error {
		var p P
		if err := BindPath(c, &p); err != nil {
			return err
		}
		out, err := fn(c, p)
		if err != nil {
			return err
		}
		if out == nil {
			return c.NoContent(status)
		}
		return c.JSON(status, out)
	}
}

// HandleBodyQuery wraps fn with body decode + strict validation, then
// query bind + strict validation, then dispatch. Body errors win (400
// malformed / 422 violations) before query binding runs. Status
// defaults to 200 when 0; a nil payload answers NoContent(status).
func HandleBodyQuery[B any, Q any](fn BodyQueryHandler[B, Q], status int) router.HandlerFunc {
	if status == 0 {
		status = 200
	}
	return func(c router.Context) error {
		body, err := Body[B](c)
		if err != nil {
			return err
		}
		var q Q
		if err := BindQuery(c, &q); err != nil {
			return err
		}
		out, err := fn(c, body, q)
		if err != nil {
			return err
		}
		if out == nil {
			return c.NoContent(status)
		}
		return c.JSON(status, out)
	}
}

// HandleBodyQueryCreate wraps fn with body+query binding + 201 dispatch
// (the canonical create-with-options shorthand).
func HandleBodyQueryCreate[B any, Q any](fn BodyQueryHandler[B, Q]) router.HandlerFunc {
	return HandleBodyQuery(fn, 201)
}
