// Command nest runs the NestJS-style Grove example: modules with imports,
// providers with constructor injection, a validated DTO pipeline, and
// JWT guards with roles.
//
//	$ go run ./examples/nest
//	$ curl -X POST localhost:3000/auth/login -d '{"username":"admin","password":"secret"}'
//	$ curl localhost:3000/users -H "Authorization: Bearer <token>"
package main

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/middleware"

	"github.com/brandonbert8/grove/examples/nest/modules/users"
)

func main() {
	app := grove.New()
	app.Router.Use(
		middleware.Recovery(app.Logger),
		middleware.Logging(app.Logger),
		middleware.CORS(middleware.DefaultCORSConfig()),
	)
	// UsersModule imports AuthModule, which builds first exactly once.
	app.MustRegister(users.UsersModule.AsModule())
	if err := app.Run(""); err != nil {
		panic(err)
	}
}
