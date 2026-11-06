# Changelog

All notable changes to Grove are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `grove routes` command: source-level (AST) HTTP route table with
  `--format table|json`, `--method`, and `--prefix` filters. Reports
  non-literal prefixes/paths as notices instead of guessing.
- Controller-level `ControllerDef.Version` is inherited by routes in
  `grove routes` output; endpoint-level `WithVersion` overrides it.
- `grove dev` watches configuration files (`.env*`, YAML/JSON/TOML/INI)
  and `go.work`/`go.work.sum`, skips test files and build output
  directories, and accepts `-- app args...`.
- `grove start` accepts `-- app args...` (for example
  `grove start --prod -- --port 8080`).
- CI matrix across Ubuntu, Windows, and macOS, a hard gofmt gate,
  coverage artifact, and advisory staticcheck/govulncheck jobs.
- Fuzz tests for JWT verification, `.env` parsing, validation tags,
  router pattern matching, and route path joining.
- Benchmarks for the router, DI container, OpenAPI builder, metrics
  registry, validation engine, and logging middleware.
- `grove.Build`/`Build2-4` and `grove.BuildWith`/`BuildWith2-4` turn the
  usual `ModuleDef.Build` closure into one field, removing `nil, err`
  plumbing while keeping module-scoped DI and giving route tables access
  to the `ModuleContext` when they need it.
- CLI help is grouped by workflow (Create, Develop, Inspect), shows
  aliases and per-command examples, suggests close command names on
  typos, and gives command-specific guidance for missing arguments or
  extra positional args. Usage errors consistently exit `2`; machine
  output from `routes --format json` and `graph --format ...` stays
  pipeable because warnings are kept off stdout.
- `.gitattributes` normalizes Go (and other text) files to LF for
  reproducible formatting checks across platforms.

### Changed

- `openapi.Mount` caches the encoded document and rebuilds only when
  `App.Docs` or registered security schemes change, instead of rebuilding
  and marshalling the spec on every request (~14x faster mounted spec in
  the benchmark, 200 → 15 allocs/op).
- `router.ServeHTTP` caches the composed global middleware chain behind
  an atomic pointer, skips outer `Context` construction when no global
  middleware is registered, recycles Chi route contexts, and installs its
  per-request `Context` as the request context. Static routes drop from
  16 to 13 allocs/op, parameterized routes from 22 to 15, and 404s from
  16 to 12. Local A/B medians: static ~2.08µs → ~1.82µs, param
  ~2.95µs → ~2.29µs, five global middlewares ~2.54µs → ~1.74µs.
- `Context.Query`/`QueryOr` parse the query string once and reuse it;
  `QueryOr` no longer parses twice per call.
- `RequestID` skips request-context allocation when a valid inbound
  `X-Request-ID` is present. `Logging` combines its context override and
  status recorder into one allocation and preallocates its log fields
  (RequestID+Logging benchmark: 26 → 24 allocs/op).
- `di.keyFor` memoizes derived keys per `reflect.Type`, so `ResolveAs` no
  longer runs reflection and allocates a key string on every call
  (~4x faster singleton resolves, 0 allocs/op).
- `validate` caches parsed field metadata (JSON names and `validate`
  rules) per struct type and no longer allocates a cycle-tracking map it
  never used. Validation of a flat DTO drops from ~2.2µs / 9 allocs to
  ~0.47µs / 0 allocs per call (~4.7x faster).

### Fixed

- The CI gofmt step now fails when files are unformatted (`gofmt -l`
  alone exits successfully).
- `.env` parsing is split into a reader-based helper so it can be
  exercised directly by fuzz tests.

## [0.2.0]

See the v0.2.0 section in the README roadmap for the full feature list.
