package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/router"
)

func benchmarkMiddleware(b *testing.B) *router.DefaultRouter {
	b.Helper()
	reg := New()
	r := router.New()
	r.Use(reg.Middleware())
	r.GET("/users/{id}", func(c router.Context) error { return c.NoContent(http.StatusNoContent) })
	return r
}

func BenchmarkMiddleware(b *testing.B) {
	r := benchmarkMiddleware(b)
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkMiddlewareParallel(b *testing.B) {
	r := benchmarkMiddleware(b)
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})
}

func BenchmarkObserve(b *testing.B) {
	r := New()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.observe("GET", "/users/{id}", 200, 0.001)
	}
}

func BenchmarkObserveManyRoutes(b *testing.B) {
	r := New()
	routes := []string{"/", "/users", "/users/{id}", "/healthz", "/openapi.json"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.observe("GET", routes[i%len(routes)], 200, 0.001)
	}
}

func BenchmarkObserveParallel(b *testing.B) {
	r := New()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.observe("GET", "/users/{id}", 200, 0.001)
		}
	})
}

func BenchmarkObserveManyRoutesParallel(b *testing.B) {
	r := New()
	routes := []string{"/", "/users", "/users/{id}", "/healthz", "/openapi.json"}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			r.observe("GET", routes[i%len(routes)], 200, 0.001)
			i++
		}
	})
}
