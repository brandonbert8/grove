package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/brandonbert8/grove/packages/logger"
	"github.com/brandonbert8/grove/packages/router"
)

// Recovery converts panics in handlers into JSON 500 responses and logs
// the panic value with its stack trace. Place it outermost so it also
// covers Logging and CORS.
//
// Inherent net/http limit: if the handler already wrote headers before
// panicking, the 500 body cannot replace the committed response — the
// write below is then best-effort and the stack log is the signal.
func Recovery(log logger.Logger) router.Middleware {
	if log == nil {
		log = logger.Discard()
	}
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					log.Error("panic recovered",
						"panic", fmt.Sprintf("%v", r),
						"request_id", GetRequestID(c),
						"stack", string(debug.Stack()),
					)
					// If headers were already committed to the wire, a
					// 500 body would be a corrupting double-write: log
					// only. If the body is buffered (captureWriter),
					// replace it with the 500 envelope instead.
					if rw, ok := c.ResponseWriter().(interface {
						Wrote() bool
						Reset()
					}); ok && rw.Wrote() {
						err = nil
						return
					}
					if rw, ok := c.ResponseWriter().(interface{ Reset() }); ok {
						rw.Reset()
					}
					err = c.JSON(http.StatusInternalServerError, map[string]string{
						"error": "internal server error",
					})
				}
			}()
			return next(c)
		}
	}
}

// Logging records method, path, status, and latency per request.
// Skip can suppress noisy routes such as health checks.
func Logging(log logger.Logger, skip ...func(r *http.Request) bool) router.Middleware {
	if log == nil {
		log = logger.Discard()
	}
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			for _, fn := range skip {
				if fn != nil && fn(c.Request()) {
					return next(c)
				}
			}
			start := time.Now()
			// One allocation carries both the Context override and the
			// status-capturing writer.
			wrapped := &loggingContext{Context: c}
			wrapped.statusRecorder.ResponseWriter = c.ResponseWriter()
			wrapped.status = http.StatusOK
			err := next(wrapped)
			status := wrapped.status
			if err != nil && !wrapped.wrote {
				status = http.StatusInternalServerError
			}
			fields := make([]any, 0, 10)
			fields = append(fields,
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
			// Correlate with RequestID when it ran (outermost-global
			// ordering puts RequestID before Logging).
			if id := GetRequestID(c); id != "" {
				fields = append(fields, "request_id", id)
			}
			log.Info("request", fields...)
			return err
		}
	}
}

// loggingContext combines the Context override and status recorder in one
// per-request allocation.
type loggingContext struct {
	router.Context
	statusRecorder
}

// ResponseWriter returns the recording writer.
func (c *loggingContext) ResponseWriter() http.ResponseWriter { return &c.statusRecorder }

// SetRequest forwards to the underlying context when it supports it,
// falling back to in-place mutation (matching setRequest semantics).
func (c *loggingContext) SetRequest(r *http.Request) {
	if sr, ok := c.Context.(interface{ SetRequest(*http.Request) }); ok {
		sr.SetRequest(r)
		return
	}
	*c.Context.Request() = *r
}

// writerSwapContext overrides only the ResponseWriter.
type writerSwapContext struct {
	router.Context
	writer http.ResponseWriter
}

// ResponseWriter returns the recording writer.
func (c *writerSwapContext) ResponseWriter() http.ResponseWriter { return c.writer }

// SetRequest forwards to the underlying context when it supports it,
// falling back to in-place mutation (matching setRequest semantics).
func (c *writerSwapContext) SetRequest(r *http.Request) {
	if sr, ok := c.Context.(interface{ SetRequest(*http.Request) }); ok {
		sr.SetRequest(r)
		return
	}
	*c.Context.Request() = *r
}
