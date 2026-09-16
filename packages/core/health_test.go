package grove

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthModuleWithChecksOK(t *testing.T) {
	app := New()
	app.MustRegister(HealthModuleWithChecks("", 0, map[string]Check{
		"db": func(ctx context.Context) error { return nil },
	}).AsModule())
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "{\"checks\":{\"db\":\"ok\"},\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestHealthModuleWithChecksDegraded(t *testing.T) {
	app := New()
	app.MustRegister(HealthModuleWithChecks("/ping", 0, map[string]Check{
		"db":    func(ctx context.Context) error { return nil },
		"queue": func(ctx context.Context) error { return errors.New("dial timeout") },
	}).AsModule())
	req := httptest.NewRequest("GET", "/ping", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	body := rec.Body.String()
	if body != "{\"checks\":{\"db\":\"ok\",\"queue\":\"fail\"},\"status\":\"degraded\"}\n" {
		t.Fatalf("body = %q", body)
	}
}
