// Package main runs the Grove hello example: app creation, module
// registration, one GET endpoint, and dependency injection.
//
//	$ go run ./examples/hello
//	$ curl localhost:3000/hello
//	{"message":"Hello Grove"}
package main

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/middleware"
	"github.com/brandonbert8/grove/packages/router"
)

// GreetingService is the example injectable dependency.
type GreetingService struct{ message string }

// NewGreetingService builds the service.
func NewGreetingService() *GreetingService {
	return &GreetingService{message: "Hello Grove"}
}

// Message returns the greeting text.
func (s *GreetingService) Message() string { return s.message }

// GreetingController serves the hello route using the injected service
// (the @Controller() equivalent).
type GreetingController struct{ svc *GreetingService }

// NewGreetingController builds the controller with constructor injection.
func NewGreetingController(svc *GreetingService) *GreetingController {
	return &GreetingController{svc: svc}
}

// Greet answers GET /hello.
func (c *GreetingController) Greet(ctx router.Context) error {
	return ctx.JSON(200, map[string]string{"message": c.svc.Message()})
}

// HelloModule wires the service and route into the app.
var HelloModule = grove.NewModule("hello", func(app *grove.App) error {
	if err := di.RegisterSingletonAs(app.Container, NewGreetingService()); err != nil {
		return err
	}
	ctrl, err := grove.Wire(app, NewGreetingController)
	if err != nil {
		return err
	}
	// App.Route (not raw Router) keeps the route visible to App.Docs.
	app.Route("GET", "/hello", ctrl.Greet)
	return nil
})

func main() {
	app := grove.New()
	app.Router.Use(middleware.DefaultChain(app.Logger)...)
	app.MustRegister(
		HelloModule,
		grove.HealthModule("").AsModule(),
	)
	if err := app.Listen(); err != nil {
		panic(err)
	}
}
