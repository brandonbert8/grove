package openapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

func specApp() *grove.App {
	app := grove.New()
	mod := &grove.ModuleDef{
		Name: "users",
		Controllers: []grove.ControllerDef{{
			Prefix: "/users",
			Tags:   []string{"users"},
			Endpoints: []grove.Endpoint{
				{Method: "GET", Path: "", Handler: ok, Summary: "List users", Tags: []string{"users"}},
				{Method: "GET", Path: "/{id}", Handler: ok, Summary: "Get user", Deprecated: true},
				{Method: "POST", Path: "", Handler: ok, Description: "Create a user"},
			},
		}},
	}
	app.MustRegister(mod.AsModule(), grove.HealthModule("").AsModule())
	return app
}

func ok(c router.Context) error { return c.NoContent(204) }

func TestBuildPaths(t *testing.T) {
	s := Build(specApp(), Info{Title: "Demo", Version: "1.0.0"})
	raw, err := s.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	for _, want := range []string{"/users", "/users/{id}", "/healthz"} {
		if _, ok := paths[want]; !ok {
			t.Fatalf("missing path %q in %v", want, paths)
		}
	}
	get, _ := paths["/users/{id}"].(map[string]any)["get"].(map[string]any)
	if get["operationId"] != "get_users_id" {
		t.Fatalf("operationId = %v", get["operationId"])
	}
	if get["deprecated"] != true {
		t.Fatalf("deprecated not propagated: %v", get)
	}
	params, _ := get["parameters"].([]any)
	if len(params) != 1 {
		t.Fatalf("parameters = %v, want one path param", params)
	}
	list, _ := paths["/users"].(map[string]any)["get"].(map[string]any)
	if list["summary"] != "List users" {
		t.Fatalf("summary = %v", list["summary"])
	}
}

func TestMountServesSpec(t *testing.T) {
	app := specApp()
	Mount(app, "/openapi.json", Info{Title: "Demo", Version: "1.0.0"})
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("spec is not JSON: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v", doc["openapi"])
	}
}

func TestRoutePathsSorted(t *testing.T) {
	got := RoutePaths(specApp())
	if len(got) != 4 {
		t.Fatalf("routes = %v, want 4", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("not sorted: %v", got)
		}
	}
}

func TestExplicitQueryResponsesSecurity(t *testing.T) {
	app := grove.New()
	mod := &grove.ModuleDef{
		Name: "search",
		Controllers: []grove.ControllerDef{{
			Prefix: "/search",
			Endpoints: []grove.Endpoint{{
				Method:  "GET",
				Path:    "",
				Handler: ok,
				Summary: "Search",
				Query: []grove.QueryDef{
					{Name: "q", Description: "terms", Required: true},
					{Name: "limit", Description: "page size"},
				},
				Responses: map[int]string{200: "matches", 400: "bad query"},
				Security:  []string{"bearerAuth"},
			}},
		}},
	}
	app.MustRegister(mod.AsModule())
	s := Build(app, Info{Title: "Demo", Version: "1.0.0"})
	raw, err := s.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	get, _ := doc["paths"].(map[string]any)["/search"].(map[string]any)["get"].(map[string]any)
	params, _ := get["parameters"].([]any)
	if len(params) != 2 {
		t.Fatalf("parameters = %v, want 2 query params", params)
	}
	first, _ := params[0].(map[string]any)
	if first["name"] != "q" || first["in"] != "query" || first["required"] != true {
		t.Fatalf("query param = %v", first)
	}
	responses, _ := get["responses"].(map[string]any)
	if responses["200"] == nil || responses["400"] == nil || responses["default"] == nil {
		t.Fatalf("responses = %v, want 200+400+default", responses)
	}
	sec, _ := get["security"].([]any)
	if len(sec) != 1 {
		t.Fatalf("security = %v, want bearerAuth", sec)
	}
	schemes, _ := doc["components"].(map[string]any)["securitySchemes"].(map[string]any)
	bearer, _ := schemes["bearerAuth"].(map[string]any)
	if bearer["scheme"] != "bearer" {
		t.Fatalf("bearerAuth scheme = %v", bearer)
	}
}

func TestMountSeesLaterRoutesAndItself(t *testing.T) {
	app := specApp()
	Mount(app, "/openapi.json", Info{Title: "Demo", Version: "1.0.0"})
	// Routes registered AFTER Mount still appear (lazy build). App.Route
	// records docs; raw Router use stays invisible by design.
	app.Route("GET", "/late", ok)
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	for _, want := range []string{"/late", "/openapi.json"} {
		if _, ok := paths[want]; !ok {
			t.Fatalf("lazy spec missing %q: %v", want, paths)
		}
	}
}

func TestWildcardAndRegexParams(t *testing.T) {
	if got := operationID("GET", "/files/*"); got != "get_files_wildcard" {
		t.Fatalf("wildcard opID = %q", got)
	}
	if got := operationID("GET", "/files"); got != "get_files" {
		t.Fatalf("plain opID = %q", got)
	}
	params := pathParams("/orgs/{id:[0-9]+}/repos/{repo}")
	if len(params) != 2 || params[0].Name != "id" || params[1].Name != "repo" {
		t.Fatalf("regex params = %+v", params)
	}
}
