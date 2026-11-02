package grove

import (
	"strings"
	"testing"

	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/router"
)

type isoService struct{ name string }

func isoProvider(name string) Provider {
	return ProvideValue(isoService{name: name})
}

func TestIsolatedModuleResolvesImportedExport(t *testing.T) {
	app := New()
	app.EnableModuleIsolation()

	var got string
	auth := &ModuleDef{
		Name:      "auth",
		Providers: []Provider{isoProvider("auth")},
		Exports:   []string{di.KeyFor[isoService]()},
	}
	users := &ModuleDef{
		Name:    "users",
		Imports: []*ModuleDef{auth},
		Build: func(m *ModuleContext) error {
			svc, err := Inject[isoService](m)
			if err != nil {
				return err
			}
			got = svc.name
			return nil
		},
	}
	if err := app.Register(users.AsModule()); err != nil {
		t.Fatal(err)
	}
	if got != "auth" {
		t.Fatalf("resolved %q, want auth", got)
	}
}

func TestIsolatedModuleRejectsUnexportedProvider(t *testing.T) {
	app := New()
	app.EnableModuleIsolation()

	auth := &ModuleDef{
		Name:      "auth",
		Providers: []Provider{isoProvider("auth")},
	}
	billing := &ModuleDef{
		Name: "billing",
		Build: func(m *ModuleContext) error {
			_, err := Inject[isoService](m)
			return err
		},
	}
	if err := app.Register(auth.AsModule()); err != nil {
		t.Fatal(err)
	}
	err := app.Register(billing.AsModule())
	if err == nil || !strings.Contains(err.Error(), "cannot resolve provider") {
		t.Fatalf("Register() = %v, want visibility error", err)
	}
}

func TestIsolatedModuleRejectsProviderFromUnimportedModule(t *testing.T) {
	app := New()
	app.EnableModuleIsolation()

	auth := &ModuleDef{
		Name:      "auth",
		Providers: []Provider{isoProvider("auth")},
		Exports:   []string{di.KeyFor[isoService]()},
	}
	billing := &ModuleDef{
		Name: "billing",
		Build: func(m *ModuleContext) error {
			_, err := Inject[isoService](m)
			return err
		},
	}
	if err := app.Register(auth.AsModule()); err != nil {
		t.Fatal(err)
	}
	err := app.Register(billing.AsModule())
	if err == nil || !strings.Contains(err.Error(), "cannot resolve provider") {
		t.Fatalf("Register() = %v, want visibility error", err)
	}
}

func TestGlobalModuleVisibleWithoutImport(t *testing.T) {
	app := New()
	app.EnableModuleIsolation()

	cfg := &ModuleDef{
		Name:      "config",
		Global:    true,
		Providers: []Provider{isoProvider("global")},
		Exports:   []string{di.KeyFor[isoService]()},
	}
	var got string
	users := &ModuleDef{
		Name: "users",
		Build: func(m *ModuleContext) error {
			svc, err := Inject[isoService](m)
			if err != nil {
				return err
			}
			got = svc.name
			return nil
		},
	}
	if err := app.Register(cfg.AsModule()); err != nil {
		t.Fatal(err)
	}
	if err := app.Register(users.AsModule()); err != nil {
		t.Fatal(err)
	}
	if got != "global" {
		t.Fatalf("resolved %q, want global", got)
	}
}

func TestNonIsolatedModulesKeepLegacyVisibility(t *testing.T) {
	app := New()

	auth := &ModuleDef{
		Name:      "auth",
		Providers: []Provider{isoProvider("auth")},
	}
	var got string
	users := &ModuleDef{
		Name: "users",
		Build: func(m *ModuleContext) error {
			svc, err := Inject[isoService](m)
			if err != nil {
				return err
			}
			got = svc.name
			return nil
		},
	}
	if err := app.Register(auth.AsModule()); err != nil {
		t.Fatal(err)
	}
	if err := app.Register(users.AsModule()); err != nil {
		t.Fatal(err)
	}
	if got != "auth" {
		t.Fatalf("resolved %q, want auth", got)
	}
}

func TestIsolatedModuleDuplicateProviderKeyFails(t *testing.T) {
	app := New()
	app.EnableModuleIsolation()

	a := &ModuleDef{Name: "a", Providers: []Provider{isoProvider("a")}}
	b := &ModuleDef{Name: "b", Providers: []Provider{isoProvider("b")}}
	if err := app.Register(a.AsModule()); err != nil {
		t.Fatal(err)
	}
	err := app.Register(b.AsModule())
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("Register() = %v, want duplicate error", err)
	}
}

