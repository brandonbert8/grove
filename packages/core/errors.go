package grove

import (
	"github.com/brandonbert8/grove/packages/httperr"
)

// HttpError is Grove's typed HTTP exception (alias of httperr.HttpError).
//
// v0.3: the canonical type lives in the leaf package
// packages/httperr so pipes/auth/router never import core.
// This alias keeps existing handlers compiling.
type HttpError = httperr.HttpError

// NewHttpError builds an HttpError with an optional details payload.
func NewHttpError(status int, message string, details ...any) *HttpError {
	return httperr.New(status, message, details...)
}

// BadRequest builds a 400 error.
func BadRequest(msg string, details ...any) *HttpError {
	return httperr.BadRequest(msg, details...)
}

// Unauthorized builds a 401 error.
func Unauthorized(msg string, details ...any) *HttpError {
	return httperr.Unauthorized(msg, details...)
}

// Forbidden builds a 403 error.
func Forbidden(msg string, details ...any) *HttpError {
	return httperr.Forbidden(msg, details...)
}

// NotFound builds a 404 error.
func NotFound(msg string, details ...any) *HttpError {
	return httperr.NotFound(msg, details...)
}

// Conflict builds a 409 error.
func Conflict(msg string, details ...any) *HttpError {
	return httperr.Conflict(msg, details...)
}

// Unprocessable builds a 422 error, used for validation failures.
func Unprocessable(msg string, details ...any) *HttpError {
	return httperr.Unprocessable(msg, details...)
}

// Internal builds a 500 error. Prefer letting unexpected failures fall
// through as plain errors; the router already maps those to 500.
func Internal(msg string, details ...any) *HttpError {
	return httperr.Internal(msg, details...)
}

// ErrorDetails is inherited from httperr.HttpError; see that package.

// AsHttpError unwraps err to an *HttpError when possible.
func AsHttpError(err error) (*HttpError, bool) { return httperr.As(err) }
