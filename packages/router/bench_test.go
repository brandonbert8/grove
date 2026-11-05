package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func benchmarkServeHTTP(b *testing.B, path string) {
	b.Helper()
	r := New()
	r.GET("/static", func(c Context) error { return c.String(http.StatusOK, "ok") })
	r.GET("/users/{id}", func(c Context) error { return c.String(http.StatusOK, c.Param("id")) })
	req := httptest.NewRequest(http.MethodGet, path, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkServeHTTPStatic(b *testing.B) { benchmarkServeHTTP(b, "/static") }

func BenchmarkServeHTTPParam(b *testing.B) { benchmarkServeHTTP(b, "/users/42") }

func BenchmarkServeHTTPMiddleware(b *testing.B) {
	r := New()
	wrap := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error { return next(c) }
	}
	r.Use(wrap, wrap, wrap, wrap, wrap)
	r.GET("/static", func(c Context) error { return c.String(http.StatusOK, "ok") })
	req := httptest.NewRequest(http.MethodGet, "/static", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkServeHTTP404(b *testing.B) {
	r := New()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkApplyMiddleware(b *testing.B) {
	h := func(Context) error { return nil }
	pass := func(next HandlerFunc) HandlerFunc { return next }
	chain := []Middleware{pass, pass, pass, pass, pass}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = applyMiddleware(h, chain)
	}
}
