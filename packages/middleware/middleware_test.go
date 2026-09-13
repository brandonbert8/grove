package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/logger"
	"github.com/brandonbert8/grove/packages/router"
)

func TestRecoveryConvertsPanicToJSON500(t *testing.T) {
	r := router.New()
	log := logger.Discard()
	r.Use(Recovery(log))
	r.GET("/boom", func(c router.Context) error { panic("kaboom") })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatal("missing Content-Type on panic response")
	}
}

func TestCORSPreflight(t *testing.T) {
	r := router.New()
	r.Use(CORS(DefaultCORSConfig()))
	r.GET("/hello", func(c router.Context) error { return c.String(200, "hi") })

	req := httptest.NewRequest(http.MethodOptions, "/hello", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("missing Access-Control-Allow-Origin on preflight")
	}
}

func TestLoggingPassesThrough(t *testing.T) {
	r := router.New()
	r.Use(Logging(logger.Discard()))
	r.GET("/ok", func(c router.Context) error { return c.String(200, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("GET /ok = %d %q, want 200 ok", rec.Code, rec.Body.String())
	}
}
