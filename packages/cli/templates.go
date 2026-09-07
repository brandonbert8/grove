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

// controllerFile renders a handler with list/create endpoints, JSON
// binding, and validation through pipes.
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
// %[2]sHandler serves the %[1]s routes.
type %[2]sHandler struct{ svc *%[2]sService }

// New%[2]sHandler builds the handler with constructor injection.
func New%[2]sHandler(svc *%[2]sService) *%[2]sHandler {
	return &%[2]sHandler{svc: svc}
}

// List answers GET /%[1]s.
func (h *%[2]sHandler) List(c router.Context) error {
	return c.JSON(200, map[string]any{"items": h.svc.List()})
}

// Create answers POST /%[1]s.
func (h *%[2]sHandler) Create(c router.Context) error {
	var in CreateInput
	if err := pipes.ValidateBody(c, &in); err != nil {
		return err
	}
	h.svc.Add(in.Name)
	return c.JSON(201, map[string]any{"created": in.Name})
}
`, pkg, t)
}
