package grove

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

func TestHttpErrorStatusCodes(t *testing.T) {
	cases := []struct {
		err  *HttpError
		want int
	}{
		{BadRequest("x"), 400},
		{Unauthorized(""), 401},
		{Forbidden(""), 403},
		{NotFound(""), 404},
		{Conflict("x"), 409},
		{Unprocessable("", nil), 422},
		{Internal(""), 500},
	}
	for _, tc := range cases {
		if tc.err.StatusCode() != tc.want {
			t.Fatalf("status = %d, want %d", tc.err.StatusCode(), tc.want)
		}
	}
}

func TestHttpErrorMappedByRouter(t *testing.T) {
	app := New()
	app.Router.GET("/missing", func(c router.Context) error {
		return NotFound("user not found")
	})
	req := httptest.NewRequest("GET", "/missing", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPlainErrorStill500(t *testing.T) {
	app := New()
	app.Router.GET("/boom", func(c router.Context) error {
		return errors.New("boom")
	})
	req := httptest.NewRequest("GET", "/boom", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

type nestSvc struct{ prefix string }

func TestModuleDefBuildOrderAndDedupe(t *testing.T) {
	builds := 0
	shared := &ModuleDef{
		Name: "shared",
		Providers: []Provider{
			Provide(di.Singleton, func(c *di.Container) (*nestSvc, error) {
				builds++
				return &nestSvc{prefix: "hi"}, nil
			}),
		},
	}
	mkLeaf := func() *ModuleDef {
		return &ModuleDef{
			Name:    "leaf",
			Imports: []*ModuleDef{shared, shared}, // duplicate import builds once
			BuildControllers: func(app *App) ([]ControllerDef, error) {
				svc, err := di.ResolveAs[*nestSvc](app.Container)
				if err != nil {
					return nil, err
				}
				return []ControllerDef{{
					Prefix:    "/leaf",
					Endpoints: []Endpoint{GET("", func(c router.Context) error { return c.String(200, svc.prefix) })},
				}}, nil
			},
		}
	}

	app := New()
	// Same pointer twice: second registration is a no-op (no double
	// routes, no double provider builds).
	leaf := mkLeaf()
	if err := app.Register(leaf.AsModule(), leaf.AsModule()); err != nil {
		t.Fatal(err)
	}
	if builds != 1 {
		t.Fatalf("shared provider built %d times, want 1", builds)
	}
	// Singleton resolves to the same instance.
	a, _ := di.ResolveAs[*nestSvc](app.Container)
	b, _ := di.ResolveAs[*nestSvc](app.Container)
	if a != b {
		t.Fatal("singleton must return the same instance")
	}
	// Route mounted and serving.
	req := httptest.NewRequest("GET", "/leaf", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "hi" {
		t.Fatalf("GET /leaf = %d %q, want 200 hi", rec.Code, rec.Body.String())
	}
}

func TestModuleDefControllerMiddleware(t *testing.T) {
	deny := UseGuard(GuardFunc(func(c router.Context) error { return Forbidden("") }))
	app := New()
	mod := &ModuleDef{
		Name: "secure",
		Controllers: []ControllerDef{{
			Prefix:     "/secure",
			Middleware: []router.Middleware{deny},
			Endpoints:  []Endpoint{GET("", func(c router.Context) error { return c.NoContent(204) })},
		}},
	}
	if err := app.Register(mod.AsModule()); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/secure", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestChainOrder(t *testing.T) {
	var order []string
	mk := func(name string) router.Middleware {
		return func(next router.HandlerFunc) router.HandlerFunc {
			return func(c router.Context) error {
				order = append(order, name)
				return next(c)
			}
		}
	}
	app := New()
	app.Router.GET("/c", func(c router.Context) error { return c.NoContent(200) },
		Chain(mk("a"), mk("b")))
	req := httptest.NewRequest("GET", "/c", nil)
	app.Handler().ServeHTTP(httptest.NewRecorder(), req)
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("chain order = %v, want [a b]", order)
	}
}
