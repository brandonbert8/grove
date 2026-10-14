package grove

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

type scopedVal struct{ n int }

func TestRequestScopeMiddlewareSharesInstance(t *testing.T) {
	app := New()
	app.MustRegister((&ModuleDef{
		Name:      "s",
		Providers: []Provider{ProvideRequest(func(c *di.Container, ctx context.Context) (*scopedVal, error) { return &scopedVal{n: 7}, nil })},
	}).AsModule())

	var first, second *scopedVal
	app.Router.Use(RequestScope(app))
	app.Route("GET", "/x", func(c router.Context) error {
		a, err := InjectRequest[*scopedVal](c, app)
		if err != nil {
			return err
		}
		b, err := InjectRequest[*scopedVal](c, app)
		if err != nil {
			return err
		}
		first, second = a, b
		return c.JSON(200, map[string]bool{"ok": true})
	})

	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if first == nil || first != second {
		t.Fatal("one request must share one scoped instance")
	}
}

func TestInjectRequestWithoutMiddlewareUsesAdhocScope(t *testing.T) {
	app := New()
	app.MustRegister((&ModuleDef{
		Name:      "s",
		Providers: []Provider{ProvideRequest(func(c *di.Container, ctx context.Context) (*scopedVal, error) { return &scopedVal{}, nil })},
	}).AsModule())
	c := router.NewContext(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if _, err := InjectRequest[*scopedVal](c, app); err != nil {
		t.Fatalf("ad-hoc scope must resolve in tests: %v", err)
	}
}
