package metrics_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/brandonbert8/grove/packages/httperr"
	"github.com/brandonbert8/grove/packages/metrics"
	"github.com/brandonbert8/grove/packages/router"
)

func serve(r router.Router, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func exposition(t *testing.T, reg *metrics.Registry) string {
	t.Helper()
	var buf bytes.Buffer
	if err := reg.WriteText(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestMiddlewareRecordsMatchedRoute(t *testing.T) {
	reg := metrics.New()
	r := router.New()
	r.Use(reg.Middleware())
	r.GET("/users/{id}", func(c router.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	rec := serve(r, http.MethodGet, "/users/42")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	out := exposition(t, reg)
	for _, want := range []string{
		`http_requests_total{method="GET",route="/users/{id}",status="204"} 1`,
		`http_requests_in_flight 0`,
		`http_request_duration_seconds_bucket{method="GET",route="/users/{id}",le="+Inf"} 1`,
		`http_request_duration_seconds_count{method="GET",route="/users/{id}"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestMiddlewareUsesErrorStatus(t *testing.T) {
	reg := metrics.New()
	r := router.New()
	r.Use(reg.Middleware())
	r.GET("/missing", func(router.Context) error {
		return httperr.NotFound("nope")
	})

	rec := serve(r, http.MethodGet, "/missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	out := exposition(t, reg)
	if !strings.Contains(out, `http_requests_total{method="GET",route="/missing",status="404"} 1`) {
		t.Fatalf("missing 404 counter in:\n%s", out)
	}
}

func TestMiddlewareLabelsUnmatchedRoutes(t *testing.T) {
	reg := metrics.New()
	r := router.New()
	r.Use(reg.Middleware())
	r.GET("/ok", func(c router.Context) error { return c.NoContent(http.StatusNoContent) })

	rec := serve(r, http.MethodGet, "/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	out := exposition(t, reg)
	if !strings.Contains(out, `http_requests_total{method="GET",route="unmatched",status="404"} 1`) {
		t.Fatalf("missing unmatched counter in:\n%s", out)
	}
}

func TestWithBucketsSortsAndFilters(t *testing.T) {
	reg := metrics.New(metrics.WithBuckets([]float64{1, -1, 0.5, 1}))
	r := router.New()
	r.Use(reg.Middleware())
	r.GET("/", func(c router.Context) error { return c.NoContent(http.StatusOK) })

	serve(r, http.MethodGet, "/")
	out := exposition(t, reg)
	if strings.Contains(out, `le="-1"`) {
		t.Fatalf("negative bucket must be filtered:\n%s", out)
	}
	if strings.Count(out, `le="1"`) != 1 {
		t.Fatalf("duplicate bucket must be removed:\n%s", out)
	}
	if !strings.Contains(out, `le="0.5"`) || !strings.Contains(out, `le="1"`) {
		t.Fatalf("expected sorted buckets in:\n%s", out)
	}
}

func TestHandlerFuncExposesMetrics(t *testing.T) {
	reg := metrics.New()
	r := router.New()
	r.GET("/metrics", reg.HandlerFunc())

	rec := serve(r, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", ct)
	}
	if !strings.Contains(rec.Body.String(), "http_requests_in_flight") {
		t.Fatalf("body missing metrics:\n%s", rec.Body.String())
	}
}

func TestResetClearsMetrics(t *testing.T) {
	reg := metrics.New()
	r := router.New()
	r.Use(reg.Middleware())
	r.GET("/", func(c router.Context) error { return c.NoContent(http.StatusOK) })

	serve(r, http.MethodGet, "/")
	reg.Reset()
	out := exposition(t, reg)
	if strings.Contains(out, "http_requests_total{") {
		t.Fatalf("reset must clear counters:\n%s", out)
	}
}
