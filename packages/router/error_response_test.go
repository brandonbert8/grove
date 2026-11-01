package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

type codedError struct {
	status int
	code   string
}

func (e codedError) Error() string     { return "coded" }
func (e codedError) StatusCode() int   { return e.status }
func (e codedError) ErrorCode() string { return e.code }

type detailedError struct {
	codedError
	details any
}

func (e detailedError) ErrorDetails() any { return e.details }

func TestErrorResponseIncludesCodeAndDetails(t *testing.T) {
	r := New()
	r.GET("/coded", func(Context) error {
		return detailedError{
			codedError: codedError{status: http.StatusNotFound, code: "USER_NOT_FOUND"},
			details:    map[string]string{"id": "42"},
		}
	})
	rec := doRequest(t, r, "GET", "/coded")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "coded" || body.Status != 404 || body.Code != "USER_NOT_FOUND" {
		t.Fatalf("body = %+v", body)
	}
	d, ok := body.Details.(map[string]any)
	if !ok || d["id"] != "42" {
		t.Fatalf("details = %+v", body.Details)
	}
}

func TestErrorResponseOmitsEmptyCode(t *testing.T) {
	r := New()
	r.GET("/plain", func(Context) error { return errors.New("boom") })
	rec := doRequest(t, r, "GET", "/plain")
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["code"]; ok {
		t.Fatalf("unexpected code: %v", raw)
	}
	if raw["error"] != "internal server error" {
		t.Fatalf("error = %v", raw["error"])
	}
}

func TestNotFoundUsesErrorEnvelope(t *testing.T) {
	r := New()
	rec := doRequest(t, r, "GET", "/missing")
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != http.StatusNotFound || body.Error != "not found" {
		t.Fatalf("body = %+v", body)
	}
}
