package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/logger"
	"github.com/brandonbert8/grove/packages/router"
)

func benchmarkChain(b *testing.B, mw ...router.Middleware) *router.DefaultRouter {
	b.Helper()
	r := router.New()
	r.Use(mw...)
	r.GET("/users/{id}", func(c router.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
	return r
}

func benchmarkRequests(b *testing.B, r *router.DefaultRouter) {
	b.Helper()
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}
}

func BenchmarkLogging(b *testing.B) {
	benchmarkRequests(b, benchmarkChain(b, Logging(logger.Discard())))
}

func BenchmarkRequestIDLogging(b *testing.B) {
	benchmarkRequests(b, benchmarkChain(b, RequestID(), Logging(logger.Discard())))
}
