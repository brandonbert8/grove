package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/brandonbert8/grove/packages/router"
)

func serveMW(t *testing.T, r *router.DefaultRouter, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRequestIDMintAndEcho(t *testing.T) {
	r := router.New()
	r.Use(RequestID())
	r.GET("/x", func(c router.Context) error {
		if GetRequestID(c) == "" {
			t.Error("empty request id inside handler")
		}
		return c.NoContent(204)
	})
	rec := serveMW(t, r, "GET", "/x", nil)
	id := rec.Header().Get(RequestIDHeader)
	if len(id) < 16 {
		t.Fatalf("X-Request-ID = %q, want minted id", id)
	}
}

func TestRequestIDReusesInbound(t *testing.T) {
	r := router.New()
	r.Use(RequestID())
	var seen string
	r.GET("/x", func(c router.Context) error {
		seen = GetRequestID(c)
		return c.NoContent(204)
	})
	rec := serveMW(t, r, "GET", "/x", map[string]string{RequestIDHeader: "abc-123"})
	if rec.Header().Get(RequestIDHeader) != "abc-123" || seen != "abc-123" {
		t.Fatalf("id not reused: header=%q seen=%q", rec.Header().Get(RequestIDHeader), seen)
	}
}

func TestTimeoutPassesFastHandler(t *testing.T) {
	r := router.New()
	r.Use(Timeout(2 * time.Second))
	r.GET("/fast", func(c router.Context) error { return c.String(200, "fast") })
	rec := serveMW(t, r, "GET", "/fast", nil)
	if rec.Code != 200 || rec.Body.String() != "fast" {
		t.Fatalf("fast = %d %q", rec.Code, rec.Body.String())
	}
}

func TestTimeoutReturnsJSON503(t *testing.T) {
	r := router.New()
	r.Use(Timeout(20 * time.Millisecond))
	release := make(chan struct{})
	r.GET("/slow", func(c router.Context) error {
		<-release // hold the handler past the deadline, then write late
		return c.String(200, "too late")
	})
	rec := serveMW(t, r, "GET", "/slow", nil)
	close(release)
	time.Sleep(50 * time.Millisecond) // let the straggler write (race-checked)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), "request timeout") {
		t.Fatalf("body = %q, want timeout JSON", rec.Body.String())
	}
}

func TestSecureHeadersPlainHTTP(t *testing.T) {
	r := router.New()
	r.Use(SecureHeaders())
	r.GET("/x", func(c router.Context) error { return c.NoContent(204) })
	rec := serveMW(t, r, "GET", "/x", nil)
	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if rec.Header().Get(k) != want {
			t.Fatalf("%s = %q, want %q", k, rec.Header().Get(k), want)
		}
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS must not pin plain-HTTP dev traffic")
	}
}

func TestRateLimitAllowsBurstThen429(t *testing.T) {
	r := router.New()
	r.Use(RateLimit(1000, 2)) // fast refill, burst of 2
	r.GET("/x", func(c router.Context) error { return c.NoContent(204) })
	h := map[string]string{"X-Forwarded-For": "10.9.8.7"}
	if rec := serveMW(t, r, "GET", "/x", h); rec.Code != 204 {
		t.Fatalf("first = %d, want 204", rec.Code)
	}
	if rec := serveMW(t, r, "GET", "/x", h); rec.Code != 204 {
		t.Fatalf("second = %d, want 204", rec.Code)
	}
	rec := serveMW(t, r, "GET", "/x", h)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After on 429")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("429 Content-Type = %q, want application/json", ct)
	}
}

func TestRateLimitDisabledPassthrough(t *testing.T) {
	r := router.New()
	r.Use(RateLimit(0, 0))
	r.GET("/x", func(c router.Context) error { return c.NoContent(204) })
	for i := 0; i < 5; i++ {
		if rec := serveMW(t, r, "GET", "/x", nil); rec.Code != 204 {
			t.Fatalf("req %d = %d, want 204", i, rec.Code)
		}
	}
}
