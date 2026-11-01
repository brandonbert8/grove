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

// NewHttpErrorCode builds an HttpError with a stable machine-readable
// code (e.g. "USER_NOT_FOUND") rendered as the JSON "code" field.
func NewHttpErrorCode(status int, code, message string, details ...any) *HttpError {
	return httperr.NewCode(status, code, message, details...)
}

// WrapHttpError wraps err with an HTTP status/message while preserving
// the cause for errors.Is / errors.As. Returns nil when err is nil.
func WrapHttpError(err error, status int, message string, details ...any) *HttpError {
	return httperr.Wrap(err, status, message, details...)
}

// WrapHttpErrorf is WrapHttpError with a formatted message.
func WrapHttpErrorf(err error, status int, format string, args ...any) *HttpError {
	return httperr.Wrapf(err, status, format, args...)
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
