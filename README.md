# Grove

A modern, modular Go backend framework. Idiomatic Go first: composition over
magic, compile-time performance over runtime reflection, and clear package
boundaries that scale from a single `GET /hello` to an ecosystem of
microservices.

> Phase 2: NestJS-style modules, controllers, guards, pipes, JWT auth,
> validation, and a functional `generate` CLI. See [Roadmap](#roadmap).

## Installation

Requires **Go 1.25+**.

```bash
go install github.com/brandonbert8/grove/cmd/grove@latest
# or from source:
git clone https://github.com/brandonbert8/grove && cd grove && go build ./...
```

## Quick Start

```bash
grove new myapp
cd myapp
go mod tidy
go run .
curl localhost:3000/hello
# {"message":"Hello Grove"}
```

Or wire it up by hand:

```go
package main

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"github.com/brandonbert8/grove/packages/di"
	"github.com/brandonbert8/grove/packages/middleware"
	"github.com/brandonbert8/grove/packages/router"
)

func main() {
	app := grove.New()
	app.Router.Use(middleware.Recovery(app.Logger), middleware.Logging(app.Logger))

	app.MustRegister(grove.NewModule("hello", func(app *grove.App) error {
		if err := di.RegisterSingletonAs(app.Container, "Hello Grove"); err != nil {
			return err
		}
		msg, _ := di.ResolveAs[string](app.Container)
		app.Router.GET("/hello", func(c router.Context) error {
			return c.JSON(200, map[string]string{"message": msg})
		})
		return nil
	}))

	if err := app.Run(":3000"); err != nil {
		panic(err)
	}
}
```

Run the bundled examples:

```bash
go run ./examples/hello     # minimal: one module, one route
curl localhost:3000/hello

go run ./examples/nest      # NestJS-style: modules, JWT guards, validation
curl -X POST localhost:3000/auth/login -d '{"username":"admin","password":"secret"}'
curl localhost:3000/users -H "Authorization: Bearer <token>"
```

## NestJS-style modules

Go has no decorators, so Grove uses plain structs: a route table
(`Endpoint`) instead of `@Get()`, and a module definition instead of
`@Module()`. Everything is compile-time checked and reflection-free:

```go
users := &grove.ModuleDef{
    Name:    "users",
    Imports: []*grove.ModuleDef{auth.AuthModule}, // built first, exactly once
    Providers: []grove.Provider{
        grove.Provide0(di.Singleton, NewUsersService),
    },
    BuildControllers: func(app *grove.App) ([]grove.ControllerDef, error) {
        svc, err := di.ResolveAs[*UsersService](app.Container)
        if err != nil {
            return nil, err
        }
        h := NewUsersHandler(svc)
        return []grove.ControllerDef{{
            Prefix:     "/users",
            Middleware: []router.Middleware{guard}, // guards & interceptors
            Endpoints: []grove.Endpoint{
                grove.GET("", h.List),
                grove.GET("/{id}", h.Get),
                grove.POST("", h.Create, fauth.RequireRole("admin")),
            },
        }}, nil
    },
}
app.MustRegister(users.AsModule())
```

Handlers return typed errors the router maps automatically, and DTOs
validate through pipes:

```go
func (h *Handler) Create(c router.Context) error {
    var in CreateUserInput // struct tags: validate:"required,email,..."
    if err := pipes.ValidateBody(c, &in); err != nil {
        return err // 400 bad JSON, 422 failed validation
    }
    return c.JSON(201, h.svc.Create(in))
}
```

## Project Structure

```
grove/
├── cmd/grove/            # CLI entrypoint (new/generate/version)
├── examples/hello/       # Minimal example: GET /hello with DI
├── examples/nest/        # NestJS-style: modules, auth, validation
├── packages/
│   ├── core/             # App, Module, ModuleDef, HttpError, guards/interceptors
│   ├── router/           # Router interface + stdlib ServeMux backend, Body binding
│   ├── di/               # Singleton/transient/lazy-singleton container, generics
│   ├── config/           # Typed Config, .env + environment loading
│   ├── logger/           # Logger interface + slog backend
│   ├── middleware/       # Recovery, Logging, CORS
│   ├── pipes/            # Tag validation engine, ValidateBody, BindQuery
│   ├── auth/             # HS256 JWT service, AuthGuard, RequireRole
│   └── cli/              # CLI implementation (new + generate)
├── internal/             # Private module internals (no public API)
├── docs/architecture.md  # Design rationale
├── scripts/              # Dev helpers
```

Package rules: `core` composes the leaf packages; leaves never import
`core` or each other (except `middleware` → `router, logger`;
`pipes`/`auth` → `router, core` for HttpError). No cycles, no global
mutable state.

## Configuration

`PORT` (or `GROVE_PORT`), `DATABASE_URL`, `LOG_LEVEL`
(`debug|info|warn|error`), `APP_NAME`, `ENV`, `HOST` — via environment or
a `.env` file discovered from the working directory upward.

```go
cfg, _ := config.Load()
fmt.Println(cfg.Addr()) // ":3000"
```

## CLI

```
grove new <project>                  scaffold a new project
grove generate module <name>         scaffold modules/<name> (module+service+controller)
grove generate controller <name>     add a controller to modules/<name>
grove generate service <name>        add a service to modules/<name>
grove version                        print the CLI version
```

## Roadmap

**Phase 2 (done)** — `generate` commands, `ModuleDef` with imports,
`ControllerDef` route tables, guards/interceptors (`CanActivate`,
`UseGuard`, `Chain`), typed `HttpError` mapped by the router, request
validation (`pipes`), JWT auth package, lazy singleton factories.

**Phase 3** — strict provider export scoping, `go generate` compile-time
DI wiring, declarative handler binding, config schema validation, test
helpers (`grovtest` harness), rate-limit + timeout + OTel middleware.

**Later** — WebSockets (`packages/ws`), gRPC (`packages/grpc`), GraphQL,
cron jobs, queue workers, plugin system, Docker/K8s scaffolds.

See `docs/architecture.md` for where each piece integrates.
