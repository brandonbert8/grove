package grovtest

import (
	"net/http/httptest"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/config"
	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

type testOptions struct {
	modules    []grove.Module
	overrides  map[string]any
	middleware []router.Middleware
}

// TestOption customizes TestingModule.
type TestOption func(*testOptions)

// Override replaces provider T with a mock (overrideProvider equivalent).
func Override[T any](instance T) TestOption {
	return func(o *testOptions) {
		if o.overrides == nil {
			o.overrides = map[string]any{}
		}
		o.overrides[di.KeyFor[T]()] = instance
	}
}

// WithModules registers extra modules in the test app.
func WithModules(mods ...grove.Module) TestOption {
	return func(o *testOptions) { o.modules = append(o.modules, mods...) }
}

// WithMiddleware applies test middleware (guards, interceptors).
func WithMiddleware(mw ...router.Middleware) TestOption {
	return func(o *testOptions) { o.middleware = append(o.middleware, mw...) }
}

// TestingModule builds an isolated App for tests (NestJS
// Test.createTestingModule equivalent):
//
//	app := grovtest.TestingModule(t, users.UsersModule,
//	    grovtest.Override[*users.Service](mockSvc))
func TestingModule(t testing.TB, mod *grove.ModuleDef, opts ...TestOption) *grove.App {
	t.Helper()
	var mods []grove.Module
	if mod != nil {
		mods = append(mods, mod.AsModule())
	}
	o := collectOptions(opts...)
	return testingModules(t, mods, o)
}

// TestingModules builds an isolated App from any parade of Modules
// (ModuleDef.AsModule() or grove.NewModule funcs). It is the escape
// hatch for hello-style func modules that TestingModule(t, *ModuleDef)
// cannot express:
//
//	app := grovtest.TestingModules(t, grove.NewModule("hello", fn))
func TestingModules(t testing.TB, mods []grove.Module, opts ...TestOption) *grove.App {
	t.Helper()
	return testingModules(t, mods, collectOptions(opts...))
}

func collectOptions(opts ...TestOption) testOptions {
	o := testOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func testingModules(t testing.TB, mods []grove.Module, o testOptions) *grove.App {
	t.Helper()
	cfg, err := config.Load(config.WithOverrides(map[string]string{
		"ENV": "test", "LOG_LEVEL": "error",
	}))
	if err != nil {
		t.Fatalf("grovtest: load test config: %v", err)
	}
	app := grove.New(grove.WithConfig(cfg))
	// Overrides first: BuildControllers resolves (and caches) lazy
	// singletons during Register, so replacing after would be ignored.
	for name, val := range o.overrides {
		app.Container.Replace(name, val)
	}
	for _, m := range mods {
		if m == nil {
			continue
		}
		if err := app.Register(m); err != nil {
			t.Fatalf("grovtest: register module: %v", err)
		}
	}
	for _, m := range o.modules {
		if err := app.Register(m); err != nil {
			t.Fatalf("grovtest: register module: %v", err)
		}
	}
	if len(o.middleware) > 0 {
		app.Router.Use(o.middleware...)
	}
	return app
}

// RequireProblem asserts a Grove error envelope carries want status
// and a non-empty error message, returning the decoded envelope.
func RequireProblem(t testing.TB, rec *httptest.ResponseRecorder, want int) map[string]any {
	t.Helper()
	RequireStatus(t, rec, want)
	body := Decode[map[string]any](t, rec)
	if _, ok := body["error"]; !ok {
		t.Fatalf("grovtest: problem body missing \"error\" (body %q)", rec.Body.String())
	}
	return body
}

// RequireValidationError asserts a 422 envelope whose details name the
// failed field (the ValidationPipe-failure equivalent), returning the
// decoded details list:
//
//	grovtest.RequireValidationError(t,
//	    cli.Post(t, "/users", map[string]string{"name": "x"}), "name")
func RequireValidationError(t testing.TB, rec *httptest.ResponseRecorder, field string) []any {
	t.Helper()
	body := RequireProblem(t, rec, 422)
	raw, ok := body["details"]
	if !ok {
		t.Fatalf("grovtest: 422 body missing \"details\" (body %q)", rec.Body.String())
	}
	details, ok := raw.([]any)
	if !ok || len(details) == 0 {
		t.Fatalf("grovtest: 422 details empty, want field %q (body %q)", field, rec.Body.String())
	}
	for _, d := range details {
		if m, ok := d.(map[string]any); ok && m["field"] == field {
			return details
		}
	}
	t.Fatalf("grovtest: 422 details miss field %q (body %q)", field, rec.Body.String())
	return details
}
