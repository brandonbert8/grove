package pipes

import (
	"net/http/httptest"
	"strings"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

func headerCtx(t *testing.T, headers map[string]string) router.Context {
	t.Helper()
	req := httptest.NewRequest("GET", "/x", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return router.NewContext(httptest.NewRecorder(), req)
}

type traceCtx struct {
	RequestID string `header:"X-Request-ID" validate:"required"`
	Retries   int    `header:"X-Retries"`
}

func TestBindHeader(t *testing.T) {
	c := headerCtx(t, map[string]string{"X-Request-ID": "abc", "X-Retries": "3"})
	var v traceCtx
	if err := BindHeader(c, &v); err != nil {
		t.Fatal(err)
	}
	if v.RequestID != "abc" || v.Retries != 3 {
		t.Fatalf("headers = %+v", v)
	}
	// Names are case-insensitive per RFC 9110.
	c2 := headerCtx(t, map[string]string{"x-request-id": "zzz"})
	var v2 traceCtx
	if err := BindHeader(c2, &v2); err != nil {
		t.Fatal(err)
	}
	if v2.RequestID != "zzz" {
		t.Fatalf("headers = %+v", v2)
	}
	// Absent required header is 422; malformed value is 400.
	if err := BindHeader(headerCtx(t, nil), &traceCtx{}); err == nil {
		t.Fatal("missing required header must fail")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("missing header must be 422, got %v", err)
	}
	var v3 traceCtx
	if err := BindHeader(headerCtx(t, map[string]string{"X-Request-ID": "a", "X-Retries": "many"}), &v3); err == nil {
		t.Fatal("malformed header must 400")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("malformed header must be 400, got %v", err)
	}
}

func TestParsePage(t *testing.T) {
	mk := func(target string) router.Context {
		return router.NewContext(httptest.NewRecorder(), httptest.NewRequest("GET", target, nil))
	}
	p, err := ParsePage(mk("/x"), 0)
	if err != nil || p.Page != 1 || p.Limit != DefaultPageLimit || p.Offset != 0 {
		t.Fatalf("defaults = %+v,%v", p, err)
	}
	p, err = ParsePage(mk("/x?page=3&limit=10"), 100)
	if err != nil || p.Page != 3 || p.Limit != 10 || p.Offset != 20 {
		t.Fatalf("page = %+v,%v", p, err)
	}
	p, err = ParsePage(mk("/x?limit=500"), 100)
	if err != nil || p.Limit != 100 {
		t.Fatalf("cap = %+v,%v", p, err)
	}
	p, err = ParsePage(mk("/x?page=0&limit=-5"), 100)
	if err != nil || p.Page != 1 || p.Limit != 1 {
		t.Fatalf("floor = %+v,%v", p, err)
	}
	if _, err := ParsePage(mk("/x?page=many"), 100); err == nil {
		t.Fatal("malformed page must 400")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("malformed page must be 400, got %v", err)
	}
}

type userParams struct {
	ID int `path:"id" validate:"gte=1"`
}

func pathCtx(t *testing.T, pattern, target string, params map[string]string) router.Context {
	t.Helper()
	_ = pattern
	req := httptest.NewRequest("GET", target, nil)
	for k, v := range params {
		req.SetPathValue(k, v)
	}
	return router.NewContext(httptest.NewRecorder(), req)
}

func TestBindPath(t *testing.T) {
	c := pathCtx(t, "/users/{id}", "/users/42", map[string]string{"id": "42"})
	var p userParams
	if err := BindPath(c, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != 42 {
		t.Fatalf("id = %d", p.ID)
	}
	var missing userParams
	if err := BindPath(pathCtx(t, "/x", "/x", nil), &missing); err == nil {
		t.Fatal("missing path param must 400")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("missing must be 400, got %v", err)
	}
	var bad userParams
	if err := BindPath(pathCtx(t, "/x", "/x", map[string]string{"id": "abc"}), &bad); err == nil {
		t.Fatal("malformed path param must 400")
	}
	var zero userParams
	if err := BindPath(pathCtx(t, "/x", "/x", map[string]string{"id": "0"}), &zero); err == nil {
		t.Fatal("rule violation must 422")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("violation must be 422, got %v", err)
	}
}

func TestValidateStrict(t *testing.T) {
	type m struct {
		Name string `json:"name" validate:"required,min=2"`
	}
	if errs := ValidateStrict(m{Name: "Ada"}); len(errs) != 0 {
		t.Fatalf("valid must pass strict: %v", errs)
	}
	type typo struct {
		Age int `json:"age" validate:"gte=abc"`
	}
	if errs := Validate(typo{Age: 30}); len(errs) != 0 {
		t.Fatalf("lenient must ignore malformed param: %v", errs)
	}
	if errs := ValidateStrict(typo{Age: 30}); len(errs) != 1 || !strings.Contains(errs[0].Message, "invalid rule parameter") {
		t.Fatalf("strict must flag malformed param: %v", errs)
	}
	type unknown struct {
		X string `json:"x" validate:"frobnicator"`
	}
	if errs := Validate(unknown{X: "y"}); len(errs) != 0 {
		t.Fatalf("lenient must ignore unknown rule: %v", errs)
	}
	if errs := ValidateStrict(unknown{X: "y"}); len(errs) != 1 || !strings.Contains(errs[0].Message, "unknown validation rule") {
		t.Fatalf("strict must flag unknown rule: %v", errs)
	}
}
