package grove

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

func okHandler(c router.Context) error { return c.NoContent(204) }

func TestEndpointOptionsDocs(t *testing.T) {
	e := GET("/users",
		okHandler,
		WithSummary("List users"),
		WithDescription("long form"),
		WithTags("users", "ops"),
		WithDeprecated(),
		WithQuery(QueryParam("page", "Page number", false)),
		WithResponses(map[int]string{200: "users"}),
		WithSecurity("bearerAuth"),
	)
	if e.Summary != "List users" || e.Description != "long form" {
		t.Fatalf("docs not applied: %+v", e)
	}
	if len(e.Tags) != 2 || !e.Deprecated {
		t.Fatalf("tags/deprecated not applied: %+v", e)
	}
	if len(e.Query) != 1 || e.Query[0].Name != "page" {
		t.Fatalf("query not applied: %+v", e)
	}
	if e.Responses[200] != "users" || len(e.Security) != 1 {
		t.Fatalf("responses/security not applied: %+v", e)
	}
}

func TestEndpointOptionsMiddlewareCompat(t *testing.T) {
	// Raw router.Middleware values keep compiling (pre-options code).
	mw := func(next router.HandlerFunc) router.HandlerFunc { return next }
	e := POST("", okHandler, mw, Use(mw), WithSummary("Create"))
	if len(e.Middleware) != 2 {
		t.Fatalf("middleware count = %d, want 2", len(e.Middleware))
	}
	if e.Summary != "Create" {
		t.Fatalf("summary = %q", e.Summary)
	}
}

func TestEndpointOptionsGuards(t *testing.T) {
	deny := GuardFunc(func(c router.Context) error { return Forbidden("") })
	e := GET("", okHandler, WithGuards(deny), deny)
	if len(e.Middleware) != 2 {
		t.Fatalf("guard middleware count = %d, want 2", len(e.Middleware))
	}
	app := New()
	mod := &ModuleDef{
		Name:        "secure",
		Controllers: []ControllerDef{{Prefix: "/secure", Endpoints: []Endpoint{e}}},
	}
	if err := app.Register(mod.AsModule()); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/secure", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestEndpointInvalidOptionPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid option type")
		}
	}()
	_ = GET("", okHandler, "not-an-option")
}

type injectSvc struct{ v string }

func TestInjectAndWire(t *testing.T) {
	app := New()
	if err := app.Container.RegisterSingleton(di.KeyFor[*injectSvc](), &injectSvc{v: "hi"}); err != nil {
		t.Fatal(err)
	}
	svc, err := Inject[*injectSvc](app)
	if err != nil || svc.v != "hi" {
		t.Fatalf("Inject = %+v, %v", svc, err)
	}
	if got := MustInject[*injectSvc](app); got.v != "hi" {
		t.Fatalf("MustInject = %+v", got)
	}
	type Ctrl struct{ svc *injectSvc }
	ctrl, err := Wire(app, func(s *injectSvc) *Ctrl { return &Ctrl{svc: s} })
	if err != nil || ctrl.svc.v != "hi" {
		t.Fatalf("Wire = %+v, %v", ctrl, err)
	}
	if _, err := Inject[*injectSvc](New()); err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestControllerFor(t *testing.T) {
	type Ctrl struct{ v string }
	app := New()
	newCtrl := func() *Ctrl { return &Ctrl{v: "c"} }
	if err := app.Container.RegisterSingletonFactory(di.KeyFor[*Ctrl](), func(c *di.Container) (any, error) {
		return newCtrl(), nil
	}); err != nil {
		t.Fatal(err)
	}
	def, err := ControllerFor(app, func(c *Ctrl) ControllerDef {
		return ControllerDef{Prefix: "/c", Endpoints: []Endpoint{GET("", okHandler)}}
	})
	if err != nil || def.Prefix != "/c" {
		t.Fatalf("ControllerFor = %+v, %v", def, err)
	}
}
