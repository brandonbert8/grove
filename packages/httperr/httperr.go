package httperr

import (
	"errors"
	"net/http"
)

type HttpError struct {
	Status  int
	Message string
	Details any
}

func (e *HttpError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Status)
}

func (e *HttpError) StatusCode() int { return e.Status }

func New(status int, message string, details ...any) *HttpError {
	e := &HttpError{Status: status, Message: message}
	if len(details) > 0 {
		e.Details = details[0]
	}
	return e
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
