package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGraphFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGraphSourceModules(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"main.go": `package main

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"example.com/demo/modules/users"
)

func main() {
	app := grove.New()
	app.MustRegister(users.UsersModule.AsModule(), grove.HealthModule("").AsModule())
}
`,
		"modules/users/module.go": `package users

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"example.com/demo/modules/auth"
)

var UsersModule = &grove.ModuleDef{
	Name: "users",
	Imports: []*grove.ModuleDef{auth.AuthModule},
}
`,
		"modules/auth/module.go": `package auth

import grove "github.com/brandonbert8/grove/packages/core"

var AuthModule = &grove.ModuleDef{Name: "auth"}
`,
	})

	res, err := Graph(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(res.Modules))
	for _, m := range res.Modules {
		names = append(names, m.Name)
	}
	want := []string{"auth", "health", "users"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("modules = %v, want %v", names, want)
	}
	if len(res.Edges) != 1 || res.Edges[0] != (GraphEdge{From: "users", To: "auth"}) {
		t.Fatalf("edges = %+v, want users -> auth", res.Edges)
	}
}

func TestGraphRenderers(t *testing.T) {
	res := GraphResult{
		Modules: []GraphModule{{Name: "users", Imports: []string{"auth"}}, {Name: "auth"}},
		Edges:   []GraphEdge{{From: "users", To: "auth"}},
	}
	if got := RenderGraphMermaid(res); !strings.Contains(got, `users["users"]`) || !strings.Contains(got, "users --> auth") {
		t.Fatalf("mermaid:\n%s", got)
	}
	if got := RenderGraphDOT(res); !strings.Contains(got, `"users" -> "auth"`) {
		t.Fatalf("dot:\n%s", got)
	}
	got, err := RenderGraphJSON(res)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"from": "users"`) || !strings.Contains(got, `"to": "auth"`) {
		t.Fatalf("json:\n%s", got)
	}
}

func TestGraphCycleProblems(t *testing.T) {
	res := GraphResult{
		Modules: []GraphModule{{Name: "a"}, {Name: "b"}},
		Edges:   []GraphEdge{{From: "a", To: "b"}, {From: "b", To: "a"}},
	}
	problems := graphCycleProblems(res)
	if len(problems) != 1 || !strings.Contains(problems[0], "module import cycle") {
		t.Fatalf("problems = %v, want one cycle", problems)
	}
}

func TestGraphMissingGoMod(t *testing.T) {
	if _, err := Graph(t.TempDir()); err == nil {
		t.Fatal("Graph() = nil, want missing go.mod error")
	}
}

func TestGraphCommandJSON(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/users/module.go": `package users

import grove "github.com/brandonbert8/grove/packages/core"

var UsersModule = &grove.ModuleDef{Name: "users"}
`,
	})
	stdout, code := run(t, dir, "graph", "--format", "json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, `"name": "users"`) {
		t.Fatalf("output:\n%s", stdout)
	}
}

func TestGraphCommandUnknownFormat(t *testing.T) {
	dir := setupProject(t)
	_, code := run(t, dir, "graph", "--format", "yaml")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestDoctorCommandClean(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/users/module.go": `package users

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/di"
)

var UsersModule = &grove.ModuleDef{
	Name: "users",
	Providers: []grove.Provider{grove.Provide0(di.Singleton, NewService)},
}

func NewService() string { return "users" }
`,
	})
	stdout, code := run(t, dir, "doctor")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "Doctor clean") {
		t.Fatalf("output:\n%s", stdout)
	}
}

func TestDoctorCommandFindsDocsProblems(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/users/controller.go": `package users

import grove "github.com/brandonbert8/grove/packages/core"

var routes = grove.GET("/users", nil)
`,
	})
	stdout, code := run(t, dir, "doctor")
	if code != 1 {
		t.Fatalf("exit = %d, want 1\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "controller without grove.WithSummary") {
		t.Fatalf("output:\n%s", stdout)
	}
}
