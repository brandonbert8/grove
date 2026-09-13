package middleware

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/logger"
	"github.com/brandonbert8/grove/packages/router"
)

// hijackableWriter simulates a websocket-capable server writer.
type hijackableWriter struct {
	http.ResponseWriter
	hijacked bool
}

func (w *hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	return nil, nil, nil
}

func TestLoggingFlushPassthrough(t *testing.T) {
	r := router.New()
	r.Use(Logging(logger.Discard()))
	r.GET("/stream", func(c router.Context) error {
		f, ok := c.ResponseWriter().(http.Flusher)
		if !ok {
			t.Error("ResponseWriter behind Logging must implement http.Flusher")
			return c.NoContent(500)
		}
		if err := c.String(200, "chunk"); err != nil {
			return err
		}
		f.Flush()
		return nil
	})
	req := httptest.NewRequest("GET", "/stream", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "chunk" || !rec.Flushed {
		t.Fatalf("stream = %d %q flushed=%v", rec.Code, rec.Body.String(), rec.Flushed)
	}
}

func TestLoggingHijackPassthrough(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: &hijackableWriter{ResponseWriter: httptest.NewRecorder()}}
	h, ok := any(rec).(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder must implement http.Hijacker")
	}
	if _, _, err := h.Hijack(); err != nil {
		t.Fatalf("hijack must delegate, got %v", err)
	}

	plain := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	if _, _, err := any(plain).(http.Hijacker).Hijack(); err == nil {
		t.Fatal("hijack without underlying support must error, not panic")
	}
}

func TestCORSExposeHeaders(t *testing.T) {
	r := router.New()
	cfg := DefaultCORSConfig()
	cfg.ExposeHeaders = []string{"X-Request-ID", "X-Total-Count"}
	r.Use(CORS(cfg))
	r.GET("/x", func(c router.Context) error { return c.NoContent(204) })

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "X-Request-ID, X-Total-Count" {
		t.Fatalf("expose = %q", got)
	}
}

func TestRateLimitIgnoresSpoofedXFFByDefault(t *testing.T) {
	r := router.New()
	r.Use(RateLimit(1000, 1)) // burst 1: second request from same TCP peer 429s
	r.GET("/x", func(c router.Context) error { return c.NoContent(204) })
	do := func(xff string) int {
		req := httptest.NewRequest("GET", "/x", nil)
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := do(""); code != 204 {
		t.Fatalf("first = %d", code)
	}
	// Spoofing a fresh XFF must NOT buy a new bucket.
	if code := do("9.9.9.9"); code != http.StatusTooManyRequests {
		t.Fatalf("spoofed XFF = %d, want 429", code)
	}
}

func TestRateLimitTrustProxySeparatesClients(t *testing.T) {
	r := router.New()
	r.Use(RateLimitWithConfig(RateLimitConfig{RPS: 1000, Burst: 1, TrustProxy: true}))
	r.GET("/x", func(c router.Context) error { return c.NoContent(204) })
	do := func(xff string) int {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := do("10.0.0.1"); code != 204 {
		t.Fatalf("client A = %d", code)
	}
	if code := do("10.0.0.2"); code != 204 {
		t.Fatalf("client B must have its own bucket, got %d", code)
	}
}

func TestRateLimitCapFailOpen(t *testing.T) {
	r := router.New()
	r.Use(RateLimitWithConfig(RateLimitConfig{RPS: 1, Burst: 1, MaxClients: 1}))
	r.GET("/x", func(c router.Context) error { return c.NoContent(204) })
	do := func(remote string) int {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := do("10.0.0.1:1"); code != 204 {
		t.Fatalf("first client = %d", code)
	}
	// Cap reached: unknown clients pass untracked instead of growing memory.
	if code := do("10.0.0.2:1"); code != 204 {
		t.Fatalf("over-cap client must fail open, got %d", code)
	}
}
