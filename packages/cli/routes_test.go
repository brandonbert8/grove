package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRoutesSourceControllers(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/users/controller.go": `package users

import grove "github.com/brandonbert8/grove/packages/core"

var Controller = &grove.ControllerDef{
	Prefix: "/users",
	Endpoints: []grove.Endpoint{
		grove.GET("", ctrl.List, grove.WithSummary("List users")),
		grove.GET("/{id}", ctrl.Get, grove.WithSummary("Get user"), grove.WithVersion("2")),
		grove.POST("", grove.HandleBody(ctrl.Create, 201), grove.WithSummary("Create user")),
		grove.Endpoint{Method: "DELETE", Path: "/{id}", Summary: "Delete user"},
	},
}
`,
		"modules/health/controller.go": `package health

import grove "github.com/brandonbert8/grove/packages/core"

func controller() grove.ControllerDef {
	return grove.ControllerDef{
		Prefix: "/health",
		Endpoints: []grove.Endpoint{
			grove.GET("", health),
		},
	}
}
`,
	})

	res, err := Routes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %v", res.Notices)
	}
	want := []RouteEntry{
		{Method: "GET", Path: "/health", Handler: "health", File: "modules/health/controller.go"},
		{Method: "GET", Path: "/users", Handler: "ctrl.List", Summary: "List users", File: "modules/users/controller.go"},
		{Method: "POST", Path: "/users", Handler: "ctrl.Create", Summary: "Create user", File: "modules/users/controller.go"},
		{Method: "DELETE", Path: "/users/{id}", Summary: "Delete user", File: "modules/users/controller.go"},
		{Method: "GET", Path: "/users/{id}", Handler: "ctrl.Get", Summary: "Get user", Version: "2", File: "modules/users/controller.go"},
	}
	if len(res.Routes) != len(want) {
		t.Fatalf("routes = %+v, want %d", res.Routes, len(want))
	}
	for i, w := range want {
		got := res.Routes[i]
		if got.Method != w.Method || got.Path != w.Path || got.Handler != w.Handler ||
			got.Summary != w.Summary || got.Version != w.Version || got.File != w.File {
			t.Fatalf("route[%d] = %+v, want %+v", i, got, w)
		}
		if got.Line <= 0 {
			t.Fatalf("route[%d] missing source line: %+v", i, got)
		}
	}
}

func TestRoutesControllerVersionAppliesToEndpoints(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/users/controller.go": `package users

import grove "github.com/brandonbert8/grove/packages/core"

var Controller = &grove.ControllerDef{
	Prefix:  "/users",
	Version: "2",
	Endpoints: []grove.Endpoint{
		grove.GET("", ctrl.List),
		grove.GET("/legacy", ctrl.Legacy, grove.WithVersion("1")),
	},
}
`,
	})

	res, err := Routes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Routes) != 2 {
		t.Fatalf("routes = %+v, want 2", res.Routes)
	}
	for _, rt := range res.Routes {
		switch rt.Path {
		case "/users":
			if rt.Version != "2" {
				t.Fatalf("controller version not inherited: %+v", rt)
			}
		case "/users/legacy":
			if rt.Version != "1" {
				t.Fatalf("endpoint version not honored: %+v", rt)
			}
		default:
			t.Fatalf("unexpected route: %+v", rt)
		}
	}
}

func TestRoutesCommandJSONFilter(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/users/controller.go": `package users

import grove "github.com/brandonbert8/grove/packages/core"

var Controller = &grove.ControllerDef{
	Prefix: "/users",
	Endpoints: []grove.Endpoint{
		grove.GET("", ctrl.List, grove.WithSummary("List users")),
		grove.POST("", ctrl.Create, grove.WithSummary("Create user")),
	},
}
`,
	})

	stdout, code := run(t, dir, "routes", "--format", "json", "--method", "get", "--prefix", "/users")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	var res RoutesResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	if len(res.Routes) != 1 || res.Routes[0].Method != "GET" || res.Routes[0].Path != "/users" {
		t.Fatalf("routes = %+v", res.Routes)
	}
}

func TestRoutesCommandNoControllers(t *testing.T) {
	dir := setupProject(t)
	stdout, code := run(t, dir, "routes")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	for _, want := range []string{"no *grove.ControllerDef", "No routes found."} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output missing %q:\n%s", want, stdout)
		}
	}
}

func TestRoutesCommandUnknownFormat(t *testing.T) {
	dir := setupProject(t)
	_, code := run(t, dir, "routes", "--format", "yaml")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRoutesCompositeEndpointEmptyPath(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/health/controller.go": `package health

import grove "github.com/brandonbert8/grove/packages/core"

var Controller = &grove.ControllerDef{
	Prefix: "/healthz",
	Endpoints: []grove.Endpoint{{
		Method:  "GET",
		Path:    "",
		Handler: health,
		Summary: "Health check",
	}},
}
`,
	})

	res, err := Routes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %v", res.Notices)
	}
	if len(res.Routes) != 1 {
		t.Fatalf("routes = %+v, want 1", res.Routes)
	}
	got := res.Routes[0]
	if got.Method != "GET" || got.Path != "/healthz" || got.Summary != "Health check" {
		t.Fatalf("route = %+v", got)
	}
}

func TestRoutesSkipsNonLiteralPrefixes(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/health/controller.go": `package health

import grove "github.com/brandonbert8/grove/packages/core"

func controller(prefix string) grove.ControllerDef {
	return grove.ControllerDef{
		Prefix: prefix,
		Endpoints: []grove.Endpoint{
			grove.GET("", health),
		},
	}
}
`,
	})

	res, err := Routes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Routes) != 0 {
		t.Fatalf("routes = %+v, want none", res.Routes)
	}
	if len(res.Notices) != 1 || !strings.Contains(res.Notices[0], "non-literal prefixes") {
		t.Fatalf("notices = %v", res.Notices)
	}
}

func TestRoutesSkipsNonLiteralPaths(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/users/controller.go": `package users

import grove "github.com/brandonbert8/grove/packages/core"

var Controller = &grove.ControllerDef{
	Prefix: "/users",
	Endpoints: []grove.Endpoint{
		grove.GET(path, ctrl.List),
	},
}
`,
	})

	res, err := Routes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Routes) != 0 {
		t.Fatalf("routes = %+v, want none", res.Routes)
	}
	if len(res.Notices) != 1 || !strings.Contains(res.Notices[0], "non-literal paths") {
		t.Fatalf("notices = %v", res.Notices)
	}

	stdout, code := run(t, dir, "routes", "--format", "json")
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	var out RoutesResult
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	if len(out.Notices) != 1 || !strings.Contains(out.Notices[0], "non-literal paths") {
		t.Fatalf("json notices = %v", out.Notices)
	}
}

func TestJoinRoutePath(t *testing.T) {
	for _, tc := range []struct{ prefix, path, want string }{
		{"", "", "/"},
		{"/users", "", "/users"},
		{"/users", "/{id}", "/users/{id}"},
		{"users/", "id", "/users/id"},
		{"/", "/health", "/health"},
	} {
		if got := joinRoutePath(tc.prefix, tc.path); got != tc.want {
			t.Fatalf("joinRoutePath(%q,%q) = %q, want %q", tc.prefix, tc.path, got, tc.want)
		}
	}
}
