// Package users implements the example users module: an in-memory service
// plus a JWT-guarded controller with validated DTOs. It imports the auth
// module instead of depending on JWT details directly.
package users

import (
	"strconv"
	"sync"

	fauth "github.com/brandonbert8/grove/packages/auth"
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/pipes"
	"github.com/brandonbert8/grove/packages/router"

	authmod "github.com/brandonbert8/grove/examples/nest/modules/auth"
)

// User is the example resource.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Service holds users in memory.
type Service struct {
	mu    sync.RWMutex
	items map[string]User
	seq   int
}

// NewService seeds two users.
func NewService() *Service {
	s := &Service{items: map[string]User{}}
	s.Add("Ada", "ada@example.com")
	s.Add("Grace", "grace@example.com")
	return s
}

// ServiceName identifies the service.
func (s *Service) ServiceName() string { return "users" }

// List returns all users.
func (s *Service) List() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]User, 0, len(s.items))
	for _, u := range s.items {
		out = append(out, u)
	}
	return out
}

// Get returns one user or false.
func (s *Service) Get(id string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.items[id]
	return u, ok
}

// Add creates a user with a generated id.
func (s *Service) Add(name, email string) User {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	u := User{ID: strconv.Itoa(s.seq), Name: name, Email: email}
	s.items[u.ID] = u
	return u
}

// createInput is the POST /users DTO.
type createInput struct {
	Name  string `json:"name" validate:"required,min=2,max=80"`
	Email string `json:"email" validate:"required,email"`
}

// Handler serves the users routes.
type Handler struct{ svc *Service }

// list answers GET /users. It greets the caller from the JWT claims.
func (h *Handler) list(c router.Context) error {
	who := "anonymous"
	if claims, ok := fauth.CurrentUser(c); ok {
		who = claims.Subject
	}
	return c.JSON(200, map[string]any{"viewer": who, "users": h.svc.List()})
}

// get answers GET /users/{id}.
func (h *Handler) get(c router.Context) error {
	u, ok := h.svc.Get(c.Param("id"))
	if !ok {
		return grove.NotFound("user not found")
	}
	return c.JSON(200, u)
}

// create answers POST /users (admin role required).
func (h *Handler) create(c router.Context) error {
	var in createInput
	if err := pipes.ValidateBody(c, &in); err != nil {
		return err
	}
	return c.JSON(201, h.svc.Add(in.Name, in.Email))
}

// UsersModule imports auth for ordering and guards every route with JWT;
// creation additionally requires the admin role.
var UsersModule = &grove.ModuleDef{
	Name:    "users",
	Imports: []*grove.ModuleDef{authmod.AuthModule},
	Providers: []grove.Provider{
		grove.Provide(di.Singleton, func(*di.Container) (*Service, error) {
			return NewService(), nil
		}),
	},
	BuildControllers: func(app *grove.App) ([]grove.ControllerDef, error) {
		svc, err := di.ResolveAs[*Service](app.Container)
		if err != nil {
			return nil, err
		}
		guard, err := authmod.Guard(app.Container)
		if err != nil {
			return nil, err
		}
		h := &Handler{svc: svc}
		return []grove.ControllerDef{{
			Prefix:     "/users",
			Tags:       []string{"users"},
			Security:   []string{"bearerAuth"},
			Middleware: []router.Middleware{guard},
			Endpoints: []grove.Endpoint{
				{Method: "GET", Path: "", Handler: h.list, Summary: "List users",
					Responses: map[int]string{200: "users + viewer"}},
				{Method: "GET", Path: "/{id}", Handler: h.get, Summary: "Get user",
					Responses: map[int]string{200: "user", 404: "unknown id"}},
				{Method: "POST", Path: "", Handler: h.create, Middleware: []router.Middleware{fauth.RequireRole("admin")},
					Summary:   "Create user (admin)",
					Responses: map[int]string{201: "created user", 401: "missing token", 403: "non-admin", 422: "invalid DTO"}},
			},
		}}, nil
	},
}
