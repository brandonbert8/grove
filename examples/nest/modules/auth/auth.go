// Package auth implements the example auth module: a JWT service provider
// plus login/refresh controllers with bcrypt passwords. Other modules
// import AuthModule to protect their routes.
package auth

import (
	"os"
	"time"

	fauth "github.com/brandonbert8/grove/packages/auth"
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/pipes"
	"github.com/brandonbert8/grove/packages/router"
)

// users is the demo credential store: bcrypt hashes of "secret" for both
// demo accounts. Real apps query a database here.
var users = map[string]struct {
	passwordHash string
	roles        []string
}{
	"admin":  {passwordHash: "$2a$10$fHUyZGhFcoQvqGy.Ub9/xOd7aYcLPgDf0jgNieDeqr19KxnGqXB16", roles: []string{"admin", "user"}},
	"gopher": {passwordHash: "$2a$10$fHUyZGhFcoQvqGy.Ub9/xOd7aYcLPgDf0jgNieDeqr19KxnGqXB16", roles: []string{"user"}},
}

// newJWTService builds the signing service from JWT_SECRET, falling back
// to a dev-only default so the example runs without configuration.
func newJWTService() *fauth.Service {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-only-secret-change-me"
	}
	return fauth.NewService([]byte(secret), "grove-nest-example")
}

// loginInput is the POST /auth/login DTO.
type loginInput struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// refreshInput is the POST /auth/refresh DTO.
type refreshInput struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// Controller issues and rotates token pairs (the @Controller() equivalent).
type Controller struct {
	jwt   *fauth.Service
	store *fauth.MemoryRefreshStore
}

// NewController builds the auth controller with constructor injection.
func NewController(jwt *fauth.Service, store *fauth.MemoryRefreshStore) *Controller {
	return &Controller{jwt: jwt, store: store}
}

// Login validates credentials and issues an access + refresh pair.
func (c *Controller) Login(ctx router.Context) error {
	in, err := pipes.Body[loginInput](ctx)
	if err != nil {
		return err
	}
	u, ok := users[in.Username]
	if !ok {
		return grove.Unauthorized("invalid credentials")
	}
	if err := fauth.ComparePassword(u.passwordHash, in.Password); err != nil {
		return grove.Unauthorized("invalid credentials")
	}
	roles := make([]any, 0, len(u.roles))
	for _, r := range u.roles {
		roles = append(roles, r)
	}
	pair, err := c.jwt.IssuePair(in.Username, map[string]any{"roles": roles})
	if err != nil {
		return err
	}
	if err := c.store.Store(ctx.Request().Context(), pair.RefreshID, in.Username, time.Unix(pair.RefreshExpiresAt, 0)); err != nil {
		return err
	}
	return ctx.JSON(200, map[string]any{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresAt - time.Now().Unix(),
		"token_type":    "Bearer",
	})
}

// Refresh redeems a single-use refresh token for the next pair.
func (c *Controller) Refresh(ctx router.Context) error {
	in, err := pipes.Body[refreshInput](ctx)
	if err != nil {
		return err
	}
	pair, err := fauth.Rotate(ctx.Request().Context(), c.jwt, c.store, in.RefreshToken)
	if err != nil {
		return grove.Unauthorized("invalid refresh token")
	}
	return ctx.JSON(200, map[string]any{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresAt - time.Now().Unix(),
		"token_type":    "Bearer",
	})
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

// AuthModule provides the JWT service, the refresh store, and the
// login/refresh controllers.
var AuthModule = &grove.ModuleDef{
	Name: "auth",
	Providers: []grove.Provider{
		grove.Provide(di.Singleton, func(*di.Container) (*fauth.Service, error) {
			return newJWTService(), nil
		}),
		grove.Provide0(di.Singleton, fauth.NewMemoryRefreshStore),
	},
	BuildControllers: func(app *grove.App) ([]grove.ControllerDef, error) {
		ctrl, err := grove.Wire2(app, NewController)
		if err != nil {
			return nil, err
		}
		return []grove.ControllerDef{{
			Prefix: "/auth",
			Tags:   []string{"auth"},
			Endpoints: []grove.Endpoint{
				grove.POST("/login", ctrl.Login, grove.WithSummary("Issue token pair")),
				grove.POST("/refresh", ctrl.Refresh, grove.WithSummary("Rotate token pair")),
			},
		}}, nil
	},
}
