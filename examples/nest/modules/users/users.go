// Package users implements the example users module: an in-memory service
// plus a JWT-guarded controller with validated DTOs. It imports the auth
// module instead of depending on JWT details directly.
package users

import (
	"strconv"
	"strings"
	"sync"

	fauth "github.com/brandonbert8/grove/packages/auth"
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/openapi"
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

// listQuery is the GET /users query DTO (declarative @Query()).
type listQuery struct {
	Search string `query:"q"`
	Limit  int    `query:"limit" validate:"gte=0,lte=100"`
}

// userParams is the GET /users/{id} path DTO (declarative @Param()).
type userParams struct {
	ID string `path:"id" validate:"required"`
}

// Controller serves the users routes (the @Controller() equivalent).
type Controller struct{ svc *Service }

// NewController builds the controller with constructor injection.
func NewController(svc *Service) *Controller { return &Controller{svc: svc} }

// List answers GET /users. It greets the caller from the JWT claims and
// optionally filters by ?q= with a ?limit= cap.
func (c *Controller) List(ctx router.Context, q listQuery) (any, error) {
	who := "anonymous"
	if claims, ok := fauth.CurrentUser(ctx); ok {
		who = claims.Subject
	}
	users := c.svc.List()
	if q.Search != "" {
		kept := users[:0]
		for _, u := range users {
			if strings.Contains(strings.ToLower(u.Name), strings.ToLower(q.Search)) {
				kept = append(kept, u)
			}
		}
		users = kept
	}
	if q.Limit > 0 && len(users) > q.Limit {
		users = users[:q.Limit]
	}
	return map[string]any{"viewer": who, "users": users}, nil
}

// Get answers GET /users/{id}.
func (c *Controller) Get(ctx router.Context, p userParams) (any, error) {
	u, ok := c.svc.Get(p.ID)
	if !ok {
		return nil, grove.NotFound("user not found")
	}
	return u, nil
}

// Create answers POST /users (admin role required).
func (c *Controller) Create(ctx router.Context, in createInput) (any, error) {
	return c.svc.Add(in.Name, in.Email), nil
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
		ctrl, err := grove.Wire(app, NewController)
		if err != nil {
			return nil, err
		}
		guard, err := authmod.Guard(app.Container)
		if err != nil {
			return nil, err
		}
		return []grove.ControllerDef{{
			Prefix:     "/users",
			Tags:       []string{"users"},
			Security:   []string{"bearerAuth"},
			Middleware: []router.Middleware{guard},
			Endpoints: []grove.Endpoint{
				grove.GET("", pipes.HandleQuery(ctrl.List, 200),
					grove.WithSummary("List users"),
					grove.WithQuery(
						grove.QueryParam("q", "Filter by name substring", false),
						grove.QueryParamTyped("limit", "Max users (0 = all)", false, "integer"),
					),
					grove.WithResponses(map[int]string{200: "users + viewer"})),
				grove.GET("/{id}", pipes.HandlePath(ctrl.Get, 200),
					grove.WithSummary("Get user"),
					grove.WithResponses(map[int]string{200: "user", 404: "unknown id"}),
					openapi.WithResponse[User](200, "user")),
				grove.POST("", grove.HandleBodyCreate(ctrl.Create),
					grove.Use(fauth.RequireRole("admin")),
					grove.WithSummary("Create user (admin)"),
					openapi.WithBody[createInput](),
					openapi.WithResponse[User](201, "created user"),
					grove.WithResponses(map[int]string{401: "missing token", 403: "non-admin", 422: "invalid DTO"})),
			},
		}}, nil
	},
}
