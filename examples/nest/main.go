// Command nest runs the NestJS-style Grove example: modules with imports,
// providers with constructor injection, a validated DTO pipeline, and
// JWT guards with roles.
//
//	$ go run ./examples/nest
//	$ curl -X POST localhost:3000/v1/auth/login -d '{"username":"admin","password":"secret"}'
//	$ curl localhost:3000/v1/users -H "Authorization: Bearer <token>"
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
	app.Router.Use(middleware.DefaultChain(app.Logger)...)
	app.Router.Use(middleware.CORS(middleware.DefaultCORSConfig()))
	app.Router.Use(grove.RequestScope(app))
	app.EnableStrictProviders()
	app.SetGlobalPrefix("/v1", "/healthz", "/openapi.json")
	// UsersModule imports AuthModule, which builds first exactly once.
	app.MustRegister(
		users.UsersModule.AsModule(),
		grove.HealthModule("").AsModule(),
	)
	if err := app.VerifyExports(); err != nil {
		panic(err)
	}
	openapi.Mount(app, "/openapi.json", openapi.Info{
		Title:       "Grove Nest Example",
		Version:     "1.0.0",
		Description: "Modules, JWT guards, validation, request scope, and generated OpenAPI.",
	})
	if err := app.Listen(); err != nil {
		panic(err)
	}
}
