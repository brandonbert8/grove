package pipes

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

type resource struct {
	ID   string `json:"id" validate:"required,uuid"`
	Link string `json:"link" validate:"url"`
}

func TestUUIDAndURLRules(t *testing.T) {
	ok := resource{ID: "550e8400-e29b-41d4-a716-446655440000", Link: "https://example.com/a"}
	if errs := Validate(ok); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	bad := resource{ID: "not-a-uuid", Link: "notaurl"}
	errs := Validate(bad)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %v", errs)
	}
	for _, badID := range []string{"", "550e8400-e29b-41d4-a716", "550e8400-e29b-41d4-a716-44665544000g", "550E8400-E29B-41D4-A716-446655440000-x"} {
		v := resource{ID: badID, Link: ok.Link}
		if errs := Validate(v); len(errs) == 0 {
			t.Fatalf("ID %q must fail uuid", badID)
		}
	}
	// Uppercase hex is valid UUID.
	if errs := Validate(resource{ID: "550E8400-E29B-41D4-A716-446655440000", Link: ok.Link}); len(errs) != 0 {
		t.Fatalf("uppercase uuid must pass, got %v", errs)
	}
	for _, badURL := range []string{"", "notaurl", "ftp://x.com", "http://", "://missing"} {
		if errs := Validate(resource{ID: ok.ID, Link: badURL}); len(errs) == 0 {
			t.Fatalf("link %q must fail url", badURL)
		}
	}
}

func TestRegisterCustomRule(t *testing.T) {
	RegisterRule("dx-even-test", func(field string, v reflect.Value, _ string) string {
		if v.Kind() != reflect.Int || v.Int()%2 != 0 {
			return "must be even (custom rule)"
		}
		return ""
	})
	type m struct {
		N int `json:"n" validate:"dx-even-test"`
	}
	if errs := Validate(m{N: 3}); len(errs) != 1 || !strings.Contains(errs[0].Message, "even") {
		t.Fatalf("custom rule must fire, got %v", errs)
	}
	if errs := Validate(m{N: 4}); len(errs) != 0 {
		t.Fatalf("custom rule must pass, got %v", errs)
	}
}

func TestParseCoercion(t *testing.T) {
	if v, err := Parse[int]("42"); err != nil || v != 42 {
		t.Fatalf("Parse[int] = %v,%v", v, err)
	}
	if v, err := Parse[bool]("true"); err != nil || !v {
		t.Fatalf("Parse[bool] = %v,%v", v, err)
	}
	if v, err := Parse[string]("hi"); err != nil || v != "hi" {
		t.Fatalf("Parse[string] = %v,%v", v, err)
	}
	if _, err := Parse[int]("abc"); err == nil {
		t.Fatal("Parse[int] abc must fail")
	}
	if _, err := Parse[int8]("999"); err == nil {
		t.Fatal("Parse[int8] overflow must fail")
	}
	if _, err := Parse[float64]("1.5"); err != nil {
		t.Fatalf("Parse[float64] must pass: %v", err)
	}
}

func testCtx(t *testing.T, method, target string) router.Context {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	return router.NewContext(httptest.NewRecorder(), req)
}

func TestBindQueryStrictMalformed(t *testing.T) {
	bad := testCtx(t, "GET", "/x?limit=abc")
	var q listQuery
	err := BindQuery(bad, &q)
	he, ok := grove.AsHttpError(err)
	if !ok || he.StatusCode() != 400 {
		t.Fatalf("malformed limit must be 400 HttpError, got %v", err)
	}

	type ids struct {
		IDs []int `query:"id"`
	}
	mixed := testCtx(t, "GET", "/x?id=1&id=oops")
	var v ids
	if err := BindQuery(mixed, &v); err == nil {
		t.Fatal("malformed slice element must 400")
	}
}

func TestPathAndQueryHelpers(t *testing.T) {
	c := testCtx(t, "GET", "/users/42?limit=10&verbose=true")
	req := c.Request()
	req.SetPathValue("id", "42")

	id, err := Path[int](c, "id")
	if err != nil || id != 42 {
		t.Fatalf("Path[int] = %v,%v", id, err)
	}
	if _, err := Path[int](c, "missing"); err == nil {
		t.Fatal("missing path param must 400")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("missing path param must be 400 HttpError, got %v", err)
	}
	limit, err := Query(c, "limit", 20)
	if err != nil || limit != 10 {
		t.Fatalf("Query limit = %v,%v", limit, err)
	}
	def, err := Query(c, "absent", 20)
	if err != nil || def != 20 {
		t.Fatalf("Query fallback = %v,%v", def, err)
	}
	verbose, err := Query(c, "verbose", false)
	if err != nil || !verbose {
		t.Fatalf("Query bool = %v,%v", verbose, err)
	}
	// Strings accept anything: no coercion failure possible.
	name, err := Query(c, "limit", "zero")
	if err != nil || name != "10" {
		t.Fatalf("Query[string] = %v,%v", name, err)
	}
	bad := testCtx(t, "GET", "/x?limit=abc")
	if _, err := Query(bad, "limit", 20); err == nil {
		t.Fatal("malformed query must 400")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("malformed query must be 400 HttpError, got %v", err)
	}
}
