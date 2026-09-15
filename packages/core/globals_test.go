package grove

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/router"
)

func TestUseGuardsGlobal(t *testing.T) {
	app := New()
	app.Router.GET("/open", func(c router.Context) error { return c.NoContent(204) })
	app.UseGuards(GuardFunc(func(c router.Context) error { return Unauthorized("") }))
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/open", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestUseInterceptorsHeader(t *testing.T) {
	app := New()
	app.Router.GET("/h", func(c router.Context) error { return c.NoContent(204) })
	app.UseInterceptors(func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			c.ResponseWriter().Header().Set("X-App", "grove")
			return next(c)
		}
	})
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/h", nil))
	if rec.Header().Get("X-App") != "grove" {
		t.Fatalf("missing interceptor header: %v", rec.Header())
	}
}

func TestWrapDataEnvelopesJSON(t *testing.T) {
	app := New()
	app.Router.GET("/u", func(c router.Context) error {
		return c.JSON(200, map[string]string{"name": "Ada"})
	})
	app.UseInterceptors(WrapData())
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/u", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok || data["name"] != "Ada" {
		t.Fatalf("envelope body = %s", rec.Body.String())
	}
}

func TestWrapDataSkipsErrors(t *testing.T) {
	app := New()
	app.Router.GET("/boom", func(c router.Context) error { return NotFound("nope") })
	app.UseInterceptors(WrapData())
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/boom", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, hasData := body["data"]; hasData {
		t.Fatalf("errors must not be enveloped: %s", rec.Body.String())
	}
}

func TestMapResponseCustom(t *testing.T) {
	app := New()
	app.Router.GET("/m", func(c router.Context) error {
		return c.JSON(200, map[string]string{"a": "b"})
	})
	app.UseInterceptors(MapResponse(func(status int, body []byte) (int, []byte) {
		return 201, body
	}))
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/m", nil))
	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

func TestOKAndCreatedHelpers(t *testing.T) {
	app := New()
	app.Router.GET("/ok", func(c router.Context) error { return OK(c, "hi") })
	app.Router.POST("/mk", func(c router.Context) error { return Created(c, "hi") })
	for path, want := range map[string]int{"/ok": 200, "/mk": 201} {
		method := "GET"
		if want == 201 {
			method = "POST"
		}
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		if rec.Code != want {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, want)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["data"] != "hi" {
			t.Fatalf("%s body = %s", path, rec.Body.String())
		}
	}
}
