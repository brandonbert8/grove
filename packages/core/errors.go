package grove

import (
	"errors"
	"net/http"
)

// HttpError is Grove's typed HTTP exception, the Go equivalent of a
// NestJS HttpException. Handlers and pipes return it; the router maps it
// to its status code automatically (see router.Statuser), so no explicit
// exception-filter middleware is required for the common cases.
type HttpError struct {
	// Status is the HTTP status code (400-599).
	Status int
	// Message is the human-readable reason, exposed in the JSON body.
	Message string
	// Details optionally carries machine-readable context (e.g.
	// validation failures). It is exposed in the JSON body when set.
	Details any
}

// Error implements error.
func (e *HttpError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Status)
}

// StatusCode implements router.Statuser.
func (e *HttpError) StatusCode() int { return e.Status }

// NewHttpError builds an HttpError with an optional details payload.
func NewHttpError(status int, message string, details ...any) *HttpError {
	e := &HttpError{Status: status, Message: message}
	if len(details) > 0 {
		e.Details = details[0]
	}
	return e
}

// BadRequest builds a 400 error.
func BadRequest(msg string, details ...any) *HttpError {
	return NewHttpError(http.StatusBadRequest, msg, details...)
}

// Unauthorized builds a 401 error.
func Unauthorized(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "unauthorized"
	}
	return NewHttpError(http.StatusUnauthorized, msg, details...)
}

// Forbidden builds a 403 error.
func Forbidden(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "forbidden"
	}
	return NewHttpError(http.StatusForbidden, msg, details...)
}

// NotFound builds a 404 error.
func NotFound(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "not found"
	}
	return NewHttpError(http.StatusNotFound, msg, details...)
}

// Conflict builds a 409 error.
func Conflict(msg string, details ...any) *HttpError {
	return NewHttpError(http.StatusConflict, msg, details...)
}

// Unprocessable builds a 422 error, used for validation failures.
func Unprocessable(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "validation failed"
	}
	return NewHttpError(http.StatusUnprocessableEntity, msg, details...)
}

// Internal builds a 500 error. Prefer letting unexpected failures fall
// through as plain errors; the router already maps those to 500.
func Internal(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "internal server error"
	}
	return NewHttpError(http.StatusInternalServerError, msg, details...)
}

// AsHttpError unwraps err to an *HttpError when possible.
func AsHttpError(err error) (*HttpError, bool) {
	var he *HttpError
	if errors.As(err, &he) {
		return he, true
	}
	return nil, false
}
