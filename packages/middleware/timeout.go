package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/brandonbert8/grove/packages/router"
)

// Timeout bounds handler execution: when d elapses before the handler
// returns, the client gets a JSON 503 and late handler writes are
// dropped. A non-positive d disables the middleware (passthrough).
//
// Unlike net/http.TimeoutHandler the timeout body is Grove-shaped JSON,
// and unlike a bare goroutine + select the response writer is
// synchronized, so -race stays green when the straggler writes late.
// Caveat: the writer is not an http.Hijacker/Flusher — keep Timeout off
// SSE/hijack routes and scope it per group or route instead of global.
func Timeout(d time.Duration) router.Middleware {
	if d <= 0 {
		return func(next router.HandlerFunc) router.HandlerFunc { return next }
	}
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			ctx, cancel := context.WithTimeout(c.Request().Context(), d)
			defer cancel()
			*c.Request() = *c.Request().WithContext(ctx)

			tw := &timeoutWriter{w: c.ResponseWriter(), h: make(http.Header)}
			wc := &writerSwapContext{Context: c, writer: tw}
			done := make(chan error, 1)
			go func() { done <- next(wc) }()

			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				tw.expire()
				tw.w.Header().Set("Content-Type", "application/json")
				tw.w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = tw.w.Write([]byte(`{"error":"request timeout","status":503}`))
				// Drain so a same-process test goroutine never leaks;
				// the production server does not depend on this.
				go func() { <-done }()
				return nil
			}
		}
	}
}

// timeoutWriter buffers handler headers privately and drops everything
// after expiry under a mutex, so a late handler can neither corrupt
// the timeout response nor race the test/server reader.
type timeoutWriter struct {
	mu      sync.Mutex
	w       http.ResponseWriter
	h       http.Header
	wrote   bool
	expire_ bool
}

func (t *timeoutWriter) Header() http.Header { return t.h }

func (t *timeoutWriter) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.expire_ {
		return len(b), nil
	}
	if !t.wrote {
		t.copyHeadersLocked()
		t.w.WriteHeader(http.StatusOK)
		t.wrote = true
	}
	return t.w.Write(b)
}

func (t *timeoutWriter) WriteHeader(code int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.expire_ || t.wrote {
		return
	}
	t.copyHeadersLocked()
	t.w.WriteHeader(code)
	t.wrote = true
}

// copyHeadersLocked moves buffered headers to the underlying writer.
// Callers hold t.mu.
func (t *timeoutWriter) copyHeadersLocked() {
	dst := t.w.Header()
	for k, vs := range t.h {
		dst[k] = vs
	}
}

func (t *timeoutWriter) expire() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.expire_ = true
}
