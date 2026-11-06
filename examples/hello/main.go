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

// HelloModule wires the service and route into the app using the
// idiomatic Grove pattern: Providers for DI and Build for the
// constructor-injected route table. No inline RegisterSingletonAs and no
// ad-hoc app.Route — everything goes through ModuleDef.
var HelloModule = &grove.ModuleDef{
	Name: "hello",
	Providers: []grove.Provider{
		grove.Provide0(di.Singleton, NewGreetingService),
	},
	Build: grove.Build(NewGreetingController, func(ctrl *GreetingController) grove.ControllerDef {
		return grove.ControllerDef{
			Prefix: "/hello",
			Endpoints: []grove.Endpoint{
				grove.GET("", ctrl.Greet),
			},
		}
	}),
}

func main() {
	app := grove.New()
	app.Router.Use(middleware.DefaultChain(app.Logger)...)
	app.Router.Use(grove.RequestScope(app))
	app.EnableStrictProviders()
	app.MustRegister(
		HelloModule.AsModule(),
		grove.HealthModule("").AsModule(),
	)
	if err := app.VerifyExports(); err != nil {
		panic(err)
	}
	if err := app.Listen(); err != nil {
		panic(err)
	}
}
