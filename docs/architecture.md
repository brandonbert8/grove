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
| `core` (`packages/core`) | `App`, `Module`, `ModuleDef` + `ControllerDef` + `Endpoint` (+docs metadata), `HttpError` (+details), guards/interceptors (`CanActivate`, `UseGuard`, `Chain`), lifecycle (`Run`, `OnStart`/`OnStop`, `HealthModule`, `Docs`) | `middleware`, `pipes`, `auth`, `cli` |
| `router` (`packages/router`) | `Router` interface, `Context` (+`Body` binding), Chi-backed `DefaultRouter`, `Statuser` error mapping, `FromHTTP` std adapter | `core`, `di`, `config` |
| `di` (`packages/di`) | `Container`, singleton/transient/lazy-singleton lifetimes, generic helpers, exported `KeyFor` | any sibling package |
| `config` (`packages/config`) | Typed `Config`, `.env` parsing, env precedence | any sibling package |
| `logger` (`packages/logger`) | `Logger` interface, `slog` backend | any sibling package |
| `middleware` (`packages/middleware`) | Recovery (+stack), Logging (+request_id, streaming-safe), CORS (+expose), RequestID, Timeout (JSON 503), SecureHeaders, RateLimit (honest Retry-After, proxy-aware) over `router` | `core` |
| `cli` (`packages/cli`) | cobra tree (`new`, `generate`, `version`), generators returning `FileChange` events, `ui` design system (theme, no-color, verbose), ldflags/buildinfo versioning | `core` internals beyond templates |
| `pipes` (`packages/pipes`) | Tag validation engine (`required,min,max,len,gte,lte,email,uuid,url,oneof` + `RegisterRule` + `ValidateStrict`), `ValidateBody`, `BindQuery` (strict), `BindHeader`, `BindPath`, `Page`, typed `Parse`/`Path`/`Query` | `router`, `core` (HttpError only) |
| `auth` (`packages/auth`) | HS256 JWT + bcrypt passwords, access/refresh pairs with rotation, `AuthGuard`, `RequireRole`/`RequireAnyRole` | `router`, `core` (HttpError, guards only) |
| `grovtest` (`packages/grovtest`) | HTTP test harness: JSON client, `Decode`, `RequireStatus` | sibling packages (imports `core` App only) |
| `openapi` (`packages/openapi`) | OpenAPI 3.1 from `App.Docs` (+`Mount`) | `router`, `core` (App only) |

Dependency direction: bottom layer `{router, di, config, logger}` ←
`core` ← `{middleware, pipes, auth, grovtest, openapi}`; `cli` is
standalone (templates only); `examples/*` compose everything. `core` never imports upward, so there are
no cycles.

## HTTP engine: Grove → Chi → net/http

Grove is NOT a router. Chi (`github.com/go-chi/chi/v5`, the only
runtime dependency) is the internal HTTP engine; `net/http` remains
the transport. Layering:

```text
Grove App / Modules / Controllers / Endpoints
   ↓  (Grove Router API: Handle/Use/Group/Routes, HandlerFunc, Middleware)
Chi (method+path matching, path params, 404/405 dispatch)
   ↓
net/http
```

Rules:

- Application code never imports Chi. Controllers register through
  `ControllerDef`/`Endpoint`, guards and pipes stay Grove types, and
  `Param()` reads Chi URL params internally (`chi.URLParam` with a
  `PathValue` fallback for contexts built outside the mux).
- `DefaultRouter` keeps the pre-Chi `Router` interface (same `Use`,
  verbs, `Handle`, `Group`, `Routes`) so modules and the CLI are
  untouched. Route patterns keep the `{id}` syntax, which Chi and the
  old `ServeMux` share; an empty endpoint path still mounts exactly at
  its group prefix. Registration is fail-fast (unknown methods panic
  with a Grove message, never poison the mutex) and last-wins: GET
  routes mirror HEAD for ServeMux parity unless HEAD is explicit, and
  re-registration replaces instead of shadowing.
- Middleware layering: **global** (`app.Router.Use`) wraps `ServeHTTP`
  around the whole mux, so Recovery/Logging/CORS observe every request
  including 404/405 and preflights. **Group/route** middleware composes
  per route (outermost group first, route innermost). No Chi middleware
  is used; std `func(http.Handler) http.Handler` middleware adapts in
  via `router.FromHTTP`.
- Error representation is Grove's: handler errors map through
  `router.Statuser` (usually `*grove.HttpError`), and unmatched paths
  / wrong methods answer Grove-shaped JSON
  (`{"error","status"}`) instead of stdlib text/plain.
- Escape hatches: `ctx.Request()`, `ctx.ResponseWriter()`, and
  `app.Handler() http.Handler` (httptest, custom `http.Server`,
  proxies, serverless). Chi is swappable later by re-implementing the
  small `Router` interface — no Gin/Echo/Fiber adapters are planned
  for v1.

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
- **Validation without extra dependencies.** `pipes` implements its own tag
  engine (`required,min,max,len,gte,lte,email,oneof` + nesting) rather
  than pulling a validation library, keeping the framework's leaves
  dependency-free (Chi is the only runtime dependency, confined to
  `packages/router`).
  Unknown rules are ignored so custom tags coexist.
