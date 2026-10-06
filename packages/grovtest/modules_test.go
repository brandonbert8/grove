package grovtest

import (
	"testing"

	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

func TestTestingModulesWithFuncModule(t *testing.T) {
	app := TestingModules(t, []grove.Module{
		grove.NewModule("hello", func(app *grove.App) error {
			app.Route("GET", "/hello", func(c router.Context) error {
				return c.JSON(200, map[string]string{"message": "hi"})
			})
			return nil
		}),
	})
	cli := New(app)
	RequireStatus(t, cli.Get(t, "/hello"), 200)
}