func TestModuleBuildMountsControllers(t *testing.T) {
	app := New()
	m := &ModuleDef{
		Name: "hello",
		Build: func(m *ModuleContext) error {
			m.Controller(ControllerDef{
				Prefix: "/hello",
				Endpoints: []Endpoint{
					GET("", func(router.Context) error { return nil }, WithSummary("Greet")),
				},
			})
			return nil
		},
	}
	if err := app.Register(m.AsModule()); err != nil {
		t.Fatal(err)
	}
	graph := app.ModuleGraph()
	if len(graph) != 1 || graph[0].Name != "hello" {
		t.Fatalf("graph = %+v", graph)
	}
	if len(graph[0].Controllers) != 1 || len(graph[0].Controllers[0].Routes) != 1 {
		t.Fatalf("controllers = %+v", graph[0].Controllers)
	}
	if graph[0].Controllers[0].Routes[0] != "GET /hello" {
		t.Fatalf("routes = %v", graph[0].Controllers[0].Routes)
	}
}

func TestModuleContextProvide(t *testing.T) {
	app := New()
	app.EnableModuleIsolation()

	m := &ModuleDef{
		Name: "dynamic",
		Build: func(m *ModuleContext) error {
			return m.Provide(isoProvider("dynamic"))
		},
	}
	if err := app.Register(m.AsModule()); err != nil {
		t.Fatal(err)
	}
	v, err := app.Container.Resolve(di.KeyFor[isoService]())
	if err != nil {
		t.Fatal(err)
	}
	if v.(isoService).name != "dynamic" {
		t.Fatalf("resolved %+v, want dynamic", v)
	}
}

func TestModuleReExportsImportedProvider(t *testing.T) {
	app := New()
	app.EnableModuleIsolation()

	db := &ModuleDef{
		Name:      "db",
		Providers: []Provider{isoProvider("db")},
		Exports:   []string{di.KeyFor[isoService]()},
	}
	shared := &ModuleDef{
		Name:          "shared",
		Imports:       []*ModuleDef{db},
		ExportModules: []*ModuleDef{db},
	}
	var got string
	users := &ModuleDef{
		Name:    "users",
		Imports: []*ModuleDef{shared},
		Build: func(m *ModuleContext) error {
			svc, err := Inject[isoService](m)
			if err != nil {
				return err
			}
			got = svc.name
			return nil
		},
	}
	if err := app.Register(users.AsModule()); err != nil {
		t.Fatal(err)
	}
	if got != "db" {
		t.Fatalf("resolved %q, want db", got)
	}
	graph := app.ModuleGraph()
	for _, info := range graph {
		if info.Name == "shared" {
			if len(info.ExportModules) != 1 || info.ExportModules[0] != "db" {
				t.Fatalf("shared ExportModules = %v, want [db]", info.ExportModules)
			}
			return
		}
	}
	t.Fatal("shared module missing from graph")
}

func TestModuleWithoutReExportKeepsProviderHidden(t *testing.T) {
	app := New()
	app.EnableModuleIsolation()

	db := &ModuleDef{
		Name:      "db",
		Providers: []Provider{isoProvider("db")},
		Exports:   []string{di.KeyFor[isoService]()},
	}
	shared := &ModuleDef{
		Name:    "shared",
		Imports: []*ModuleDef{db},
	}
	users := &ModuleDef{
		Name:    "users",
		Imports: []*ModuleDef{shared},
		Build: func(m *ModuleContext) error {
			_, err := Inject[isoService](m)
			return err
		},
	}
	err := app.Register(users.AsModule())
	if err == nil || !strings.Contains(err.Error(), "cannot resolve provider") {
		t.Fatalf("Register() = %v, want visibility error", err)
	}
}

func TestModuleReExportRequiresImport(t *testing.T) {
	app := New()
	db := &ModuleDef{Name: "db"}
	barrel := &ModuleDef{Name: "barrel", ExportModules: []*ModuleDef{db}}

	if err := app.Register(db.AsModule()); err != nil {
		t.Fatal(err)
	}
	err := app.Register(barrel.AsModule())
	if err == nil || !strings.Contains(err.Error(), "does not import it") {
		t.Fatalf("Register() = %v, want missing import error", err)
	}
}

func TestModuleCannotReExportItself(t *testing.T) {
	app := New()
	m := &ModuleDef{Name: "self"}
	m.ExportModules = []*ModuleDef{m}

	err := app.Register(m.AsModule())
	if err == nil || !strings.Contains(err.Error(), "cannot re-export itself") {
		t.Fatalf("Register() = %v, want self re-export error", err)
	}
}

func TestValidateRejectsNilReExport(t *testing.T) {
	app := New()
	app.MustRegister((&ModuleDef{Name: "barrel", ExportModules: []*ModuleDef{nil}}).AsModule())

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "re-exports a nil module") {
		t.Fatalf("Validate() = %v, want nil re-export error", err)
	}
}
