package grove

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

// TestHealthModuleWithChecksHungCheckDoesNotHang verifies that a check
// that ignores its context (and thus never completes) does not hang the
// handler: the probe returns 503 with "unknown" for the hung check.
// This guards against the goroutine leak where a background drain
// goroutine blocked forever waiting on a hung check's channel write.
func TestHealthModuleWithChecksHungCheckDoesNotHang(t *testing.T) {
	app := New()
	app.MustRegister(HealthModuleWithChecks("", 50*time.Millisecond, map[string]Check{
		"db": func(ctx context.Context) error {
			// Intentionally ignore ctx and block longer than the timeout.
			time.Sleep(time.Second)
			return nil
		},
	}).AsModule())
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		app.Handler().ServeHTTP(rec, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler hung: hung check blocked the probe")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body := rec.Body.String(); !contains(body, `"db":"unknown"`) {
		t.Fatalf("hung check should be unknown, body = %q", body)
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
