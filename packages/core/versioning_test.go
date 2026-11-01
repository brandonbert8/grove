package grove

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/router"
)

func versionHandler(body string) router.HandlerFunc {
	return func(c router.Context) error { return c.String(http.StatusOK, body) }
}

func doVersionRequest(app *App, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec
}

func TestVersioningURIDefaultAndOverride(t *testing.T) {
	app := New()
	app.EnableVersioning(WithDefaultVersion("1"))
	app.mountController(ControllerDef{
		Prefix: "/users",
		Endpoints: []Endpoint{
			GET("", versionHandler("v1"), WithSummary("list")),
			GET("/new", versionHandler("v2"), WithSummary("new"), WithVersion("2")),
		},
	})

	if rec := doVersionRequest(app, "GET", "/v1/users", nil); rec.Code != 200 || rec.Body.String() != "v1" {
		t.Fatalf("/v1/users = %d %q", rec.Code, rec.Body.String())
	}
	if rec := doVersionRequest(app, "GET", "/v2/users/new", nil); rec.Code != 200 || rec.Body.String() != "v2" {
		t.Fatalf("/v2/users/new = %d %q", rec.Code, rec.Body.String())
	}
	if rec := doVersionRequest(app, "GET", "/users", nil); rec.Code != 404 {
		t.Fatalf("/users = %d, want 404", rec.Code)
	}
	if rec := doVersionRequest(app, "GET", "/v1/users/new", nil); rec.Code != 404 {
		t.Fatalf("/v1/users/new = %d, want 404", rec.Code)
	}

	docs := app.Docs()
	paths := map[string]bool{}
	for _, d := range docs {
		paths[d.Path] = true
	}
	if !paths["/v1/users"] || !paths["/v2/users/new"] {
		t.Fatalf("docs paths = %v", paths)
	}
}

func TestVersioningURIGlobalPrefixAndExclusion(t *testing.T) {
	app := New()
	app.SetGlobalPrefix("/api", "/healthz")
	app.EnableVersioning(WithDefaultVersion("1"))
	app.mountController(ControllerDef{
		Prefix:    "/users",
		Endpoints: []Endpoint{GET("", versionHandler("ok"), WithSummary("list"))},
	})
	app.mountController(ControllerDef{
		Prefix:    "/healthz",
		Endpoints: []Endpoint{GET("", versionHandler("healthy"), WithSummary("health"))},
	})

	if rec := doVersionRequest(app, "GET", "/api/v1/users", nil); rec.Code != 200 {
		t.Fatalf("/api/v1/users = %d", rec.Code)
	}
	if rec := doVersionRequest(app, "GET", "/v1/users", nil); rec.Code != 404 {
		t.Fatalf("/v1/users = %d, want 404", rec.Code)
	}
	if rec := doVersionRequest(app, "GET", "/healthz", nil); rec.Code != 200 {
		t.Fatalf("/healthz = %d", rec.Code)
	}
	if rec := doVersionRequest(app, "GET", "/api/v1/healthz", nil); rec.Code != 404 {
		t.Fatalf("/api/v1/healthz = %d, want 404", rec.Code)
	}
}

func TestVersioningExcludedPrefixBypassesHeaderVersion(t *testing.T) {
	app := New()
	app.SetGlobalPrefix("/api", "/healthz")
	app.EnableVersioning(WithVersioningType(VersioningHeader), WithDefaultVersion("1"))
	app.mountController(ControllerDef{
		Prefix:    "/healthz",
		Endpoints: []Endpoint{GET("", versionHandler("healthy"), WithSummary("health"))},
	})
	if rec := doVersionRequest(app, "GET", "/healthz", nil); rec.Code != 200 {
		t.Fatalf("/healthz = %d", rec.Code)
	}
}

