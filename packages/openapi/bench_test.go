package openapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

type benchDTO struct {
	Name  string `json:"name" validate:"required,min=2,max=80"`
	Email string `json:"email" validate:"required,email"`
}

func benchmarkApp() *grove.App {
	app := grove.New()
	handler := func(router.Context) error { return nil }
	app.RouteDoc("GET", "/users/{id}", handler,
		grove.WithSummary("Get user"),
		WithResponse[benchDTO](200, "user"),
	)
	app.RouteDoc("POST", "/users", handler,
		grove.WithSummary("Create user"),
		WithBody[benchDTO](),
		WithResponse[benchDTO](201, "created"),
	)
	return app
}

func BenchmarkBuild(b *testing.B) {
	app := benchmarkApp()
	info := Info{Title: "Bench", Version: "1.0.0"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Build(app, info)
	}
}

func BenchmarkSchemaFor(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SchemaFor[benchDTO]()
	}
}

func BenchmarkMountedSpec(b *testing.B) {
	app := benchmarkApp()
	Mount(app, "/openapi.json", Info{Title: "Bench", Version: "1.0.0"})
	handler := app.Handler()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("status = %d", w.Code)
		}
	}
}
