package grove

import (
	"strings"

	"github.com/brandonbert8/grove/packages/router"
)

// EndpointDoc describes one mounted route for documentation and
// tooling (the @ApiOperation view of an Endpoint). It carries no
// handler or middleware — only the metadata the openapi package needs.
type EndpointDoc struct {
	// Method is the upper-cased HTTP verb.
	Method string
	// Path is the full mounted path (prefix + endpoint path).
	Path string
	// Summary is a short operation title.
	Summary string
	// Description is long-form operation docs.
	Description string
	// Tags merges controller and endpoint tags, deduplicated.
	Tags []string
	// Deprecated marks the operation deprecated.
	Deprecated bool
	// Query declares query parameters.
	Query []QueryDef
	// Responses maps status codes to descriptions.
	Responses map[int]string
	// Security lists required security schemes.
	Security []string
}

// Docs returns the endpoint metadata recorded during Register, in
// registration order. It powers the openapi package and any custom
// docs; routing itself never reads it.
func (a *App) Docs() []EndpointDoc {
	return append([]EndpointDoc(nil), a.docs...)
}

// RecordDocs captures one controller's metadata into Docs. Modules
// record through Register automatically; call it directly to document
// routes mounted below the App API (raw Router use), e.g.:
//
//	app.Router.GET("/legacy", h)
//	app.RecordDocs(grove.ControllerDef{Prefix: "", Endpoints: []grove.Endpoint{
//	    {Method: "GET", Path: "/legacy", Summary: "Legacy endpoint"},
//	}})
func (a *App) RecordDocs(c ControllerDef) {
	for _, e := range c.Endpoints {
		p := router.JoinPath(c.Prefix, e.Path)
		if p == "" {
			p = "/"
		} else if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		doc := EndpointDoc{
			Method:      strings.ToUpper(strings.TrimSpace(e.Method)),
			Path:        p,
			Summary:     e.Summary,
			Description: e.Description,
			Tags:        mergeTags(c.Tags, e.Tags),
			Deprecated:  e.Deprecated,
			Query:       append([]QueryDef(nil), e.Query...),
			Responses:   copyResponses(e.Responses),
			Security:    mergeTags(c.Security, e.Security),
		}
		// Last wins, like the router: re-recording a route replaces it
		// instead of shadowing the spec with phantoms.
		replaced := false
		for i, prev := range a.docs {
			if prev.Method == doc.Method && prev.Path == doc.Path {
				a.docs[i] = doc
				replaced = true
				break
			}
		}
		if !replaced {
			a.docs = append(a.docs, doc)
		}
	}
}

// recordDocs is the internal alias kept for a uniform Register path.
func (a *App) recordDocs(c ControllerDef) { a.RecordDocs(c) }

// copyResponses duplicates a responses map (nil stays nil).
func copyResponses(m map[int]string) map[int]string {
	if m == nil {
		return nil
	}
	out := make(map[int]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// mergeTags concatenates tag sets preserving order, deduplicated.
func mergeTags(sets ...[]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, set := range sets {
		for _, t := range set {
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
