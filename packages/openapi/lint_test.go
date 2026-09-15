package openapi

import (
	"strings"
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
)

func TestLintFlagsUndocumented(t *testing.T) {
	app := grove.New()
	app.MustRegister((&grove.ModuleDef{
		Name: "mixed",
		Controllers: []grove.ControllerDef{{
			Prefix: "/mixed",
			Endpoints: []grove.Endpoint{
				grove.GET("/clean", ok,
					grove.WithSummary("Clean"),
					grove.WithResponses(map[int]string{200: "ok"})),
				grove.GET("/bare", ok),
			},
		}},
	}).AsModule())
	warnings := Lint(app)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "GET /mixed/bare: missing summary") {
		t.Fatalf("must flag missing summary, got %v", warnings)
	}
	if !strings.Contains(joined, "GET /mixed/bare: missing responses") {
		t.Fatalf("must flag missing responses, got %v", warnings)
	}
	for _, w := range warnings {
		if strings.Contains(w, "/mixed/clean") {
			t.Fatalf("documented route must be clean, got %v", warnings)
		}
	}
}

func TestLintEmptyWhenDocumented(t *testing.T) {
	if warnings := Lint(specApp()); len(warnings) == 0 {
		t.Fatal("fixture has undocumented routes, want warnings")
	}
}
