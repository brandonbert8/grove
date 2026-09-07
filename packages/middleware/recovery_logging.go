package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/brandonbert8/grove/packages/logger"
	"github.com/brandonbert8/grove/packages/router"
)

// Recovery converts panics in handlers into JSON 500 responses and logs
// the panic value. Place it outermost so it also covers Logging and CORS.
func Recovery(log logger.Logger) router.Middleware {
	if log == nil {
		log = logger.Discard()
	}
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					log.Error("panic recovered", "panic", fmt.Sprintf("%v", r))
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
			rec := &statusRecorder{ResponseWriter: c.ResponseWriter(), status: http.StatusOK}
			// Swap the writer for the duration of the request so the
			// status code can be observed without changing the Context API.
			origCtx := c
			wrapped := &writerSwapContext{Context: origCtx, writer: rec}
			err := next(wrapped)
			status := rec.status
			if err != nil && !rec.wrote {
				status = http.StatusInternalServerError
			}
			log.Info("request",
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
			return err
		}
	}
}

// writerSwapContext overrides only the ResponseWriter.
type writerSwapContext struct {
	router.Context
	writer http.ResponseWriter
}

// ResponseWriter returns the recording writer.
func (c *writerSwapContext) ResponseWriter() http.ResponseWriter { return c.writer }
