package grove

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brandonbert8/grove/packages/router"
)

func TestStartHookFailureAbortsRun(t *testing.T) {
	app := New()
	app.OnStart(func(ctx context.Context) error { return errors.New("db down") })
	// Failing starts return before the port is even bound.
	if err := app.Run(":0"); err == nil {
		t.Fatal("Run must fail when a start hook fails")
	}
}

func TestStartHooksRunInOrder(t *testing.T) {
	app := New()
	var order []string
	app.OnStart(
		func(ctx context.Context) error { order = append(order, "a"); return nil },
		func(ctx context.Context) error { order = append(order, "b"); return nil },
	)
	// Exercise the hook chain without binding: replicate Run's start phase.
	for i, fn := range app.starts {
		if err := fn(context.Background()); err != nil {
			t.Fatalf("hook %d: %v", i, err)
		}
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("order = %v", order)
	}
}

func TestStopHooksRunReverseAndBestEffort(t *testing.T) {
	app := New()
	var order []string
	app.OnStop(
		func(ctx context.Context) error { order = append(order, "a"); return nil },
		func(ctx context.Context) error { order = append(order, "b"); return errors.New("bad closer") },
		func(ctx context.Context) error { order = append(order, "c"); return nil },
	)
	app.stop(context.Background())
	want := []string{"c", "b", "a"} // reverse, and c still ran after b failed
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestHealthModule(t *testing.T) {
	app := New()
	app.MustRegister(HealthModule("").AsModule())
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q", rec.Body.String())
	}

	custom := New()
	custom.MustRegister(HealthModule("/ping").AsModule())
	req = httptest.NewRequest("GET", "/ping", nil)
	rec = httptest.NewRecorder()
	custom.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("custom path status = %d, want 200", rec.Code)
	}
}

func TestDocsRegistry(t *testing.T) {
	app := New()
	mod := &ModuleDef{
		Name: "docs",
		Controllers: []ControllerDef{{
			Prefix: "/items",
			Tags:   []string{"items"},
			Endpoints: []Endpoint{
				{Method: "GET", Path: "", Handler: func(c router.Context) error { return c.NoContent(204) }, Summary: "List"},
				{Method: "GET", Path: "/{id}", Handler: func(c router.Context) error { return c.NoContent(204) }, Tags: []string{"single"}, Deprecated: true},
			},
		}},
	}
	if err := app.Register(mod.AsModule()); err != nil {
		t.Fatal(err)
	}
	docs := app.Docs()
	if len(docs) != 2 {
		t.Fatalf("docs = %v, want 2 entries", docs)
	}
	if docs[0].Path != "/items" || docs[0].Method != "GET" || docs[0].Summary != "List" {
		t.Fatalf("docs[0] = %+v", docs[0])
	}
	if len(docs[0].Tags) != 1 || docs[0].Tags[0] != "items" {
		t.Fatalf("docs[0].Tags = %v", docs[0].Tags)
	}
	if docs[1].Path != "/items/{id}" || !docs[1].Deprecated {
		t.Fatalf("docs[1] = %+v", docs[1])
	}
	// Controller + endpoint tags merge, controller first.
	if len(docs[1].Tags) != 2 || docs[1].Tags[0] != "items" || docs[1].Tags[1] != "single" {
		t.Fatalf("docs[1].Tags = %v", docs[1].Tags)
	}
}
