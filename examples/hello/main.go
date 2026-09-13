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

// GreetingHandler serves the hello route using the injected service.
type GreetingHandler struct{ svc *GreetingService }

// NewGreetingHandler builds the handler with constructor injection.
func NewGreetingHandler(svc *GreetingService) *GreetingHandler {
	return &GreetingHandler{svc: svc}
}

// Greet answers GET /hello.
func (h *GreetingHandler) Greet(c router.Context) error {
	return c.JSON(200, map[string]string{"message": h.svc.Message()})
}

// HelloModule wires the service and route into the app.
var HelloModule = grove.NewModule("hello", func(app *grove.App) error {
	if err := di.RegisterSingletonAs(app.Container, NewGreetingService()); err != nil {
		return err
	}
	svc, err := di.ResolveAs[*GreetingService](app.Container)
	if err != nil {
		return err
	}
	h := NewGreetingHandler(svc)
	// App.Route (not raw Router) keeps the route visible to App.Docs.
	app.Route("GET", "/hello", h.Greet)
	return nil
})

func main() {
	app := grove.New()
	app.Router.Use(
		middleware.Recovery(app.Logger),
		middleware.RequestID(),
		middleware.Logging(app.Logger),
	)
	app.MustRegister(
		HelloModule,
		grove.HealthModule("").AsModule(),
	)
	if err := app.Run(""); err != nil {
		panic(err)
	}
}
