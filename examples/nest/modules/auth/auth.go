// Package auth implements the example auth module: a JWT service provider
// plus a login controller. Other modules import AuthModule to protect
// their routes.
package auth

import (
	fauth "github.com/brandonbert8/grove/packages/auth"
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/pipes"
	"github.com/brandonbert8/grove/packages/router"
)

// users is the demo credential store. Real apps query a database here.
var users = map[string]struct {
	password string
	roles    []string
}{
	"admin":  {password: "secret", roles: []string{"admin", "user"}},
	"gopher": {password: "secret", roles: []string{"user"}},
}

// newJWTService builds the signing service. The secret falls back to a
// dev-only default so the example runs without configuration.
func newJWTService() *fauth.Service {
	return fauth.NewService([]byte("dev-only-secret-change-me"), "grove-nest-example")
}

// loginInput is the POST /auth/login DTO.
type loginInput struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// loginHandler issues tokens.
type loginHandler struct{ jwt *fauth.Service }

// login validates credentials and signs a token.
func (h *loginHandler) login(c router.Context) error {
	var in loginInput
	if err := pipes.ValidateBody(c, &in); err != nil {
		return err
	}
	u, ok := users[in.Username]
	if !ok || u.password != in.Password {
		return grove.Unauthorized("invalid credentials")
	}
	roles := make([]any, 0, len(u.roles))
	for _, r := range u.roles {
		roles = append(roles, r)
	}
	token, err := h.jwt.Sign(in.Username, map[string]any{"roles": roles})
	if err != nil {
		return err
	}
	return c.JSON(200, map[string]any{"token": token})
}

// Guard protects routes with bearer JWTs. It resolves the shared service
// from the container so importing modules share one key.
func Guard(c *di.Container) (router.Middleware, error) {
	svc, err := di.ResolveAs[*fauth.Service](c)
	if err != nil {
		return nil, err
	}
	return fauth.AuthGuard(svc), nil
}

// AuthModule provides the JWT service and the login controller.
var AuthModule = &grove.ModuleDef{
	Name: "auth",
	Providers: []grove.Provider{
		grove.Provide(di.Singleton, func(*di.Container) (*fauth.Service, error) {
			return newJWTService(), nil
		}),
	},
	BuildControllers: func(app *grove.App) ([]grove.ControllerDef, error) {
		svc, err := di.ResolveAs[*fauth.Service](app.Container)
		if err != nil {
			return nil, err
		}
		h := &loginHandler{jwt: svc}
		return []grove.ControllerDef{{
			Prefix: "/auth",
			Endpoints: []grove.Endpoint{
				grove.POST("/login", h.login),
			},
		}}, nil
	},
}
