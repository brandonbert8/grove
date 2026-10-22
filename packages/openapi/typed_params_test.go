package openapi

import (
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
)

func TestTypedQueryParams(t *testing.T) {
	qs := queryParams([]grove.QueryDef{
		{Name: "page", Type: "integer"},
		{Name: "q"},
		{Name: "verbose", Type: "bool"},
	})
	got := map[string]string{}
	for _, q := range qs {
		got[q.Name] = q.Schema["type"]
	}
	if got["page"] != "integer" || got["q"] != "string" || got["verbose"] != "boolean" {
		t.Fatalf("unexpected query types: %v", got)
	}
}

func TestTypedPathParams(t *testing.T) {
	ps := pathParams("/users/{id}/posts/{slug} /orgs/{oid:[0-9]+}")
	_ = ps
	got := map[string]string{}
	for _, p := range pathParams("/users/{id}/orgs/{oid:[0-9]+}/repos/{repo}") {
		got[p.Name] = p.Schema["type"]
	}
	if got["id"] != "string" || got["oid"] != "integer" || got["repo"] != "string" {
		t.Fatalf("unexpected path types: %v", got)
	}
}
