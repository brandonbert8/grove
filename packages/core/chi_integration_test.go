package grove_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/middleware"
	"github.com/brandonbert8/grove/packages/pipes"
	"github.com/brandonbert8/grove/packages/router"
)

func serve(t *testing.T, app *grove.App, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	var h http.Handler = app.Handler()
	h.ServeHTTP(rec, req)
	return rec
}

func TestChiAppVerbsAndParams(t *testing.T) {
	app := grove.New()
	app.Router.GET("/items/{id}", func(c router.Context) error {
		return c.JSON(200, map[string]string{"id": c.Param("id")})
	})
	app.Router.POST("/items", func(c router.Context) error { return c.JSON(201, map[string]bool{"ok": true}) })
	app.Router.PUT("/items/{id}", func(c router.Context) error { return c.NoContent(200) })
	app.Router.PATCH("/items/{id}", func(c router.Context) error { return c.NoContent(200) })
	app.Router.DELETE("/items/{id}", func(c router.Context) error { return c.NoContent(204) })

	if rec := serve(t, app, "GET", "/items/7", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"7"`) {
		t.Fatalf("GET /items/7 = %d %q", rec.Code, rec.Body.String())
	}
	for _, tc := range []struct{ method, path string }{
		{"POST", "/items"}, {"PUT", "/items/7"}, {"PATCH", "/items/7"}, {"DELETE", "/items/7"},
	} {
		if rec := serve(t, app, tc.method, tc.path, nil); rec.Code >= 300 {
			t.Fatalf("%s %s = %d, want 2xx", tc.method, tc.path, rec.Code)
		}
	}
}

func TestChiAppNotFoundAndMethodNotAllowedAreJSON(t *testing.T) {
	app := grove.New()
	app.Router.GET("/only", func(c router.Context) error { return c.NoContent(204) })

	rec := serve(t, app, "GET", "/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("404 status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("404 Content-Type = %q", ct)
	}

	rec = serve(t, app, "POST", "/only", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("405 status = %d, want 405", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("405 body is not JSON: %v", err)
	}
}

func TestChiAppPanicRecovery(t *testing.T) {
	app := grove.New()
	app.Router.Use(middleware.Recovery(app.Logger))
	app.Router.GET("/boom", func(c router.Context) error { panic("kaboom") })

	rec := serve(t, app, "GET", "/boom", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("panic body is not JSON: %v", err)
	}
}

func TestChiAppModulesGuardsPipes(t *testing.T) {
	type item struct {
		Name string `json:"name" validate:"required,min=2"`
	}
	type svc struct{ prefix string }

	mod := &grove.ModuleDef{
		Name: "shop",
		Providers: []grove.Provider{
			grove.Provide(di.Singleton, func(*di.Container) (*svc, error) { return &svc{prefix: "v"}, nil }),
		},
		BuildControllers: func(app *grove.App) ([]grove.ControllerDef, error) {
			s, err := di.ResolveAs[*svc](app.Container)
			if err != nil {
				return nil, err
			}
			guard := grove.UseGuard(grove.GuardFunc(func(c router.Context) error {
				if c.Header("Authorization") != "Bearer ok" {
					return grove.Unauthorized("nope")
				}
				return nil
			}))
			return []grove.ControllerDef{{
				Prefix:     "/shop",
				Middleware: []router.Middleware{guard},
				Endpoints: []grove.Endpoint{
					grove.GET("/{id}", func(c router.Context) error {
						return c.JSON(200, map[string]string{"id": c.Param("id"), "p": s.prefix})
					}),
					grove.POST("", func(c router.Context) error {
						var in item
						if err := pipes.ValidateBody(c, &in); err != nil {
							return err
						}
						return c.JSON(201, in)
					}),
				},
			}}, nil
		},
	}
	app := grove.New()
	if err := app.Register(mod.AsModule()); err != nil {
		t.Fatal(err)
	}

	if rec := serve(t, app, "GET", "/shop/9", map[string]string{"Authorization": "Bearer ok"}); rec.Code != 200 {
		t.Fatalf("guarded GET = %d, want 200", rec.Code)
	}
	if rec := serve(t, app, "GET", "/shop/9", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unguarded GET = %d, want 401", rec.Code)
	}
	if n := len(app.Router.Routes()); n != 3 {
		t.Fatalf("routes = %d, want 3 (GET + auto HEAD + POST)", n)
	}
}
