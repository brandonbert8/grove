package pipes

import (
	"net/http/httptest"
	"strings"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

type address struct {
	Zip string `json:"zip" validate:"required,len=5"`
}

type signup struct {
	Name    string   `json:"name" validate:"required,min=2,max=80"`
	Email   string   `json:"email" validate:"required,email"`
	Age     int      `json:"age" validate:"gte=18,lte=120"`
	Role    string   `json:"role" validate:"oneof=admin user"`
	Tags    []string `json:"tags" validate:"max=3"`
	Address address  `json:"address"`
}

func validSignup() signup {
	return signup{
		Name: "Ada", Email: "ada@example.com", Age: 30, Role: "admin",
		Tags: []string{"a"}, Address: address{Zip: "12345"},
	}
}

func TestValidateOK(t *testing.T) {
	if errs := Validate(validSignup()); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if errs := Validate(&signup{Name: "Ada", Email: "a@b.co", Age: 18, Role: "user", Address: address{Zip: "12345"}}); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateFailures(t *testing.T) {
	v := signup{Name: "A", Email: "not-an-email", Age: 12, Role: "root",
		Tags: []string{"a", "b", "c", "d"}, Address: address{}}
	errs := Validate(v)
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, want := range []string{"name", "email", "age", "role", "tags", "address.zip"} {
		if !fields[want] {
			t.Errorf("expected failure for %q, got %v", want, errs)
		}
	}
}

func TestValidateRequired(t *testing.T) {
	var v signup
	errs := Validate(v)
	if len(errs) == 0 {
		t.Fatal("expected required failures")
	}
}

func TestValidateBodyOK(t *testing.T) {
	body := `{"name":"Ada","email":"ada@example.com","age":30,"role":"user","address":{"zip":"12345"}}`
	req := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	rec := httptest.NewRecorder()
	c := router.NewContext(rec, req)
	var v signup
	if err := ValidateBody(c, &v); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if v.Name != "Ada" {
		t.Fatalf("name = %q", v.Name)
	}
}

func TestValidateBodyBadJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"name":`))
	c := router.NewContext(httptest.NewRecorder(), req)
	var v signup
	err := ValidateBody(c, &v)
	he, ok := grove.AsHttpError(err)
	if !ok || he.StatusCode() != 400 {
		t.Fatalf("expected 400 HttpError, got %v", err)
	}
}

func TestValidateBodyUnprocessable(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"name":"x"}`))
	c := router.NewContext(httptest.NewRecorder(), req)
	var v signup
	err := ValidateBody(c, &v)
	he, ok := grove.AsHttpError(err)
	if !ok || he.StatusCode() != 422 {
		t.Fatalf("expected 422 HttpError, got %v", err)
	}
}

type listQuery struct {
	Q     string `query:"q"`
	Limit int    `query:"limit" validate:"gte=1,lte=100"`
	Exact bool   `query:"exact"`
}

func TestBindQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/x?q=hello&limit=10&exact=true", nil)
	c := router.NewContext(httptest.NewRecorder(), req)
	var q listQuery
	if err := BindQuery(c, &q); err != nil {
		t.Fatal(err)
	}
	if q.Q != "hello" || q.Limit != 10 || !q.Exact {
		t.Fatalf("query = %+v", q)
	}
}

func TestBindQueryValidationFails(t *testing.T) {
	req := httptest.NewRequest("GET", "/x?limit=500", nil)
	c := router.NewContext(httptest.NewRecorder(), req)
	var q listQuery
	err := BindQuery(c, &q)
	he, ok := grove.AsHttpError(err)
	if !ok || he.StatusCode() != 422 {
		t.Fatalf("expected 422 HttpError, got %v", err)
	}
}
