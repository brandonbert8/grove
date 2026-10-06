package grove

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

type svcA struct{ V string }
type svcB struct{ V string }
type svcC struct{ V string }
type ctrl3 struct{ A, B, C string }

func TestControllers3And4(t *testing.T) {
	app := New()
	app.MustRegister((&ModuleDef{
		Name: "m",
		Providers: []Provider{
			ProvideValue(svcA{V: "a"}),
			ProvideValue(svcB{V: "b"}),
			ProvideValue(svcC{V: "c"}),
		},
		BuildControllers: func(app *App) ([]ControllerDef, error) {
			return Controllers3(app,
				func(a svcA, b svcB, c svcC) *ctrl3 { return &ctrl3{a.V, b.V, c.V} },
				func(ctrl *ctrl3) ControllerDef {
					if ctrl.A+ctrl.B+ctrl.C != "abc" {
						t.Fatalf("wire3 got %+v", ctrl)
					}
					return ControllerDef{Prefix: "/x", Endpoints: []Endpoint{GET("", func(c router.Context) error {
						return c.JSON(200, ctrl)
					})}}
				})
		},
	}).AsModule())
	if len(app.Docs()) != 1 {
		t.Fatalf("want 1 doc, got %d", len(app.Docs()))
	}
}

func TestHandleBodyCreate(t *testing.T) {
	type In struct {
		Name string `json:"name" validate:"required,min=2"`
	}
	app := New()
	app.MustRegister((&ModuleDef{
		Name: "m",
		Controllers: []ControllerDef{{
			Prefix: "/items",
			Endpoints: []Endpoint{
				POST("", HandleBodyCreate(func(c router.Context, in In) (any, error) {
					return map[string]string{"name": in.Name}, nil
				}), WithOperation("Create", map[int]string{201: "created"})),
			},
		}},
	}).AsModule())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/items", strings.NewReader(`{"name":"Ada"}`))
	req.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("want 201, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestStrictProvidersDuplicateFails(t *testing.T) {
	app := New()
	app.EnableStrictProviders()
	err := app.Register((&ModuleDef{
		Name:      "a",
		Providers: []Provider{ProvideValue(svcA{V: "1"})},
	}).AsModule())
	if err != nil {
		t.Fatal(err)
	}
	err = app.Register((&ModuleDef{
		Name:      "b",
		Providers: []Provider{ProvideValue(svcA{V: "2"})},
	}).AsModule())
	if err == nil {
		t.Fatal("strict providers must error on duplicate key")
	}
}

func TestVerifyExports(t *testing.T) {
	app := New()
	app.EnableExportScoping()
	app.MustRegister((&ModuleDef{
		Name:      "a",
		Providers: []Provider{ProvideValue(svcA{V: "1"})},
		Exports:   []string{di.KeyFor[svcA]()},
	}).AsModule())
	if err := app.VerifyExports(); err != nil {
		t.Fatalf("valid exports must pass: %v", err)
	}
	bad := New()
	bad.EnableExportScoping()
	bad.MustRegister((&ModuleDef{
		Name:    "bad",
		Exports: []string{"missing.Provider"},
	}).AsModule())
	if err := bad.VerifyExports(); err == nil {
		t.Fatal("unknown export must fail VerifyExports")
	}
}

func TestNotFoundGoesThroughFilters(t *testing.T) {
	app := New()
	called := false
	app.UseFilters(FilterFunc(func(c router.Context, err error) error {
		called = true
		return err
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/nope", nil)
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	if !called {
		t.Fatal("global filter must observe 404")
	}
}

func TestWithOperation(t *testing.T) {
	e := GET("/x", func(c router.Context) error { return nil }, WithOperation("Do x", map[int]string{200: "ok"}))
	if e.Summary != "Do x" || e.Responses[200] != "ok" {
		t.Fatalf("WithOperation must set summary+responses, got %+v", e)
	}
}