- **JWT without dependencies.** HS256 with `crypto/hmac` + stdlib
  base64/JSON. Fail-closed: empty secret panics at construction, malformed
  and expired tokens are 401s. Password hashing alone uses
  `golang.org/x/crypto/bcrypt` — the framework's only non-Chi dependency.
- **Generated code must compile.** `grove generate` output is verified by
  building it inside the module in CI fashion; constructor-name agreement
  across files is covered by unit tests.

## DX layer (v0.2.0): NestJS ergonomics, Go runtime

Grove copies NestJS developer experience, never its runtime magic:

- **Lifecycle hooks.** `app.OnStart` (validate before listen: pools,
  crons, caches — `OnModuleInit`) and `app.OnStop` reversed best-effort
  (`OnApplicationShutdown`). Shutdown drains HTTP first so in-flight
  handlers keep their resources, then releases them. `Run` also sets
  Slowloris-safe header/read/write timeouts; `WithShutdownTimeout`
  bounds the whole sequence.
- **Health.** `grove.HealthModule("")` mounts `GET /healthz` like any
  module — no special server path. `HealthModuleWithChecks` adds
  Terminus-style indicators with 200/503 semantics and a per-check
  timeout.
- **Docs as data.** `grove.WithSummary/WithDescription/WithTags/
  WithQuery/WithResponses/WithSecurity` options on `GET/POST/...` (the
  `@ApiOperation`/`@ApiQuery`/`@ApiResponse`/`@ApiBearerAuth`
  equivalent) fill `Endpoint{Summary, ...}`, recorded into `App.Docs()`
  at `Register` time (`App.Route` and `App.RecordDocs` cover non-module
  routes; raw `Router` use stays invisible by design);
  `packages/openapi` renders OpenAPI 3.1 — paths, params, response
  codes, bearer schemes — and lazy `openapi.Mount` serves it in one
  line, always current. `openapi.Lint` audits missing summaries and
  responses like a docs-coverage `vet`.
- **Pipes beyond bodies.** `pipes.Body[T]` decodes + validates DTOs in
  one line (the `@Body()` + `ValidationPipe` equivalent);
  `pipes.Path[T]` / `pipes.Query[T]` coerce params with `*HttpError`
  400s (the `ParseIntPipe` equivalent, but one generic instead of one
  class per type); `RegisterRule` adds custom validators (the
  custom-decorator equivalent); `uuid`/`url` join the tag engine.
- **Controllers, not handlers.** User route structs are named
  `XxxController` (the `@Controller()` equivalent); `router.HandlerFunc`
  stays the low-level function type. `grove.Inject` / `grove.Wire` /
  `grove.Wire2` / `grove.ControllerFor` remove `ResolveAs` boilerplate
  from `BuildControllers`, and `app.UseGuards/UseInterceptors/UsePipes`
  are the `APP_GUARD` / `APP_INTERCEPTOR` / `APP_PIPE` equivalent.
- **Rich errors over the wire.** `HttpError.Details` (e.g. validation
  field failures) now serializes as `{"error","status","details"}` via
  a structural router↔core contract — no import cycle.
- **Guards that survive JWT round-trips.** `RequireRole` accepts
  `[]any`, `[]string`, and single-string role claims (JSON decode
  yields `[]any`; hand-built claims usually `[]string`); plus
  `RequireAnyRole`.
- **Real auth.** `bcrypt` password hashing, access + single-use refresh
  pairs (`IssuePair`/`Rotate` over a `RefreshStore`, in-memory
  included), secrets wired from `config` (`JWT_SECRET` +
  `WithRequired` fail-fast).
- **Test harness.** `packages/grovtest` drives `App.Handler()` through
  `httptest` with JSON bodies, `Bearer`, `Decode[T]`, `RequireStatus`
  — the `Test.createTestingModule` equivalent without a server.

## Future integration points

All reserved in `packages/core/extensions.go` (guards, interceptors, and
pipes are implemented since Phase 2; lifecycle hooks, health, docs,
grovtest, and the middleware pack since v0.2.0; the rest remains reserved):

- **Services** — plain structs with constructors in `di` (`Provide` for
  injecting constructors, `Provide0` for zero-arg ones).
- **WebSockets** (`packages/ws`), **gRPC** (`packages/grpc`),
  **GraphQL** (`packages/graphql`) — sibling transports composed with
  `App`, never inside `core`'s HTTP path.
- **Cron** (`packages/cron`), **Queue** (`packages/queue`) — background
  runners started/stopped with `App` lifecycle.
- **Plugins** — `Plugin.Apply(app)` before `app.Run`.
- **Phase 3** — strict provider export scoping, `go generate`
  compile-time DI wiring, declarative handler binding, OTel
  middleware. (Rate-limit/timeout middleware and the test harness,
  once Phase 3, shipped in v0.2.0.)
