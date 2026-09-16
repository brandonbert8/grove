package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"

	"github.com/brandonbert8/grove/packages/logger"
	"github.com/brandonbert8/grove/packages/router"
)

// DefaultChain returns the stock NestJS-style middleware stack:
// Recovery outermost, then RequestID, Logging, and SecureHeaders.
// It is the one-line equivalent of wiring the four by hand:
//
//	app.Router.Use(middleware.DefaultChain(app.Logger)...)
//
// Place it first in Use so RequestID correlates every log line.
func DefaultChain(log logger.Logger) []router.Middleware {
	return []router.Middleware{
		Recovery(log),
		RequestID(),
		Logging(log),
		SecureHeaders(),
	}
}

// statusRecorder captures the status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
		r.ResponseWriter.WriteHeader(code)
	}
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// Wrote reports whether headers were committed.
func (r *statusRecorder) Wrote() bool { return r.wrote }

// Flush implements http.Flusher so SSE and streaming handlers keep
// working behind Logging: the implicit 200 is committed first, then
// the flush is forwarded.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		if !r.wrote {
			r.WriteHeader(http.StatusOK)
		}
		f.Flush()
	}
}

// Hijack implements http.Hijacker so websockets keep working behind
// Logging, forwarding to the underlying writer.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("middleware: underlying writer does not support hijacking")
}
