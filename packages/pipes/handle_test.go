package pipes

import (
	"net/http/httptest"
	"strings"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

type handleSearchQuery struct {
	Q     string `query:"q" validate:"required"`
	Limit int    `query:"limit" validate:"gte=1,lte=100"`
}

func handleQueryCtx(t *testing.T, target string) router.Context {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	return router.NewContext(httptest.NewRecorder(), req)
}

func TestHandleQuery(t *testing.T) {
	h := HandleQuery(func(c router.Context, q handleSearchQuery) (any, error) {
		return map[string]any{"q": q.Q, "limit": q.Limit}, nil
	}, 200)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/s?q=ada&limit=10", nil)
	if err := h(router.NewContext(rec, req)); err != nil {
		t.Fatalf("valid query must not error: %v", err)
	}
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Missing required q → 422.
	if err := h(handleQueryCtx(t, "/s?limit=10")); err == nil {
		t.Fatal("missing q must fail")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("missing q must be 422, got %v", err)
	}
	// Malformed limit → 400.
	if err := h(handleQueryCtx(t, "/s?q=ada&limit=oops")); err == nil {
		t.Fatal("malformed limit must fail")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("malformed limit must be 400, got %v", err)
	}
	// Unknown rule → 422 (strict, consistent with Body/BindPath).
	type odd struct {
		X string `query:"x" validate:"frobnicator"`
	}
	ho := HandleQuery(func(c router.Context, q odd) (any, error) { return q, nil }, 0)
	if err := ho(handleQueryCtx(t, "/s?x=y")); err == nil {
		t.Fatal("unknown rule must fail strict")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("unknown rule must be 422, got %v", err)
	}
}

func TestHandleQueryNilPayload(t *testing.T) {
	h := HandleQuery(func(c router.Context, q handleSearchQuery) (any, error) { return nil, nil }, 0)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/s?q=ada&limit=5", nil)
	if err := h(router.NewContext(rec, req)); err != nil {
		t.Fatalf("nil payload must not error: %v", err)
	}
	if rec.Code != 200 {
		t.Fatalf("default status = %d, want 200", rec.Code)
	}
}

func TestHandlePath(t *testing.T) {
	h := HandlePath(func(c router.Context, p userParams) (any, error) {
		return map[string]int{"id": p.ID}, nil
	}, 0)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/users/42", nil)
	req.SetPathValue("id", "42")
	if err := h(router.NewContext(rec, req)); err != nil {
		t.Fatalf("valid path must not error: %v", err)
	}
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	mk := func(id string) router.Context {
		return pathCtx(t, "/users/{id}", "/users/"+id, map[string]string{"id": id})
	}
	if err := h(pathCtx(t, "/x", "/x", map[string]string{"id": "oops"})); err == nil {
		t.Fatal("malformed id must fail")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("malformed id must be 400, got %v", err)
	}
	if err := h(mk("0")); err == nil {
		t.Fatal("id=0 must fail gte=1")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("rule violation must be 422, got %v", err)
	}
}

type handleCreateBody struct {
	Name string `json:"name" validate:"required,min=2,max=80"`
}

func handleBodyQueryCtx(t *testing.T, target, payload string) router.Context {
	t.Helper()
	req := httptest.NewRequest("POST", target, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return router.NewContext(httptest.NewRecorder(), req)
}

func TestHandleBodyQuery(t *testing.T) {
	type opts struct {
		DryRun bool `query:"dry_run"`
	}
	h := HandleBodyQuery(func(c router.Context, b handleCreateBody, q opts) (any, error) {
		return map[string]any{"name": b.Name, "dry": q.DryRun}, nil
	}, 201)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x?dry_run=true", strings.NewReader(`{"name":"Ada"}`))
	req.Header.Set("Content-Type", "application/json")
	if err := h(router.NewContext(rec, req)); err != nil {
		t.Fatalf("valid body+query must not error: %v", err)
	}
	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	// Body errors win before query binding.
	hb := HandleBodyQuery(func(c router.Context, b handleCreateBody, q opts) (any, error) {
		return nil, nil
	}, 0)
	if err := hb(handleBodyQueryCtx(t, "/x?dry_run=oops", `{"name":"x"}`)); err == nil {
		t.Fatal("short name must fail")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("body violation must be 422, got %v", err)
	}
	// Valid body + malformed query → 400.
	h2 := HandleBodyQuery(func(c router.Context, b handleCreateBody, q opts) (any, error) {
		return b, nil
	}, 0)
	if err := h2(handleBodyQueryCtx(t, "/x?dry_run=oops", `{"name":"Ada"}`)); err == nil {
		t.Fatal("malformed query must fail")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("malformed query must be 400, got %v", err)
	}
}
