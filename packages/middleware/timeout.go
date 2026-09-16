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
			setRequest(c, c.Request().WithContext(ctx))

			tw := &timeoutWriter{w: c.ResponseWriter(), h: make(http.Header)}
			wc := &writerSwapContext{Context: c, writer: tw}
			done := make(chan error, 1)
			go func() { done <- next(wc) }()

			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				tw.expire()
				// If the handler already committed a response, the
				// timeout body cannot replace it: drop it and let the
				// committed bytes stand (no double-write corruption).
				if tw.didWrite() {
					<-done // handler already finished writing; no leak (buffered)
					return nil
				}
				tw.w.Header().Set("Content-Type", "application/json")
				tw.w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = tw.w.Write([]byte(`{"error":"request timeout","status":503}`))
				// done is buffered (cap 1): the straggler's send never
				// blocks, so no drain goroutine is needed and none leaks.
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

// didWrite reports whether the underlying response was committed.
func (t *timeoutWriter) didWrite() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.wrote
}

// Wrote reports whether bytes were committed (for Recovery checks).
func (t *timeoutWriter) Wrote() bool { return t.didWrite() }

// setRequest swaps the request on contexts supporting it, falling back
// to in-place mutation for foreign Context implementations.
func setRequest(c router.Context, r *http.Request) {
	if sr, ok := c.(interface{ SetRequest(*http.Request) }); ok {
		sr.SetRequest(r)
		return
	}
	*c.Request() = *r
}