func TestVersioningHeader(t *testing.T) {
	app := New()
	app.EnableVersioning(WithVersioningType(VersioningHeader), WithDefaultVersion("1"))
	app.mountController(ControllerDef{
		Prefix: "/users",
		Endpoints: []Endpoint{
			GET("", versionHandler("v1"), WithSummary("v1")),
			GET("", versionHandler("v2"), WithSummary("v2"), WithVersion("2")),
		},
	})

	if rec := doVersionRequest(app, "GET", "/users", nil); rec.Code != 200 || rec.Body.String() != "v1" {
		t.Fatalf("default = %d %q", rec.Code, rec.Body.String())
	}
	if rec := doVersionRequest(app, "GET", "/users", map[string]string{"Accept-Version": "2"}); rec.Body.String() != "v2" {
		t.Fatalf("header 2 = %d %q", rec.Code, rec.Body.String())
	}
	if rec := doVersionRequest(app, "GET", "/users", map[string]string{"Accept-Version": "v2"}); rec.Body.String() != "v2" {
		t.Fatalf("header v2 = %d %q", rec.Code, rec.Body.String())
	}
	rec := doVersionRequest(app, "GET", "/users", map[string]string{"Accept-Version": "9"})
	if rec.Code != 404 {
		t.Fatalf("unknown version = %d, want 404", rec.Code)
	}
	var body router.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "API_VERSION_NOT_FOUND" {
		t.Fatalf("code = %q", body.Code)
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want header versions not to look like duplicate routes", err)
	}
}

func TestVersioningMediaType(t *testing.T) {
	app := New()
	app.EnableVersioning(WithVersioningType(VersioningMediaType), WithDefaultVersion("1"))
	app.mountController(ControllerDef{
		Prefix: "/users",
		Endpoints: []Endpoint{
			GET("", versionHandler("v1"), WithSummary("v1")),
			GET("", versionHandler("v2"), WithSummary("v2"), WithVersion("2")),
		},
	})

	if rec := doVersionRequest(app, "GET", "/users", nil); rec.Body.String() != "v1" {
		t.Fatalf("default = %q", rec.Body.String())
	}
	rec := doVersionRequest(app, "GET", "/users", map[string]string{
		"Accept": "application/vnd.grove.v2+json",
	})
	if rec.Code != 200 || rec.Body.String() != "v2" {
		t.Fatalf("media type = %d %q", rec.Code, rec.Body.String())
	}
}

func TestVersioningControllerVersion(t *testing.T) {
	app := New()
	app.EnableVersioning(WithVersionPrefix(""))
	app.mountController(ControllerDef{
		Prefix:    "/users",
		Version:   "3",
		Endpoints: []Endpoint{GET("", versionHandler("v3"), WithSummary("list"))},
	})
	if rec := doVersionRequest(app, "GET", "/3/users", nil); rec.Code != 200 {
		t.Fatalf("/3/users = %d", rec.Code)
	}
}

func TestVersioningDisabledKeepsPaths(t *testing.T) {
	app := New()
	app.mountController(ControllerDef{
		Prefix:    "/users",
		Endpoints: []Endpoint{GET("", versionHandler("plain"), WithSummary("list"), WithVersion("2"))},
	})
	if rec := doVersionRequest(app, "GET", "/users", nil); rec.Code != 200 {
		t.Fatalf("/users = %d", rec.Code)
	}
}

func TestVersioningHeaderAppliesControllerMiddlewarePerVersion(t *testing.T) {
	app := New()
	app.EnableVersioning(WithVersioningType(VersioningHeader), WithDefaultVersion("1"))
	var seen []string
	mark := func(name string) router.Middleware {
		return func(next router.HandlerFunc) router.HandlerFunc {
			return func(c router.Context) error {
				seen = append(seen, name)
				return next(c)
			}
		}
	}
	app.mountController(ControllerDef{
		Prefix:     "/users",
		Version:    "1",
		Middleware: []router.Middleware{mark("v1-controller")},
		Endpoints:  []Endpoint{GET("", versionHandler("v1"))},
	})
	app.mountController(ControllerDef{
		Prefix:     "/users",
		Version:    "2",
		Middleware: []router.Middleware{mark("v2-controller")},
		Endpoints:  []Endpoint{GET("", versionHandler("v2"))},
	})

	seen = nil
	rec := doVersionRequest(app, "GET", "/users", map[string]string{"Accept-Version": "2"})
	if rec.Body.String() != "v2" || len(seen) != 1 || seen[0] != "v2-controller" {
		t.Fatalf("v2 = %d %q, middleware = %v", rec.Code, rec.Body.String(), seen)
	}
	seen = nil
	rec = doVersionRequest(app, "GET", "/users", map[string]string{"Accept-Version": "1"})
	if rec.Body.String() != "v1" || len(seen) != 1 || seen[0] != "v1-controller" {
		t.Fatalf("v1 = %d %q, middleware = %v", rec.Code, rec.Body.String(), seen)
	}
}
