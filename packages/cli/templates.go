package cli

import "fmt"

// guardFile renders a CanActivate guard (the NestJS @Injectable guard
// equivalent): return nil to allow, an *HttpError to deny.
func guardFile(pkg, typ string) string {
	return fmt.Sprintf(`package %[1]s

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

// %[2]sGuard decides whether a request may proceed (authN/authZ).
// Attach it per route, per controller, or globally:
//
//	grove.GET("", ctrl.List, grove.WithGuards(%[2]sGuard{}))
//	app.UseGuards(%[2]sGuard{})
type %[2]sGuard struct{}

// CanActivate implements grove.CanActivate.
func (%[2]sGuard) CanActivate(c router.Context) error {
	// TODO: inspect headers/claims via c.Header / auth.CurrentUser.
	_ = c
	return nil
}

// %[2]sMiddleware adapts the guard to route middleware.
func %[2]sMiddleware() router.Middleware { return grove.UseGuard(%[2]sGuard{}) }
`, pkg, typ)
}

// pipeFile renders a custom validation rule (the class-validator custom
// decorator equivalent) registered with pipes.RegisterRule.
func pipeFile(pkg, typ string) string {
	return fmt.Sprintf(`package %[1]s

import (
	"reflect"

	"github.com/brandonbert8/grove/packages/pipes"
)

// Lower is the rule name used in validate tags: validate:"lower".
const Lower = "%[3]s"

// Register%[2]sRule registers the custom validation rule. Call it from
// module wiring or init — never per request.
func Register%[2]sRule() {
	pipes.RegisterRule(Lower, func(field string, value reflect.Value, param string) string {
		if value.Kind() != reflect.String {
			return ""
		}
		if value.String() == "" {
			return "field \"" + field + "\" must not be empty"
		}
		return ""
	})
}
`, pkg, typ, pkg)
}

// filterFile renders an exception filter (the NestJS @Catch() equivalent).
func filterFile(pkg, typ string) string {
	return fmt.Sprintf(`package %[1]s

import (
	"github.com/brandonbert8/grove/packages/router"
)

// %[2]sFilter handles handler errors. Return nil when handled
// (response already written), or a non-nil error to fall through:
//
//	app.UseFilters(%[2]sFilter{})
type %[2]sFilter struct{}

// Catch implements grove.ExceptionFilter.
func (%[2]sFilter) Catch(c router.Context, err error) error {
	// TODO: map domain errors to status codes, then:
	//   return c.JSON(400, map[string]string{"error": err.Error()})
	_ = c
	return err
}
`, pkg, typ)
}

// interceptorFile renders a response-mapping interceptor (the NestJS
// @Injectable interceptor equivalent) built on grove.MapResponse.
func interceptorFile(pkg, typ string) string {
	return fmt.Sprintf(`package %[1]s

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

// %[2]sInterceptor wraps handler execution (tracing, metrics, response
// mapping). Attach globally or per route:
//
//	app.UseInterceptors(%[2]sInterceptor())
func %[2]sInterceptor() router.Middleware {
	return grove.InterceptorFunc(func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			// TODO: observe before/after next(c).
			return next(c)
		}
	})
}
`, pkg, typ)
}


// serviceFile renders an in-memory CRUD service with a ServiceName marker
// so it satisfies grove.Service for future tooling. typ is the capitalized
// base name ("Billing"); the "Service" suffix is added by the template.
func serviceFile(pkg, typ string) string {
	return fmt.Sprintf(`package %[1]s

import "sync"

// %[2]s is the %[1]s business-logic service.
type %[2]sService struct {
	mu    sync.RWMutex
	items []string
}

// New%[2]sService builds the service.
func New%[2]sService() *%[2]sService { return &%[2]sService{} }

// ServiceName identifies the service for logs and diagnostics.
func (s *%[2]sService) ServiceName() string { return "%[1]s" }

// List returns all items.
func (s *%[2]sService) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.items...)
}

// Add appends an item.
func (s *%[2]sService) Add(item string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, item)
}
`, pkg, typ)
}

// controllerFile renders a NestJS-style controller with list/create
// endpoints and a declarative validated DTO handler. The struct is
// named XxxController (the @Controller() equivalent); its methods are
// the route handlers wired into a ControllerDef in module.go.
func controllerFile(pkg string) string {
	t := title(pkg)
	// Struct tags need backticks, which cannot appear inside a raw string
	// literal, so the DTO block is concatenated as an interpreted string.
	dto := "type CreateInput struct {\n\tName string `json:\"name\" validate:\"required,min=2,max=80\"`\n}\n"
	return fmt.Sprintf(`package %[1]s

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/router"
)

// CreateInput is the POST /%[1]s DTO.
`+dto+`
// %[2]sController serves the %[1]s routes (the @Controller() equivalent).
// Its methods are wired into a grove.ControllerDef in module.go.
type %[2]sController struct{ svc *%[2]sService }

// New%[2]sController builds the controller with constructor injection.
func New%[2]sController(svc *%[2]sService) *%[2]sController {
	return &%[2]sController{svc: svc}
}

// List answers GET /%[1]s.
func (c *%[2]sController) List(ctx router.Context) error {
	return ctx.JSON(200, map[string]any{"items": c.svc.List()})
}

// Create answers POST /%[1]s. The DTO arrives decoded + validated via
// grove.HandleBody in module.go (the @Body() + ValidationPipe equivalent).
func (c *%[2]sController) Create(ctx router.Context, in CreateInput) (any, error) {
	c.svc.Add(in.Name)
	return map[string]any{"created": in.Name}, nil
}

// Ensure Create matches grove.BodyHandler[CreateInput].
var _ grove.BodyHandler[CreateInput] = (*%[2]sController)(nil).Create
`, pkg, t)
}
