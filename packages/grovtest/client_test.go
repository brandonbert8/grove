package grovtest

import (
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/pipes"
	"github.com/brandonbert8/grove/packages/router"
)

func testApp() *grove.App {
	app := grove.New()
	app.Router.GET("/users/{id}", func(c router.Context) error {
		id, err := pipes.Path[int](c, "id")
		if err != nil {
			return err
		}
		return c.JSON(200, map[string]int{"id": id})
	})
	app.Router.POST("/users", func(c router.Context) error {
		var in struct {
			Name string `json:"name" validate:"required,min=2"`
		}
		if err := pipes.ValidateBody(c, &in); err != nil {
			return err
		}
		return c.JSON(201, in)
	})
	return app
}

func TestGetAndDecode(t *testing.T) {
	cli := New(testApp())
	rec := cli.Get(t, "/users/7")
	RequireStatus(t, rec, 200)
	body := Decode[map[string]int](t, rec)
	if body["id"] != 7 {
		t.Fatalf("id = %v, want 7", body)
	}
}

func TestPostValidationDetailsSurvive(t *testing.T) {
	cli := New(testApp())
	rec := cli.Post(t, "/users", map[string]string{"name": "x"})
	RequireStatus(t, rec, 422)
	body := Decode[map[string]any](t, rec)
	details, ok := body["details"].([]any)
	if !ok || len(details) == 0 {
		t.Fatalf("422 must carry details, got %v", body)
	}
	first, _ := details[0].(map[string]any)
	if first["field"] != "name" {
		t.Fatalf("details[0] = %v, want field=name", first)
	}
}

func TestHeadersAndBearer(t *testing.T) {
	app := grove.New()
	app.Router.GET("/me", func(c router.Context) error {
		return c.JSON(200, map[string]string{"auth": c.Header("Authorization")})
	})
	cli := New(app).Bearer("tok-1")
	rec := cli.Get(t, "/me")
	RequireStatus(t, rec, 200)
	if body := Decode[map[string]string](t, rec); body["auth"] != "Bearer tok-1" {
		t.Fatalf("auth = %q", body["auth"])
	}
}
