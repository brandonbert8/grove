package httperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestNewCodeAndChain(t *testing.T) {
	err := NewCode(http.StatusNotFound, "USER_NOT_FOUND", "missing").WithDetails(nil)
	if err.StatusCode() != 404 || err.ErrorCode() != "USER_NOT_FOUND" || err.Error() != "missing" {
		t.Fatalf("err = %+v", err)
	}
	err.WithCode("USER_GONE")
	if err.ErrorCode() != "USER_GONE" {
		t.Fatalf("code = %q", err.ErrorCode())
	}
}

func TestWrapPreservesCause(t *testing.T) {
	cause := errors.New("db down")
	err := Wrap(cause, http.StatusServiceUnavailable, "try later")
	if err == nil {
		t.Fatal("Wrap() = nil")
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is did not find cause")
	}
	if err.StatusCode() != http.StatusServiceUnavailable || err.Error() != "try later" {
		t.Fatalf("err = %+v", err)
	}
	if Wrap(nil, 500, "x") != nil {
		t.Fatal("Wrap(nil) must be nil")
	}
}

func TestWrapf(t *testing.T) {
	cause := fmt.Errorf("inner")
	err := Wrapf(cause, 422, "bad %s", "field")
	if err.Error() != "bad field" || !errors.Is(err, cause) {
		t.Fatalf("err = %+v", err)
	}
}

func TestErrorFallsBackToStatusText(t *testing.T) {
	if got := (&HttpError{Status: 418}).Error(); got != http.StatusText(418) {
		t.Fatalf("Error() = %q", got)
	}
}

func TestAs(t *testing.T) {
	base := NotFound("nope").WithCode("NOT_FOUND")
	wrapped := fmt.Errorf("outer: %w", base)
	got, ok := As(wrapped)
	if !ok || got.ErrorCode() != "NOT_FOUND" {
		t.Fatalf("As() = %+v, %v", got, ok)
	}
	if _, ok := As(errors.New("plain")); ok {
		t.Fatal("As(plain) = true")
	}
}

func TestErrorDetails(t *testing.T) {
	err := BadRequest("bad", map[string]string{"field": "name"})
	d, ok := err.ErrorDetails().(map[string]string)
	if !ok || d["field"] != "name" {
		t.Fatalf("details = %+v", err.ErrorDetails())
	}
	var nilErr *HttpError
	if nilErr.ErrorDetails() != nil {
		t.Fatal("nil ErrorDetails() != nil")
	}
}
