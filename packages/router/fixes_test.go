package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// countWriter counts WriteHeader calls to prove single-write semantics.
type countWriter struct {
	http.ResponseWriter
	calls int
}

func (w *countWriter) WriteHeader(code int) {
	w.calls++
	w.ResponseWriter.WriteHeader(code)
}

func TestHandleInvalidMethodPanicsWithoutPoisoning(t *testing.T) {
	r := New()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for unsupported method")
			}
		}()
		r.Handle("BREW", "/x", func(c Context) error { return c.NoContent(204) })
	}()
	// The mutex must not be poisoned: the router keeps working.
	r.GET("/ok", func(c Context) error { return c.String(200, "ok") })
	rec := doRequest(t, r, "GET", "/ok")
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("router poisoned: GET /ok = %d %q", rec.Code, rec.Body.String())
	}
	if len(r.Routes()) != 2 {
		t.Fatalf("routes = %d, want 2 (GET + auto HEAD; failed BREW leaves no trace)", len(r.Routes()))
	}
	for _, ri := range r.Routes() {
		if ri.Method == "BREW" {
			t.Fatal("failed registration must leave no trace")
		}
	}
}

func TestStatusChainingSingleWrite(t *testing.T) {
	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	cw := &countWriter{ResponseWriter: rec}
	c := NewContext(cw, req)
	if err := c.Status(201).JSON(200, map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	if cw.calls != 1 {
		t.Fatalf("WriteHeader calls = %d, want exactly 1", cw.calls)
	}
	if rec.Code != 201 {
		t.Fatalf("stashed status = %d, want 201", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	c2 := NewContext(rec2, httptest.NewRequest("GET", "/x", nil))
	if err := c2.JSON(200, map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	if rec2.Code != 200 {
		t.Fatalf("plain status = %d, want 200", rec2.Code)
	}
}

func TestArbitrarySupportedMethods(t *testing.T) {
	r := New()
	r.Handle("TRACE", "/t", func(c Context) error { return c.NoContent(200) })
	req := httptest.NewRequest("TRACE", "/t", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("TRACE /t = %d, want 200", rec.Code)
	}
}

func TestGetMirrorsHead(t *testing.T) {
	r := New()
	r.GET("/h", func(c Context) error { return c.String(200, "body") })
	req := httptest.NewRequest("HEAD", "/h", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("HEAD /h = %d, want 200", rec.Code)
	}
	// Note: httptest records the written body; the real net/http server
	// strips bodies on HEAD responses in production.
	found := false
	for _, ri := range r.Routes() {
		if ri.Method == "HEAD" && ri.Path == "/h" {
			found = true
		}
	}
	if !found {
		t.Fatal("mirror HEAD must be visible in Routes()")
	}
}

func TestExplicitHeadWinsOverMirror(t *testing.T) {
	r := New()
	r.GET("/h", func(c Context) error { return c.String(200, "get") })
	r.Handle("HEAD", "/h", func(c Context) error { return c.NoContent(204) })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("HEAD", "/h", nil))
	if rec.Code != 204 {
		t.Fatalf("explicit HEAD = %d, want 204", rec.Code)
	}
	n := 0
	for _, ri := range r.Routes() {
		if ri.Method == "HEAD" && ri.Path == "/h" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("HEAD entries = %d, want exactly 1 (upsert, no phantom)", n)
	}
}

func TestDuplicateReregisterReplaces(t *testing.T) {
	r := New()
	r.GET("/d", func(c Context) error { return c.String(200, "one") })
	r.GET("/d", func(c Context) error { return c.String(200, "two") })
	rec := doRequest(t, r, "GET", "/d")
	if rec.Body.String() != "two" {
		t.Fatalf("last wins: body = %q", rec.Body.String())
	}
	n := 0
	for _, ri := range r.Routes() {
		if ri.Method == "GET" && ri.Path == "/d" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("GET entries = %d, want 1", n)
	}
}
