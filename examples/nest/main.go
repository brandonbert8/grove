// Command nest runs the NestJS-style Grove example: modules with imports,
// providers with constructor injection, a validated DTO pipeline, and
// JWT guards with roles.
//
//	$ go run ./examples/nest
//	$ curl -X POST localhost:3000/auth/login -d '{"username":"admin","password":"secret"}'
//	$ curl localhost:3000/users -H "Authorization: Bearer <token>"
//	$ curl localhost:3000/openapi.json
package main

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/middleware"
	"github.com/brandonbert8/grove/packages/openapi"

	"github.com/brandonbert8/grove/examples/nest/modules/users"
)

func main() {
	app := grove.New()
	app.Router.Use(
		middleware.Recovery(app.Logger),
		middleware.RequestID(),
		middleware.Logging(app.Logger),
		middleware.CORS(middleware.DefaultCORSConfig()),
	)
	// UsersModule imports AuthModule, which builds first exactly once.
	app.MustRegister(
		users.UsersModule.AsModule(),
		grove.HealthModule("").AsModule(),
	)
	openapi.Mount(app, "/openapi.json", openapi.Info{
		Title:       "Grove Nest Example",
		Version:     "0.2.0",
		Description: "Modules, JWT guards, validation, and generated OpenAPI.",
	})
	if err := app.Run(""); err != nil {
		panic(err)
	}
}
