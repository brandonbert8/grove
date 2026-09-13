package grove

import (
	"context"
	"time"

	"github.com/brandonbert8/grove/packages/router"
)

// HealthModule returns a module serving GET <path> with
// {"status":"ok"} — the Terminus-style health check, Grove edition.
// An empty path defaults to "/healthz". Register it like any module:
//
//	app.MustRegister(grove.HealthModule(""))
func HealthModule(path string) *ModuleDef {
	if path == "" {
		path = "/healthz"
	}
	return &ModuleDef{
		Name: "health",
		Controllers: []ControllerDef{{
			Prefix: path,
			Tags:   []string{"ops"},
			Endpoints: []Endpoint{{
				Method:  "GET",
				Path:    "",
				Handler: healthHandler,
				Summary: "Health check",
				Tags:    []string{"ops"},
			}},
		}},
	}
}

// healthHandler answers the health probe.
func healthHandler(c router.Context) error {
	return c.JSON(200, map[string]string{"status": "ok"})
}

// Check is one health indicator (db, queue, downstream): nil means ok.
type Check func(ctx context.Context) error

// defaultCheckTimeout bounds every indicator so one hung dependency
// cannot hang the probe (and k8s) forever.
const defaultCheckTimeout = 5 * time.Second

// HealthModuleWithChecks returns a module serving GET <path> with
// per-indicator status — {"status":"ok","checks":{"db":"ok"}} — and
// 503 when any check fails:
//
//	app.MustRegister(grove.HealthModuleWithChecks("", 0, map[string]grove.Check{
//	    "db": pool.PingContext,
//	}))
//
// An empty path defaults to "/healthz"; a non-positive timeout means
// the 5s default. Indicator failures report their messages; the probe
// never leaks internals beyond that.
func HealthModuleWithChecks(path string, timeout time.Duration, checks map[string]Check) *ModuleDef {
	if path == "" {
		path = "/healthz"
	}
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	handler := func(c router.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), timeout)
		defer cancel()
		status := map[string]string{}
		failed := false
		for name, check := range checks {
			if check == nil {
				continue
			}
			if err := check(ctx); err != nil {
				status[name] = err.Error()
				failed = true
			} else {
				status[name] = "ok"
			}
		}
		if failed {
			return c.JSON(503, map[string]any{"status": "degraded", "checks": status})
		}
		return c.JSON(200, map[string]any{"status": "ok", "checks": status})
	}
	return &ModuleDef{
		Name: "health",
		Controllers: []ControllerDef{{
			Prefix: path,
			Tags:   []string{"ops"},
			Endpoints: []Endpoint{{
				Method:  "GET",
				Path:    "",
				Handler: handler,
				Summary: "Health check with indicators",
				Tags:    []string{"ops"},
			}},
		}},
	}
}
