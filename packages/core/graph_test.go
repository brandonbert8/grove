package grove

import (
	"testing"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

func graphOK(c router.Context) error { return c.JSON(200, map[string]string{"ok": "1"}) }

func TestModuleGraphIncludesImportsProvidersAndControllers(t *testing.T) {
	auth := &ModuleDef{
		Name: "auth",
		Providers: []Provider{
			ProvideNamed("auth.service", di.Singleton, func(*di.Container) (any, error) { return "auth", nil }),
		},
		Exports: []string{"auth.service"},
		BuildControllers: func(*App) ([]ControllerDef, error) {
			return []ControllerDef{{
				Prefix: "/auth",
				Endpoints: []Endpoint{
					GET("/login", graphOK),
				},
			}}, nil
		},
	}
	users := &ModuleDef{
		Name:    "users",
		Imports: []*ModuleDef{auth},
		Providers: []Provider{
			ProvideNamed("users.service", di.Singleton, func(*di.Container) (any, error) { return "users", nil }),
		},
		BuildControllers: func(*App) ([]ControllerDef, error) {
			return []ControllerDef{{
				Prefix: "/users",
				Endpoints: []Endpoint{
					GET("", graphOK),
					POST("/{id}", graphOK),
				},
			}}, nil
		},
	}

	app := New()
	app.MustRegister(users.AsModule())

	graph := app.ModuleGraph()
	if len(graph) != 2 {
		t.Fatalf("graph modules = %d, want 2: %+v", len(graph), graph)
	}
	if graph[0].Name != "auth" || graph[1].Name != "users" {
		t.Fatalf("graph order = %q, %q; want auth, users", graph[0].Name, graph[1].Name)
	}

	authInfo := graph[0]
	if len(authInfo.Providers) != 1 || authInfo.Providers[0].Name != "auth.service" || authInfo.Providers[0].Lifetime != "singleton" {
		t.Fatalf("auth providers = %+v", authInfo.Providers)
	}
	if len(authInfo.Exports) != 1 || authInfo.Exports[0] != "auth.service" {
		t.Fatalf("auth exports = %+v", authInfo.Exports)
	}
	if len(authInfo.Controllers) != 1 || authInfo.Controllers[0].Prefix != "/auth" {
		t.Fatalf("auth controllers = %+v", authInfo.Controllers)
	}
	if len(authInfo.Controllers[0].Routes) != 1 || authInfo.Controllers[0].Routes[0] != "GET /auth/login" {
		t.Fatalf("auth routes = %+v", authInfo.Controllers[0].Routes)
	}

	usersInfo := graph[1]
	if len(usersInfo.Imports) != 1 || usersInfo.Imports[0] != "auth" {
		t.Fatalf("users imports = %+v", usersInfo.Imports)
	}
	if len(usersInfo.Controllers) != 1 {
		t.Fatalf("users controllers = %+v", usersInfo.Controllers)
	}
	if got := usersInfo.Controllers[0].Routes; len(got) != 2 || got[0] != "GET /users" || got[1] != "POST /users/{id}" {
		t.Fatalf("users routes = %+v", got)
	}
}

func TestModuleGraphIncludesLegacyModuleFunc(t *testing.T) {
	app := New()
	app.MustRegister(NewModule("legacy", func(*App) error { return nil }))

	graph := app.ModuleGraph()
	if len(graph) != 1 || graph[0].Name != "legacy" {
		t.Fatalf("graph = %+v", graph)
	}
}

func TestModuleGraphAppliesGlobalPrefix(t *testing.T) {
	app := New()
	app.SetGlobalPrefix("/v1")
	app.MustRegister((&ModuleDef{
		Name: "users",
		BuildControllers: func(*App) ([]ControllerDef, error) {
			return []ControllerDef{{
				Prefix: "/users",
				Endpoints: []Endpoint{
					GET("/{id}", graphOK),
				},
			}}, nil
		},
	}).AsModule())

	graph := app.ModuleGraph()
	if len(graph) != 1 || len(graph[0].Controllers) != 1 {
		t.Fatalf("graph = %+v", graph)
	}
	if got := graph[0].Controllers[0].Prefix; got != "/v1/users" {
		t.Fatalf("controller prefix = %q, want /v1/users", got)
	}
	if got := graph[0].Controllers[0].Routes[0]; got != "GET /v1/users/{id}" {
		t.Fatalf("route = %q", got)
	}
}
