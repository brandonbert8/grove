package openapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

// Info is the spec's info block.
type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

// Parameter is one operation parameter. Path params are extracted from
// {name} segments automatically; query params come from explicit
// Endpoint.Query metadata (the @ApiQuery equivalent).
type Parameter struct {
	Name        string            `json:"name"`
	In          string            `json:"in"`
	Required    bool              `json:"required"`
	Description string            `json:"description,omitempty"`
	Schema      map[string]string `json:"schema"`
}

// Response is one entry of an operation's responses.
type Response struct {
	Description string `json:"description"`
}

// Operation is one method+path entry.
type Operation struct {
	OperationID string                `json:"operationId,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Deprecated  bool                  `json:"deprecated,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	Responses   map[string]Response   `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
}

// SecurityScheme is one components.securitySchemes entry.
type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
}

// knownSchemes are the security scheme names Grove understands.
// Operations referencing other names still list them; the app owns
// their components definition via custom spec assembly.
var knownSchemes = map[string]SecurityScheme{
	"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
}

// Spec is an OpenAPI 3.1 document.
type Spec struct {
	OpenAPI    string                               `json:"openapi"`
	Info       Info                                 `json:"info"`
	Paths      map[string]map[string]Operation      `json:"paths"`
	Components map[string]map[string]SecurityScheme `json:"components,omitempty"`
}

// Build renders the app's documented routes (App.Docs, recorded at
// Register time) into a Spec. Routes without metadata still appear —
// with an operationId and a default response — so the spec always
// mirrors the real router. "*" (all-method) routes are skipped: OpenAPI
// has no wildcard method.
func Build(app *grove.App, info Info) Spec {
	s := Spec{OpenAPI: "3.1.0", Info: info, Paths: map[string]map[string]Operation{}}
	if s.Info.Title == "" {
		s.Info.Title = "Grove API"
	}
	if s.Info.Version == "" {
		s.Info.Version = "0.0.0"
	}
	for _, d := range app.Docs() {
		method := strings.ToLower(d.Method)
		if method == "" || method == "*" {
			continue
		}
		item, ok := s.Paths[d.Path]
		if !ok {
			item = map[string]Operation{}
			s.Paths[d.Path] = item
		}
		desc := d.Summary
		if desc == "" {
			desc = strings.ToUpper(d.Method) + " " + d.Path
		}
		responses := map[string]Response{"default": {Description: desc}}
		for code, text := range d.Responses {
			key := "default"
			if code != 0 {
				key = strconv.Itoa(code)
			}
			responses[key] = Response{Description: text}
		}
		var security []map[string][]string
		for _, name := range d.Security {
			if name == "" {
				continue
			}
			security = append(security, map[string][]string{name: {}})
			if scheme, ok := knownSchemes[name]; ok {
				if s.Components == nil {
					s.Components = map[string]map[string]SecurityScheme{}
				}
				if s.Components["securitySchemes"] == nil {
					s.Components["securitySchemes"] = map[string]SecurityScheme{}
				}
				s.Components["securitySchemes"][name] = scheme
			}
		}
		item[method] = Operation{
			OperationID: operationID(d.Method, d.Path),
			Summary:     d.Summary,
			Description: d.Description,
			Tags:        d.Tags,
			Deprecated:  d.Deprecated,
			Parameters:  append(pathParams(d.Path), queryParams(d.Query)...),
			Responses:   responses,
			Security:    security,
		}
	}
	return s
}

// operationID renders e.g. GET /users/{id} as get_users_id. A catch-all
// /* suffix becomes _wildcard so /files and /files/* never collide.
func operationID(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	for _, seg := range strings.Split(path, "/") {
		seg = strings.Trim(seg, "{}")
		if seg == "" {
			continue
		}
		if seg == "*" {
			b.WriteString("_wildcard")
			continue
		}
		b.WriteString("_")
		for _, r := range seg {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
				b.WriteRune(r)
			} else {
				b.WriteString("_")
			}
		}
	}
	return b.String()
}

// pathParams extracts {name} segments in order, stripping chi regex
// constraints ({id:[0-9]+} documents as id). The /* catch-all has no
// OpenAPI equivalent and is skipped (documented on Build).
func pathParams(path string) []Parameter {
	var out []Parameter
	for _, seg := range strings.Split(path, "/") {
		if len(seg) > 2 && strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			name := seg[1 : len(seg)-1]
			if i := strings.IndexByte(name, ':'); i >= 0 {
				name = name[:i]
			}
			if name != "" && name != "*" {
				out = append(out, Parameter{
					Name:     name,
					In:       "path",
					Required: true,
					Schema:   map[string]string{"type": "string"},
				})
			}
		}
	}
	return out
}

// queryParams renders explicit Endpoint.Query metadata.
func queryParams(qs []grove.QueryDef) []Parameter {
	var out []Parameter
	for _, q := range qs {
		if q.Name == "" {
			continue
		}
		out = append(out, Parameter{
			Name:        q.Name,
			In:          "query",
			Required:    q.Required,
			Description: q.Description,
			Schema:      map[string]string{"type": "string"},
		})
	}
	return out
}

// JSON renders the spec.
func (s Spec) JSON() ([]byte, error) { return json.MarshalIndent(s, "", "  ") }

// Handler serves the spec as application/json.
func (s Spec) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		raw, err := s.JSON()
		if err != nil {
			http.Error(w, "openapi: encode spec", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	})
}

// Mount registers GET path serving the spec (e.g.
// Mount(app, "/openapi.json", Info{...})), the one-line
// SwaggerModule.setup equivalent. The spec builds per request from
// App.Docs, so routes registered after Mount still appear; Mount
// documents itself under the ops tag.
func Mount(app *grove.App, path string, info Info) {
	app.Router.Handle("GET", path, func(c router.Context) error {
		Build(app, info).Handler().ServeHTTP(c.ResponseWriter(), c.Request())
		return nil
	})
	app.RecordDocs(grove.ControllerDef{
		Prefix: path,
		Tags:   []string{"ops"},
		Endpoints: []grove.Endpoint{{
			Method:  "GET",
			Path:    "",
			Handler: func(c router.Context) error { return c.NoContent(200) },
			Summary: "OpenAPI spec",
			Tags:    []string{"ops"},
		}},
	})
}

// Lint audits the app's endpoint docs (App.Docs) and returns one
// human-readable warning per gap, the `grove vet` equivalent for docs
// coverage: endpoints without a Summary, and endpoints without a
// Responses map. An empty result means every route is documented.
//
//	warnings := openapi.Lint(app) // []string{"GET /users: missing summary"}
func Lint(app *grove.App) []string {
	var out []string
	for _, d := range app.Docs() {
		label := strings.ToUpper(strings.TrimSpace(d.Method)) + " " + d.Path
		if strings.TrimSpace(d.Summary) == "" {
			out = append(out, label+": missing summary (add grove.WithSummary)")
		}
		if len(d.Responses) == 0 {
			out = append(out, label+": missing responses (add grove.WithResponses)")
		}
	}
	sort.Strings(out)
	return out
}

// RoutePaths returns the app's mounted method+path pairs sorted —
// handy for asserting docs coverage in tests.
func RoutePaths(app *grove.App) []string {
	var out []string
	for _, d := range app.Docs() {
		out = append(out, strings.ToUpper(d.Method)+" "+d.Path)
	}
	sort.Strings(out)
	return out
}
