package pipes

import (
	"net/http/httptest"
	"strings"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

type bodyDTO struct {
	Name string `json:"name" validate:"required,min=2,max=80"`
}

func bodyCtx(t *testing.T, payload string) router.Context {
	t.Helper()
	req := httptest.NewRequest("POST", "/x", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return router.NewContext(httptest.NewRecorder(), req)
}

func TestBodyValueSemantics(t *testing.T) {
	in, err := Body[bodyDTO](bodyCtx(t, `{"name":"Ada"}`))
	if err != nil {
		t.Fatal(err)
	}
	if in.Name != "Ada" {
		t.Fatalf("name = %q", in.Name)
	}
}

func TestBodyValidationErrors(t *testing.T) {
	// Malformed JSON is 400.
	if _, err := Body[bodyDTO](bodyCtx(t, `{oops`)); err == nil {
		t.Fatal("malformed JSON must fail")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 400 {
		t.Fatalf("malformed must be 400, got %v", err)
	}
	// Rule violation is 422 with field details.
	if _, err := Body[bodyDTO](bodyCtx(t, `{"name":"x"}`)); err == nil {
		t.Fatal("short name must fail")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("violation must be 422, got %v", err)
	}
}

func TestBodyStrictFlagsUnknownRules(t *testing.T) {
	type odd struct {
		X string `json:"x" validate:"frobnicator"`
	}
	// v0.3: Body is strict by default (fail-closed on unknown rules).
	if _, err := Body[odd](bodyCtx(t, `{"x":"y"}`)); err == nil {
		t.Fatal("default Body must flag unknown rules (strict by default)")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("strict violation must be 422, got %v", err)
	}
	if _, err := Body[odd](bodyCtx(t, `{"x":"y"}`), Strict()); err == nil {
		t.Fatal("strict Body must flag unknown rules")
	} else if he, ok := grove.AsHttpError(err); !ok || he.StatusCode() != 422 {
		t.Fatalf("strict violation must be 422, got %v", err)
	}
	if _, err := Body[odd](bodyCtx(t, `{"x":"y"}`), Lenient()); err != nil {
		t.Fatalf("lenient Body must ignore unknown rules, got %v", err)
	}
}
