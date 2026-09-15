package cli

import "fmt"

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
// endpoints, JSON binding, and validation through pipes. The struct is
// named XxxController (the @Controller() equivalent); its methods are
// the route handlers wired into a ControllerDef in module.go.
func controllerFile(pkg string) string {
	t := title(pkg)
	// Struct tags need backticks, which cannot appear inside a raw string
	// literal, so the DTO block is concatenated as an interpreted string.
	dto := "type CreateInput struct {\n\tName string `json:\"name\" validate:\"required,min=2,max=80\"`\n}\n"
	return fmt.Sprintf(`package %[1]s

import (
	"github.com/brandonbert8/grove/packages/pipes"
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

// Create answers POST /%[1]s.
func (c *%[2]sController) Create(ctx router.Context) error {
	in, err := pipes.Body[CreateInput](ctx)
	if err != nil {
		return err
	}
	c.svc.Add(in.Name)
	return ctx.JSON(201, map[string]any{"created": in.Name})
}
`, pkg, t)
}
