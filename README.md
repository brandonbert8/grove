<div align="center">

# 🌿 Grove

**A modern, modular Go backend framework with NestJS-grade developer experience.**

_Composition over magic · compile-time over reflection · clear boundaries from `GET /hello` to microservices._

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Release](https://img.shields.io/badge/release-v0.2.0-blue)](https://github.com/brandonbert8/grove/tags)
[![CI](https://github.com/brandonbert8/grove/actions/workflows/ci.yml/badge.svg)](https://github.com/brandonbert8/grove/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Deps](https://img.shields.io/badge/deps-chi_%2B_x%2Fcrypto-lightgrey)](go.mod)

</div>

## ✨ Why Grove?

| 🧩 | **NestJS-style modules** — `ModuleDef` with imports, providers & controllers. No decorators, no magic: plain structs the compiler understands. |
|---|---|
| 💉 | **Explicit DI** — singletons, transients & lazy singletons with constructor injection. No reflection, no global container. |
| 🛡️ | **Guards & Auth** — `CanActivate`, JWT access + rotating refresh tokens, `bcrypt`, roles. |
| 🔍 | **Pipes** — validate bodies, query, headers & path params with a tag engine you can extend. |
| 🧪 | **Test harness** — drive your app through `httptest`, no booted server needed. |
| 📖 | **OpenAPI 3.1** — generated from your route metadata, served in one line. |
| ⚡ | **Chi engine** — `Grove → Chi → net/http`. Battle-tested routing, Grove-owned API. You never import Chi. |
| 🖥️ | **CLI** — `new` + `generate` scaffolding that actually compiles. |

## 🗺️ Coming from NestJS?

| NestJS | Grove |
|---|---|
| `@Module()` | `ModuleDef` + `AsModule()` |
| `@Controller()` / `@Get()` | `XxxController` + `ControllerDef` + `grove.GET()` with `WithSummary()` options |
| `@Injectable()` + constructor DI | `Provide` / `Provide0` + `grove.Inject[T]` / `grove.Wire(app, NewXxxController)` |
| `@UseGuards()` / `APP_GUARD` | `UseGuard()` / per-controller `Middleware` / `app.UseGuards()` |
| `@UseInterceptors()` / `APP_INTERCEPTOR` | `app.UseInterceptors()` + `grove.WrapData()` / `MapResponse()` |
| `ValidationPipe` + `@Body()` | `pipes.Body[T](ctx)` / `BindQuery` / `BindHeader` / `BindPath` |
| `ParseIntPipe` | `pipes.Path[int](c, "id")` — one generic, not one class per type |
| `@ApiOperation` + Swagger | `WithSummary/Tags/Responses/Security` options + `openapi.Mount` (+ `openapi.Lint`) |
| Passport JWT | `auth.Service` + `AuthGuard` + `RequireRole` |
| `bcrypt` | `auth.HashPassword` / `ComparePassword` |
| `OnModuleInit` / `OnShutdown` | `app.OnStart` / `app.OnStop` |
| Terminus | `HealthModule` / `HealthModuleWithChecks` |
| Testing module | `grovtest` (`RequireBody[T]`, `RequireStatus`) |
| `HttpException` | `HttpError` (+ machine-readable `details`) |
| `nest generate` | `grove generate` (`module\|controller\|service\|resource`) |
| `nest g resource` | `grove g resource users` (module + service + controller + smoke test) |

## 🚀 Quick Start

**Requires Go 1.26+.**

```bash
go install github.com/brandonbert8/grove/cmd/grove@latest
# or from source:
git clone https://github.com/brandonbert8/grove && cd grove && go build ./...
```

```bash
grove new myapp
cd myapp
go mod tidy
go run .
curl localhost:3000/hello   # {"message":"Hello Grove"}
curl localhost:3000/healthz # {"status":"ok"}
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
	app.Router.Use(
		middleware.Recovery(app.Logger),
		middleware.RequestID(),
		middleware.Logging(app.Logger),
	)

	app.MustRegister(grove.NewModule("hello", func(app *grove.App) error {
		if err := di.RegisterSingletonAs(app.Container, "Hello Grove"); err != nil {
			return err
		}
		msg, _ := di.ResolveAs[string](app.Container)
		app.Route("GET", "/hello", func(c router.Context) error {
			return c.JSON(200, map[string]string{"message": msg})
		})
		return nil
	}))

	app.MustRegister(grove.HealthModule("").AsModule())

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
# {"access_token":"...","refresh_token":"...","expires_in":3600,"token_type":"Bearer"}
curl localhost:3000/users -H "Authorization: Bearer <access_token>"
curl -X POST localhost:3000/auth/refresh -d '{"refresh_token":"<refresh_token>"}'
curl localhost:3000/openapi.json   # generated OpenAPI 3.1 spec
```

## 🧩 NestJS-style modules

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
        ctrl, err := grove.Wire(app, NewUsersController)
        if err != nil {
            return nil, err
        }
        return []grove.ControllerDef{{
            Prefix:     "/users",
            Tags:       []string{"users"},
            Security:   []string{"bearerAuth"},
            Middleware: []router.Middleware{guard}, // guards & interceptors
            Endpoints: []grove.Endpoint{
                grove.GET("", ctrl.List,
                    grove.WithSummary("List users"),
                    grove.WithResponses(map[int]string{200: "users"})),
                grove.GET("/{id}", ctrl.Get, grove.WithSummary("Get user")),
                grove.POST("", ctrl.Create,
                    grove.Use(fauth.RequireRole("admin")),
                    grove.WithSummary("Create user (admin)")),
            },
        }}, nil
    },
}
app.MustRegister(users.AsModule())
```

Handlers return typed errors the router maps automatically, and DTOs
validate through pipes:

```go
func (c *UsersController) Create(ctx router.Context) error {
    in, err := pipes.Body[CreateUserInput](ctx) // validate:"required,email,..."
    if err != nil {
        return err // 400 bad JSON, 422 failed validation (+ field details)
    }
    page, err := pipes.Query(ctx, "page", 1) // typed ?page with default
    if err != nil {
        return err // 400
    }
    _ = page
    return ctx.JSON(201, c.svc.Create(in))
}
```

## 🛡️ Middleware & Auth

Production-ready middleware, all `net/http`-friendly and streaming-safe:

| Middleware | What it does |
|---|---|
| `Recovery` | Panics → JSON 500 with stack-trace log |
| `RequestID` | Mint/reuse `X-Request-ID`, echo it, correlate logs |
| `Logging` | Method, path, status, latency (+ `request_id`) |
| `CORS` | Explicit allow-lists, preflights, `ExposeHeaders` |
| `Timeout` | Handler deadline → JSON 503 (race-safe writer) |
| `SecureHeaders` | `nosniff`, `DENY`, `no-referrer`, TLS-only HSTS |
| `RateLimit` | Per-IP token bucket → JSON 429 + honest `Retry-After` |

Auth without Passport-phobia:

```go
hash, _ := auth.HashPassword("secret")              // bcrypt
pair, _ := svc.IssuePair("ada", map[string]any{     // access + refresh
    "roles": []string{"admin"},
})
guard := auth.AuthGuard(svc)                        // Bearer guard
adminOnly := auth.RequireRole("admin")              // role guard
next, _ := auth.Rotate(ctx, svc, store, pair.RefreshToken) // single-use rotation
```

## 🧪 Testing

No booted servers. Drive the real app through `httptest`:

```go
cli := grovtest.New(app).Bearer(token)
body := grovtest.RequireBody[map[string]any](t,
    cli.Post(t, "/users", map[string]string{"name": "Ada"}), 201)
// 422s carry machine-readable details:
rec := cli.Post(t, "/users", map[string]string{"name": "x"})
grovtest.RequireStatus(t, rec, 422)
```

## 📖 OpenAPI

Document routes as data, serve the spec in one line:

```go
openapi.Mount(app, "/openapi.json", openapi.Info{
    Title: "My API", Version: "1.0.0",
})
// → paths, params, response codes, bearerAuth schemes — always in sync,
//    because the spec builds lazily from the real router.
```

## ⚙️ Configuration

`PORT` (or `GROVE_PORT`), `DATABASE_URL`, `LOG_LEVEL`
(`debug|info|warn|error`), `APP_NAME`, `ENV`, `HOST`, `JWT_SECRET` — via
environment or a `.env` file discovered from the working directory
upward.

```go
cfg, _ := config.Load()
fmt.Println(cfg.Addr()) // ":3000"

// Fail fast on secrets, catch GROVE_ typos:
cfg := config.MustLoad(
    config.WithRequired("JWT_SECRET", "DATABASE_URL"),
    config.WithStrict(),
)
svc := auth.NewService([]byte(cfg.JWTSecret), cfg.AppName)
```

Plus lifecycle hooks for the real world:

```go
app.OnStart(pool.Connect)  // abort boot on error — OnModuleInit
app.OnStop(pool.Close)     // reversed, after HTTP drain — OnShutdown
```

## 🖥️ CLI

```text
◆ Grove

A structured backend framework for Go.

Usage:
  grove [command]

Commands:
  generate    Generate a Grove component
  new         Create a new Grove application
  version     Print the Grove CLI version

Aliases:
  generate, g

Examples:
  grove new api
  grove g module users
```

```bash
grove new demo --grove-version v0.2.0   # pin the framework version
grove g module billing                  # scaffold + register in main.go
grove g resource billing                # full slice: module + service + controller + smoke test
grove g controller billing              # add a controller (reports wiring)
grove g service billing                 # add a service (patches providers)
grove version                           # CLI + Go + OS
grove --no-color new demo               # plain output for CI/logs
grove --verbose new demo                # debug details
```

Generated code compiles — CI even builds a fresh scaffold to prove it.

## 🏗️ Project Structure

```
grove/
├── cmd/grove/            # CLI entrypoint (new/generate/version)
├── examples/hello/       # Minimal example: GET /hello with DI
├── examples/nest/        # NestJS-style: modules, auth, validation
├── packages/
│   ├── core/             # App, Module, ModuleDef, HttpError, guards, lifecycle, health, docs
│   ├── router/           # Router interface + Chi engine, Body binding
│   ├── di/               # Singleton/transient/lazy-singleton container, generics
│   ├── config/           # Typed Config, .env + environment loading
│   ├── logger/           # Logger interface + slog backend
│   ├── middleware/       # Recovery, Logging, CORS, RequestID, Timeout, SecureHeaders, RateLimit
│   ├── pipes/            # Validation engine, Body/Query/Header/Path binding, Page, strict mode
│   ├── auth/             # JWT pairs + rotation, bcrypt, AuthGuard, RequireRole/AnyRole
│   ├── grovtest/         # HTTP test harness (JSON client, Decode, RequireStatus)
│   ├── openapi/          # OpenAPI 3.1 from App.Docs (+Mount)
│   └── cli/              # CLI implementation (new + generate)
├── internal/             # Private module internals (no public API)
├── docs/architecture.md  # Design rationale
├── scripts/              # Dev helpers
```

Package rules: `core` composes the leaf packages; leaves never import
`core` or each other (except `middleware` → `router, logger`;
`pipes`/`auth` → `router, core` for HttpError). No cycles, no global
mutable state.

HTTP engine: `Grove → Chi → net/http`. Chi
(`github.com/go-chi/chi/v5`) is the internal router — matching, path
params, groups, 404/405 — while Grove owns handlers, middleware,
`router.Context`, and JSON errors. App code never imports Chi; see
`docs/architecture.md`. Escape hatches: `ctx.Request()`,
`ctx.ResponseWriter()`, `app.Handler()`.

## 🗺️ Roadmap

**Phase 2 (done)** — `generate` commands, `ModuleDef` with imports,
`ControllerDef` route tables, guards/interceptors (`CanActivate`,
`UseGuard`, `Chain`), typed `HttpError` mapped by the router, request
validation (`pipes`), JWT auth package, lazy singleton factories.

**v0.2.0 (done)** — Chi as the default HTTP engine (`Grove → Chi →
net/http`), JSON 404/405, `RequestID`/`Timeout`/`SecureHeaders`/
`RateLimit` middleware, lifecycle hooks (`OnStart`/`OnStop`) +
`HealthModule`, endpoint docs metadata + OpenAPI 3.1 (`openapi`),
`grovtest` harness, typed `Path`/`Query` pipes + custom rules,
`uuid`/`url` validators, error `details` over the wire, working
`grove new` scaffold — plus hardening: mutex-safe registration,
lazy `Status()`, strict `BindQuery`, streaming-safe logging,
stack-trace recovery, honest `Retry-After`, drain-before-stop,
bcrypt + refresh rotation, required/strict config, self-wiring
`generate`, lazy OpenAPI with query/responses/bearer docs, and CI.

**Phase 3** — strict provider export scoping, `go generate` compile-time
DI wiring, declarative param binding, OTel middleware.
(Config schema via `config.Schema` and docs lint via `openapi.Lint` already shipped.)

**Later** — WebSockets (`packages/ws`), gRPC (`packages/grpc`), GraphQL,
cron jobs, queue workers, plugin system, Docker/K8s scaffolds.

See `docs/architecture.md` for where each piece integrates.

## 📄 License

MIT — see [LICENSE](LICENSE). Built with 💚 and stdlib-first Go.
