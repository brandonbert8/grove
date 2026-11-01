package httperr

import (
	"errors"
	"fmt"
	"net/http"
)

// HttpError is a typed HTTP error: status, stable machine-readable code,
// human message, optional details, and an optional wrapped cause.
//
// Handlers return it directly; the router maps it through the Statuser
// contract and renders {error,status,code?,details?}. Wrap preserves the
// cause for errors.Is / errors.As and logs without changing the response.
type HttpError struct {
	Status  int
	Message string
	Code    string
	Details any
	Cause   error
}

func (e *HttpError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Status)
}

func (e *HttpError) StatusCode() int { return e.Status }

// ErrorCode returns the stable machine-readable code, if any.
func (e *HttpError) ErrorCode() string {
	if e == nil {
		return ""
	}
	return e.Code
}

// Unwrap returns the wrapped cause.
func (e *HttpError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// WithCode sets the machine-readable code and returns e for chaining:
//
//	return httperr.NotFound("user not found").WithCode("USER_NOT_FOUND")
func (e *HttpError) WithCode(code string) *HttpError {
	if e != nil {
		e.Code = code
	}
	return e
}

// WithCause attaches the underlying error and returns e for chaining.
func (e *HttpError) WithCause(err error) *HttpError {
	if e != nil {
		e.Cause = err
	}
	return e
}

// WithDetails sets the machine-readable details payload and returns e
// for chaining.
func (e *HttpError) WithDetails(details any) *HttpError {
	if e != nil {
		e.Details = details
	}
	return e
}

func New(status int, message string, details ...any) *HttpError {
	e := &HttpError{Status: status, Message: message}
	if len(details) > 0 {
		e.Details = details[0]
	}
	return e
}

// NewCode builds an HttpError with a stable machine-readable code.
func NewCode(status int, code, message string, details ...any) *HttpError {
	e := New(status, message, details...)
	e.Code = code
	return e
}

// Wrap builds an HttpError that preserves err as its cause. It returns
// nil when err is nil, so it composes cleanly in handler returns.
func Wrap(err error, status int, message string, details ...any) *HttpError {
	if err == nil {
		return nil
	}
	return New(status, message, details...).WithCause(err)
}

// Wrapf is Wrap with a formatted message.
func Wrapf(err error, status int, format string, args ...any) *HttpError {
	if err == nil {
		return nil
	}
	return Wrap(err, status, fmt.Sprintf(format, args...))
}

func BadRequest(msg string, details ...any) *HttpError {
	return New(http.StatusBadRequest, msg, details...)
}

func Unauthorized(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "unauthorized"
	}
	return New(http.StatusUnauthorized, msg, details...)
}

func Forbidden(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "forbidden"
	}
	return New(http.StatusForbidden, msg, details...)
}

func NotFound(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "not found"
	}
	return New(http.StatusNotFound, msg, details...)
}

func Conflict(msg string, details ...any) *HttpError {
	return New(http.StatusConflict, msg, details...)
}

func Unprocessable(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "validation failed"
	}
	return New(http.StatusUnprocessableEntity, msg, details...)
}

func Internal(msg string, details ...any) *HttpError {
	if msg == "" {
		msg = "internal server error"
	}
	return New(http.StatusInternalServerError, msg, details...)
}

func (e *HttpError) ErrorDetails() any {
	if e == nil {
		return nil
	}
	return e.Details
}

func As(err error) (*HttpError, bool) {
	var he *HttpError
	if errors.As(err, &he) {
		return he, true
	}
	return nil, false
}
