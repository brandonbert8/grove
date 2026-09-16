package grove

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

type handleBodyDTO struct {
	Name string `json:"name" validate:"required,min=2,max=80"`
}

func handleBodyCtx(t *testing.T, payload string) router.Context {
	t.Helper()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return router.NewContext(httptest.NewRecorder(), req)
}

func TestHandleBodyValidates(t *testing.T) {
	h := HandleBody(func(c router.Context, in handleBodyDTO) (any, error) {
		return map[string]string{"hello": in.Name}, nil
	}, 201)
	// Valid body → 201 payload (recorder doubles as writer here).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"name":"Ada"}`))
	req.Header.Set("Content-Type", "application/json")
	if err := h(router.NewContext(rec, req)); err != nil {
		t.Fatalf("valid body must not error, got %v", err)
	}
	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201 (body %q)", rec.Code, rec.Body.String())
	}
	// Short name → 422.
	if err := h(handleBodyCtx(t, `{"name":"x"}`)); err == nil {
		t.Fatal("short name must fail")
	} else if he, ok := AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("violation must be 422, got %v", err)
	}
	// Malformed JSON → 400.
	if err := h(handleBodyCtx(t, `{oops`)); err == nil {
		t.Fatal("malformed JSON must fail")
	} else if he, ok := AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("malformed must be 400, got %v", err)
	}
	// Unknown rule (strict by default) → 422.
	type odd struct {
		X string `json:"x" validate:"frobnicator"`
	}
	ho := HandleBody(func(c router.Context, in odd) (any, error) { return in, nil }, 0)
	if err := ho(handleBodyCtx(t, `{"x":"y"}`)); err == nil {
		t.Fatal("unknown rule must fail strict")
	}
}

func TestControllersHelper(t *testing.T) {
	type Svc struct{}
	type Ctrl struct{ svc *Svc }
	app := New()
	app.Container.Replace(di.KeyFor[Svc](), &Svc{})
	defs, err := Controllers(app, func(s *Svc) *Ctrl { return &Ctrl{svc: s} },
		func(c *Ctrl) ControllerDef {
			return ControllerDef{Prefix: "/x", Endpoints: []Endpoint{
				GET("", func(ctx router.Context) error { return ctx.NoContent(200) }),
			}}
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 || defs[0].Prefix != "/x" {
		t.Fatalf("defs = %+v", defs)
	}
}
