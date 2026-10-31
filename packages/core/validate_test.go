package grove

import (
	"strings"
	"testing"

	"github.com/brandonbert8/grove/packages/di"
)

func namedProvider(name string) Provider {
	return ProvideNamed(name, di.Singleton, func(*di.Container) (any, error) { return name, nil })
}

func TestValidateValidApp(t *testing.T) {
	auth := &ModuleDef{
		Name:      "auth",
		Providers: []Provider{namedProvider("auth.service")},
		Exports:   []string{"auth.service"},
	}
	users := &ModuleDef{
		Name:      "users",
		Imports:   []*ModuleDef{auth},
		Providers: []Provider{namedProvider("users.service")},
		Exports:   []string{"users.service"},
		BuildControllers: func(*App) ([]ControllerDef, error) {
			return []ControllerDef{{
				Prefix:    "/users",
				Endpoints: []Endpoint{GET("", graphOK)},
			}}, nil
		},
	}

	app := New()
	app.MustRegister(users.AsModule())
	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidateDuplicateModuleName(t *testing.T) {
	app := New()
	app.MustRegister(
		(&ModuleDef{Name: "dup"}).AsModule(),
		(&ModuleDef{Name: "dup"}).AsModule(),
	)
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), `module name "dup"`) {
		t.Fatalf("Validate() = %v, want duplicate module name", err)
	}
}

func TestValidateImportCycle(t *testing.T) {
	a := &ModuleDef{Name: "a"}
	b := &ModuleDef{Name: "b"}
	a.Imports = []*ModuleDef{b}
	b.Imports = []*ModuleDef{a}

	app := New()
	app.MustRegister(a.AsModule())
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "module import cycle") {
		t.Fatalf("Validate() = %v, want import cycle", err)
	}
	if !strings.Contains(err.Error(), "a -> b -> a") && !strings.Contains(err.Error(), "b -> a -> b") {
		t.Fatalf("cycle path missing in %v", err)
	}
}

func TestValidateDuplicateProvider(t *testing.T) {
	app := New()
	app.MustRegister(
		(&ModuleDef{Name: "one", Providers: []Provider{namedProvider("shared")}}).AsModule(),
		(&ModuleDef{Name: "two", Providers: []Provider{namedProvider("shared")}}).AsModule(),
	)
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), `provider "shared" is registered by multiple modules`) {
		t.Fatalf("Validate() = %v, want duplicate provider", err)
	}
}

func TestValidateUnknownExport(t *testing.T) {
	app := New()
	app.MustRegister((&ModuleDef{Name: "users", Exports: []string{"missing"}}).AsModule())
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), `exports unknown provider "missing"`) {
		t.Fatalf("Validate() = %v, want unknown export", err)
	}
}

func TestValidateDuplicateRoute(t *testing.T) {
	app := New()
	app.MustRegister((&ModuleDef{
		Name: "users",
		BuildControllers: func(*App) ([]ControllerDef, error) {
			return []ControllerDef{
				{Prefix: "/users", Endpoints: []Endpoint{GET("", graphOK)}},
				{Prefix: "/users", Endpoints: []Endpoint{GET("", graphOK)}},
			}, nil
		},
	}).AsModule())

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate route GET /users") {
		t.Fatalf("Validate() = %v, want duplicate route", err)
	}
}

func TestValidateOnStartFailsBeforeListening(t *testing.T) {
	app := New()
	app.ValidateOnStart()
	app.MustRegister((&ModuleDef{
		Name: "users",
		BuildControllers: func(*App) ([]ControllerDef, error) {
			return []ControllerDef{
				{Prefix: "/users", Endpoints: []Endpoint{GET("", graphOK)}},
				{Prefix: "/users", Endpoints: []Endpoint{GET("", graphOK)}},
			}, nil
		},
	}).AsModule())

	err := app.RunWith(ServerOptions{Addr: "127.0.0.1:0"})
	if err == nil || !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("RunWith() = %v, want validation failure before listen", err)
	}
}
