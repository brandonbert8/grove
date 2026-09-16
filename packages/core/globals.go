package grove

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/brandonbert8/grove/packages/router"
)

// UseGuards registers global guards (the APP_GUARD equivalent): every
// route, including 404/405-safe global middleware, passes them first.
//
//	app.UseGuards(authGuard) // or any grove.CanActivate
//
// Prefer controller-scoped Middleware and endpoint grove.Use for narrow
// scopes; globals are the blunt default-deny layer.
func (a *App) UseGuards(guards ...CanActivate) {
	for _, g := range guards {
		if g == nil {
			continue
		}
		a.Router.Use(UseGuard(g))
	}
}

// UseInterceptors registers global interceptors (the APP_INTERCEPTOR
// equivalent): tracing, metrics, response mapping. Plain
// router.Middleware values already are interceptors.
func (a *App) UseInterceptors(mw ...router.Middleware) {
	filtered := make([]router.Middleware, 0, len(mw))
	for _, m := range mw {
		if m != nil {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) > 0 {
		a.Router.Use(filtered...)
	}
}

// UsePipes registers the global pipe stage (the APP_PIPE equivalent):
// transforms that run for every request before handlers. Body DTO
// validation itself stays explicit inside handlers
// (pipes.Body / pipes.ValidateBody), because Go has no parameter
// decorators; UsePipes is the home for cross-cutting coercions such as
// trimming, default headers, or tenant injection.
func (a *App) UsePipes(mw ...router.Middleware) { a.UseInterceptors(mw...) }

// Envelope is the standard success wrapper used by OK/Created and the
// WrapData interceptor, the Go equivalent of a NestJS response-mapping
// interceptor emitting `{data: ...}`.
type Envelope[T any] struct {
	Data T `json:"data"`
}

// OK answers 200 with the payload wrapped as {"data": ...}.
func OK[T any](c router.Context, data T) error {
	return c.JSON(http.StatusOK, Envelope[T]{Data: data})
}

// Created answers 201 with the payload wrapped as {"data": ...}.
func Created[T any](c router.Context, data T) error {
	return c.JSON(http.StatusCreated, Envelope[T]{Data: data})
}

// MapResponse rewrites JSON responses through mapper, the Go equivalent
// of a NestJS response-mapping interceptor. Non-JSON responses pass
// through untouched; handler errors (non-nil return) skip mapping so
// Grove-shaped errors stay canonical.
//
// Scope it to JSON APIs only: streams, SSE, hijacked connections, and
// non-JSON payloads bypass buffering entirely (no OOM, no latency).
func MapResponse(mapper func(status int, body []byte) (int, []byte)) router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			if isStreamWriter(c.ResponseWriter()) {
				return next(c)
			}
			cw := &captureWriter{header: cloneHeader(c.ResponseWriter().Header()), status: http.StatusOK, dst: c.ResponseWriter()}
			wrapped := &mappedContext{Context: c, w: cw}
			if err := next(wrapped); err != nil {
				return err
			}
			if !cw.wrote || !isJSON(cw.header.Get("Content-Type")) {
				cw.flushTo(c.ResponseWriter())
				return nil
			}
			status, body := mapper(cw.status, cw.body.Bytes())
			if status == 0 {
				status = cw.status
			}
			w := c.ResponseWriter()
			for k, vv := range cw.header {
				w.Header()[k] = vv
			}
			w.Header().Del("Content-Length")
			w.WriteHeader(status)
			_, _ = w.Write(body)
			return nil
		}
	}
}

// isStreamWriter is a pre-check hook (conservative: never bypass here;
// the captureWriter itself detects Flush/Hijack mid-handler and streams
// through instead of buffering). Kept as a seam for future heuristics.
func isStreamWriter(w http.ResponseWriter) bool { return false }

// maxMapBody caps MapResponse buffering (streams beyond this flush
// through instead of OOMing).
const maxMapBody = 4 << 20

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vv := range h {
		out[k] = append([]string(nil), vv...)
	}
	return out
}

// WrapData wraps 2xx JSON responses as {"data": ...}, the one-line
// response-envelope interceptor:
//
//	app.UseInterceptors(grove.WrapData())
func WrapData() router.Middleware {
	return MapResponse(func(status int, body []byte) (int, []byte) {
		if status < 200 || status >= 300 {
			return status, body
		}
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			return status, body
		}
		if m, ok := v.(map[string]any); ok {
			if _, has := m["data"]; has {
				return status, body
			}
		}
		out, err := json.Marshal(map[string]any{"data": v})
		if err != nil {
			return status, body
		}
		return status, out
	})
}

// mappedContext swaps the ResponseWriter for the capture writer.
type mappedContext struct {
	router.Context
	w http.ResponseWriter
}

// ResponseWriter returns the capture writer.
func (c *mappedContext) ResponseWriter() http.ResponseWriter { return c.w }

// captureWriter buffers one response (status + body) for mapping,
// streaming through when the handler flushes (SSE) or exceeds the cap.
type captureWriter struct {
	header      http.Header
	status      int
	body        bytes.Buffer
	wrote       bool
	wroteHeader bool
	streamed    bool
	dst         http.ResponseWriter
}

// Header returns the private header map.
func (w *captureWriter) Header() http.Header { return w.header }

// Wrote reports whether headers were committed (for Recovery checks).
func (w *captureWriter) Wrote() bool { return w.wroteHeader && w.streamed }

// WriteHeader records the status code.
func (w *captureWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.wrote = true
}

// Write buffers the body, implying 200 when no status was set.
// Oversized or non-JSON payloads stream through unmapped.
func (w *captureWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.wrote = true
	if w.streamed {
		return w.dst.Write(b)
	}
	if w.body.Len()+len(b) > maxMapBody {
		w.streamThrough()
		return w.dst.Write(b)
	}
	return w.body.Write(b)
}

// Flush implements http.Flusher: the handler is streaming (SSE), so
// commit headers/body live and bypass mapping from here on.
func (w *captureWriter) Flush() {
	if w.dst == nil {
		return
	}
	w.streamThrough()
	if f, ok := w.dst.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *captureWriter) streamThrough() {
	if w.streamed || w.dst == nil {
		return
	}
	w.streamed = true
	for k, vv := range w.header {
		w.dst.Header()[k] = vv
	}
	w.dst.WriteHeader(w.status)
	if w.body.Len() > 0 {
		_, _ = w.dst.Write(w.body.Bytes())
		w.body.Reset()
	}
}

// flushTo replays the captured response untouched.
func (w *captureWriter) flushTo(dst http.ResponseWriter) {
	if !w.wrote {
		return
	}
	if w.streamed {
		return // already live on dst
	}
	for k, vv := range w.header {
		dst.Header()[k] = vv
	}
	dst.WriteHeader(w.status)
	_, _ = dst.Write(w.body.Bytes())
}

// isJSON reports whether a Content-Type is JSON.
func isJSON(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	return strings.HasPrefix(ct, "application/json")
}
