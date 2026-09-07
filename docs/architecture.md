# Grove Architecture

## Why a modular architecture

Grove applications are composed of **modules** (`packages/core.Module`):
a module wires routes, providers, and sub-resources into the `App`.
Modules never import sibling modules; shared code lives in packages they
both import. This gives:

- **Independent reasoning** — each module can be read, tested, and
  replaced in isolation.
- **Team scaling** — `users`, `billing`, and `health` evolve without
  merge conflicts in a central route file.
- **Phased growth** — controllers, guards, interceptors, cron jobs, and
  queue workers (see `packages/core/extensions.go`) all attach through
  the same `Module.Register(app)` seam, so Phase 2 features do not
  require re-architecting Phase 1 apps.

## Why compile-time friendly DI

`packages/di` resolves dependencies through **explicit constructor
injection**: factories receive the `*Container` and call `Resolve`
themselves. There is no struct-tag scanning, no interface-to-impl
guessing, and no hidden graph building. The only reflection is a single
`reflect.TypeOf` used to derive default generic keys — and even that is
documented as the seam where generated code plugs in.

Consequences:

- Wiring failures are ordinary Go errors returned from `Register` /
  `Resolve`, visible at startup and testable with plain unit tests.
- A future `grove generate` step can emit static wiring (plain
  constructor calls) that replaces container resolution with zero
  runtime cost and full `go vet` / dead-code coverage.
- No global container: each `App` owns its `*di.Container`, so tests
  run in parallel without shared mutable state.

## Why minimal reflection

Reflection defeats the Go toolchain: it hides call graphs from the
compiler, the linker, linters, and IDE navigation, and it moves errors
from build time to request time. Grove's rule is:

> If it can be a function call, it must be a function call.

Reflection is currently limited to DI key derivation
(`di.keyFor[T]`). Everything else — routing, middleware, config,
logging — is interfaces plus composition.

## Package responsibilities

| Package | Owns | Must NOT import |
|---|---|---|
| `core` (`packages/core`) | `App`, `Module`, `ModuleDef` + `ControllerDef` + `Endpoint`, `HttpError`, guards/interceptors (`CanActivate`, `UseGuard`, `Chain`), lifecycle (`Run`) | `middleware`, `pipes`, `auth`, `cli` |
| `router` (`packages/router`) | `Router` interface, `Context` (+`Body` binding), `DefaultRouter` (stdlib `ServeMux`), `Statuser` error mapping | `core`, `di`, `config` |
| `di` (`packages/di`) | `Container`, singleton/transient/lazy-singleton lifetimes, generic helpers, exported `KeyFor` | any sibling package |
| `config` (`packages/config`) | Typed `Config`, `.env` parsing, env precedence | any sibling package |
| `logger` (`packages/logger`) | `Logger` interface, `slog` backend | any sibling package |
| `middleware` (`packages/middleware`) | Recovery, Logging, CORS over `router` | `core` |
| `cli` (`packages/cli`) | `new` scaffolding, `generate` (module/controller/service), `version` | `core` internals beyond templates |
| `pipes` (`packages/pipes`) | Tag validation engine, `ValidateBody`, `BindQuery` | `router`, `core` (HttpError only) |
| `auth` (`packages/auth`) | HS256 JWT service, `AuthGuard`, `RequireRole` | `router`, `core` (HttpError, guards only) |

Dependency direction: bottom layer `{router, di, config, logger}` ←
`core` ← `{middleware, pipes, auth}`; `cli` is standalone (templates only);
`examples/*` compose everything. `core` never imports upward, so there are
no cycles.

## Phase 2 design notes

- **No decorators, no magic.** Go cannot attach metadata to methods, so
  NestJS decorators map to plain data: `Endpoint{Method, Path, Handler}`
  instead of `@Get()`, `ModuleDef{Imports, Providers, Controllers}`
  instead of `@Module()`. Route tables are ordinary slices the compiler
  and `go vet` fully understand.
- **Lazy singletons.** `di.RegisterSingletonFactory` builds on first
  resolve and caches, so provider order within and across modules never
  matters and import cycles surface as errors at resolve time, not as
  initialization-order bugs.
- **Typed errors instead of exception filters.** Handlers return
  `*grove.HttpError`; the router maps any `router.Statuser` to its status
  automatically. No filter chain to configure for the 95% case.
- **Validation without dependencies.** `pipes` implements its own tag
  engine (`required,min,max,len,gte,lte,email,oneof` + nesting) rather
  than pulling a validation library, keeping `go.mod` dependency-free.
  Unknown rules are ignored so custom tags coexist.
- **JWT without dependencies.** HS256 with `crypto/hmac` + stdlib
  base64/JSON. Fail-closed: empty secret panics at construction, malformed
  and expired tokens are 401s.
- **Generated code must compile.** `grove generate` output is verified by
  building it inside the module in CI fashion; constructor-name agreement
  across files is covered by unit tests.

## Future integration points

All reserved in `packages/core/extensions.go` (guards, interceptors, and
pipes are implemented since Phase 2; the rest remains reserved):

- **Services** — plain structs with constructors in `di` (`Provide` for
  injecting constructors, `Provide0` for zero-arg ones).
- **WebSockets** (`packages/ws`), **gRPC** (`packages/grpc`),
  **GraphQL** (`packages/graphql`) — sibling transports composed with
  `App`, never inside `core`'s HTTP path.
- **Cron** (`packages/cron`), **Queue** (`packages/queue`) — background
  runners started/stopped with `App` lifecycle.
- **Plugins** — `Plugin.Apply(app)` before `app.Run`.
- **Phase 3** — strict provider export scoping, `go generate`
  compile-time DI wiring, declarative handler binding, test harness.
