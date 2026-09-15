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
// Responses buffer in memory: use it for JSON APIs, not for streams,
// SSE, or file downloads.
func MapResponse(mapper func(status int, body []byte) (int, []byte)) router.Middleware {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			cw := &captureWriter{header: c.ResponseWriter().Header(), status: http.StatusOK}
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
			w.Header().Set("Content-Length", "")
			w.WriteHeader(status)
			_, _ = w.Write(body)
			return nil
		}
	}
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

// captureWriter buffers one response (status + body) for mapping.
type captureWriter struct {
	header      http.Header
	status      int
	body        bytes.Buffer
	wrote       bool
	wroteHeader bool
}

// Header returns the shared header map.
func (w *captureWriter) Header() http.Header { return w.header }

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
func (w *captureWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.wrote = true
	return w.body.Write(b)
}

// flushTo replays the captured response untouched.
func (w *captureWriter) flushTo(dst http.ResponseWriter) {
	if !w.wrote {
		return
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
