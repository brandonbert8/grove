package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (body %q)", err, rec.Body.String())
	}
	return body
}

func TestChiPathParams(t *testing.T) {
	r := New()
	r.GET("/users/{id}", func(c Context) error {
		return c.JSON(200, map[string]string{"id": c.Param("id")})
	})
	r.GET("/orgs/{org}/repos/{repo}", func(c Context) error {
		return c.JSON(200, map[string]string{"org": c.Param("org"), "repo": c.Param("repo")})
	})

	rec := doRequest(t, r, "GET", "/users/42")
	var one map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &one); err != nil || one["id"] != "42" {
		t.Fatalf("id = %v, want 42 (body %q)", one, rec.Body.String())
	}

	rec = doRequest(t, r, "GET", "/orgs/acme/repos/grove")
	var two map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &two); err != nil {
		t.Fatal(err)
	}
	if two["org"] != "acme" || two["repo"] != "grove" {
		t.Fatalf("params = %v, want org=acme repo=grove", two)
	}
}

func TestChiQueryParams(t *testing.T) {
	r := New()
	r.GET("/search", func(c Context) error {
		return c.JSON(200, map[string]string{
			"q":    c.Query("q"),
			"page": c.QueryOr("page", "1"),
		})
	})
	rec := doRequest(t, r, "GET", "/search?q=grove")
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["q"] != "grove" || body["page"] != "1" {
		t.Fatalf("query = %v, want q=grove page=1", body)
	}
}

func TestChiNestedGroups(t *testing.T) {
	r := New()
	api := r.Group("/api")
	v1 := api.Group("/v1")
	v1.GET("/hello", func(c Context) error { return c.String(200, "hi") })

	if rec := doRequest(t, r, "GET", "/api/v1/hello"); rec.Code != 200 {
		t.Fatalf("nested group status = %d, want 200", rec.Code)
	}
	if rec := doRequest(t, r, "GET", "/api/hello"); rec.Code != http.StatusNotFound {
		t.Fatalf("partial prefix status = %d, want 404", rec.Code)
	}
}

func TestChiNestedGroupMiddlewareOrder(t *testing.T) {
	r := New()
	var order []string
	mk := func(name string) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(c Context) error {
				order = append(order, name+"-before")
				err := next(c)
				order = append(order, name+"-after")
				return err
			}
		}
	}
	r.Use(mk("global"))
	api := r.Group("/api", mk("outer"))
	v1 := api.Group("/v1", mk("inner"))
	v1.GET("/x", func(c Context) error { return c.NoContent(204) }, mk("route"))

	if rec := doRequest(t, r, "GET", "/api/v1/x"); rec.Code != 204 {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	want := []string{
		"global-before", "outer-before", "inner-before", "route-before",
		"route-after", "inner-after", "outer-after", "global-after",
	}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestChiGlobalUseAfterGroupApplies(t *testing.T) {
	r := New()
	var ran []string
	mk := func(name string) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(c Context) error {
				ran = append(ran, name)
				return next(c)
			}
		}
	}
	g := r.Group("/g")
	r.Use(mk("late-global")) // registered after Group, before Handle
	g.GET("/x", func(c Context) error { return c.NoContent(204) })

	if rec := doRequest(t, r, "GET", "/g/x"); rec.Code != 204 {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(ran) != 1 || ran[0] != "late-global" {
		t.Fatalf("global middleware ran = %v, want [late-global]", ran)
	}
}

func TestChiNotFoundIsJSON(t *testing.T) {
	r := New()
	r.GET("/exists", func(c Context) error { return c.NoContent(204) })

	rec := doRequest(t, r, "GET", "/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	body := decodeJSON(t, rec)
	if body["status"] != float64(404) || body["error"] == "" {
		t.Fatalf("body = %v, want {error,status:404}", body)
	}
}

func TestChiMethodNotAllowedIsJSON(t *testing.T) {
	r := New()
	r.GET("/only-get", func(c Context) error { return c.NoContent(204) })

	req := httptest.NewRequest(http.MethodPost, "/only-get", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	body := decodeJSON(t, rec)
	if body["status"] != float64(405) {
		t.Fatalf("body = %v, want status 405", body)
	}
}

func TestChiEmptyPathMountsAtPrefix(t *testing.T) {
	r := New()
	g := r.Group("/users")
	g.GET("", func(c Context) error { return c.String(200, "list") })

	if rec := doRequest(t, r, "GET", "/users"); rec.Code != 200 {
		t.Fatalf("GET /users = %d, want 200", rec.Code)
	}
}

func TestChiArbitraryMethodViaHandle(t *testing.T) {
	r := New()
	r.Handle("HEAD", "/h", func(c Context) error { return c.NoContent(200) })
	r.Handle("OPTIONS", "/h", func(c Context) error { return c.NoContent(204) })

	req := httptest.NewRequest(http.MethodHead, "/h", nil)
	if rec := httptest.NewRecorder(); true {
		r.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("HEAD /h = %d, want 200", rec.Code)
		}
	}
	req = httptest.NewRequest(http.MethodOptions, "/h", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("OPTIONS /h = %d, want 204", rec.Code)
	}
}

func TestChiWildcardPassthrough(t *testing.T) {
	r := New()
	r.GET("/files/*", func(c Context) error {
		return c.String(200, c.Param("*"))
	})
	rec := doRequest(t, r, "GET", "/files/a/b")
	if rec.Code != 200 || rec.Body.String() != "a/b" {
		t.Fatalf("wildcard = %d %q, want 200 a/b", rec.Code, rec.Body.String())
	}
}

func TestFromHTTPAdapter(t *testing.T) {
	r := New()
	std := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Std", "1")
			next.ServeHTTP(w, req)
		})
	}
	r.GET("/s", func(c Context) error { return c.String(200, "ok") }, FromHTTP(std))

	rec := doRequest(t, r, "GET", "/s")
	if rec.Code != 200 || rec.Header().Get("X-Std") != "1" {
		t.Fatalf("adapter = %d headers=%v, want 200 with X-Std", rec.Code, rec.Header())
	}
}
