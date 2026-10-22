package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doRequest(t *testing.T, r Router, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestMethodRegistration(t *testing.T) {
	r := New()
	ok := func(c Context) error { return c.String(200, "ok") }
	r.GET("/g", ok)
	r.POST("/p", ok)
	r.PUT("/u", ok)
	r.DELETE("/d", ok)
	r.PATCH("/pa", ok)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/g"}, {"POST", "/p"}, {"PUT", "/u"}, {"DELETE", "/d"}, {"PATCH", "/pa"},
	} {
		rec := doRequest(t, r, tc.method, tc.path)
		if rec.Code != 200 {
			t.Fatalf("%s %s = %d, want 200", tc.method, tc.path, rec.Code)
		}
	}
	if len(r.Routes()) != 6 {
		t.Fatalf("want 6 routes (5 explicit + auto HEAD on GET), got %d", len(r.Routes()))
	}
}

func TestPathParamsAndJSON(t *testing.T) {
	r := New()
	r.GET("/users/{id}", func(c Context) error {
		return c.JSON(200, map[string]string{"id": c.Param("id")})
	})
	rec := doRequest(t, r, "GET", "/users/42")
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != "42" {
		t.Fatalf("id = %q, want 42", body["id"])
	}
}

func TestMiddlewareOrder(t *testing.T) {
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
	r.GET("/m", func(c Context) error { return c.NoContent(204) }, mk("route"))

	rec := doRequest(t, r, "GET", "/m")
	if rec.Code != 204 {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	want := []string{"global-before", "route-before", "route-after", "global-after"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestGroupPrefix(t *testing.T) {
	r := New()
	g := r.Group("/api", func(next HandlerFunc) HandlerFunc { return next })
	g.GET("/hello", func(c Context) error { return c.String(200, "hi") })

	if rec := doRequest(t, r, "GET", "/api/hello"); rec.Code != 200 {
		t.Fatalf("grouped route status = %d, want 200", rec.Code)
	}
	if rec := doRequest(t, r, "GET", "/hello"); rec.Code != http.StatusNotFound {
		t.Fatalf("ungrouped path status = %d, want 404", rec.Code)
	}
}

func TestHandlerErrorBecomes500(t *testing.T) {
	r := New()
	r.GET("/fail", func(c Context) error { return errBoom })
	rec := doRequest(t, r, "GET", "/fail")
	if rec.Code != 500 {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// TestOversizedBodyIs413 guards the P1 fix: a request body larger than the
// cap yields 413 Payload Too Large (not 400).
func TestOversizedBodyIs413(t *testing.T) {
	r := New()
	r.POST("/upload", func(c Context) error {
		var v struct {
			Data string `json:"data"`
		}
		if err := c.Body(&v); err != nil {
			return err
		}
		return c.String(200, "ok")
	})
	big := strings.Repeat("a", 2*1024*1024) // 2 MiB > 1 MiB cap
	req := httptest.NewRequest("POST", "/upload", strings.NewReader(big))
	req.ContentLength = int64(len(big))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

// TestMethodNotAllowedIncludesAllowHeader guards the P1 fix: a 405
// advertises the valid methods in an Allow header (RFC 9110 §15.5.6).
func TestMethodNotAllowedIncludesAllowHeader(t *testing.T) {
	r := New()
	r.GET("/foo", func(c Context) error { return c.String(200, "ok") })
	rec := doRequest(t, r, "POST", "/foo")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	allow := rec.Header().Get("Allow")
	if allow == "" {
		t.Fatal("missing Allow header on 405")
	}
	if !strings.Contains(allow, "GET") {
		t.Errorf("Allow = %q, want GET", allow)
	}
}
