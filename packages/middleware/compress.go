package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"

	"github.com/brandonbert8/grove/packages/router"
)

// Compress gzips responses when the client sends Accept-Encoding: gzip.
// It skips event-streams, websockets, and already-encoded payloads, so
// SSE/file downloads keep streaming instead of buffering.
func Compress(level ...int) router.Middleware {
	lvl := gzip.DefaultCompression
	if len(level) > 0 && level[0] >= gzip.NoCompression && level[0] <= gzip.BestCompression {
		lvl = level[0]
	}
	return router.FromHTTP(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}
			if isStream(r) {
				next.ServeHTTP(w, r)
				return
			}
			gz, err := gzip.NewWriterLevel(w, lvl)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			cw := &compressWriter{ResponseWriter: w, w: gz}
			w.Header().Set("Vary", varyValue(w.Header().Get("Vary")))
			next.ServeHTTP(cw, r)
			// Compress only successful bodies with content: 204/304/HEAD
			// and pre-encoded payloads pass through untouched.
			cw.finish()
			if cw.gzUsed {
				_ = gz.Close()
			}
		})
	})
}

func isStream(r *http.Request) bool {
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		return true
	}
	if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
		return true
	}
	return false
}

func varyValue(v string) string {
	if v == "" {
		return "Accept-Encoding"
	}
	if strings.Contains(v, "Accept-Encoding") {
		return v
	}
	return v + ", Accept-Encoding"
}

// compressWriter delays Content-Encoding until the response proves
// compressible: 2xx with a body, non-stream content, no existing
// encoding. Flush/Hijack pass through so SSE/websockets survive.
type compressWriter struct {
	http.ResponseWriter
	w       *gzip.Writer
	status  int
	wrote   bool
	started bool
	gzUsed  bool
}

func (g *compressWriter) WriteHeader(code int) {
	if g.wrote {
		return
	}
	g.wrote = true
	g.status = code
	// Don't commit yet: Write decides encoding once Content-Type is known.
}

func (g *compressWriter) ensureStarted() {
	if g.started {
		return
	}
	g.started = true
	h := g.ResponseWriter.Header()
	ct := h.Get("Content-Type")
	enc := h.Get("Content-Encoding")
	status := g.status
	if status == 0 {
		status = http.StatusOK
	}
	if g.status == 0 {
		g.status = status
	}
	if status == http.StatusNoContent || status == http.StatusNotModified ||
		enc != "" || isStreamContent(ct) {
		g.ResponseWriter.WriteHeader(g.status)
		return
	}
	g.gzUsed = true
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	g.ResponseWriter.WriteHeader(g.status)
}

func (g *compressWriter) Write(b []byte) (int, error) {
	g.ensureStarted()
	h := g.ResponseWriter.Header()
	if h.Get("Content-Encoding") != "gzip" {
		return g.ResponseWriter.Write(b)
	}
	return g.w.Write(b)
}

// Wrote reports commit state for Recovery checks.
func (g *compressWriter) Wrote() bool { return g.started }

func (g *compressWriter) Flush() {
	g.ensureStarted()
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		if g.gzUsed && g.w != nil {
			_ = g.w.Flush()
		}
		f.Flush()
	}
}

func (g *compressWriter) finish() {
	if !g.wrote && !g.started {
		return
	}
	g.ensureStarted()
}

func isStreamContent(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "text/event-stream") ||
		strings.Contains(ct, "websocket") ||
		strings.HasPrefix(ct, "video/") ||
		strings.HasPrefix(ct, "audio/")
}
