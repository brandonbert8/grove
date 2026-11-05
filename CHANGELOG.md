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
  registry, and validation engine.
- `.gitattributes` normalizes Go (and other text) files to LF for
  reproducible formatting checks across platforms.

### Changed

- `openapi.Mount` caches the encoded document and rebuilds only when
  `App.Docs` or registered security schemes change, instead of rebuilding
  and marshalling the spec on every request (~14x faster mounted spec in
  the benchmark, 200 → 15 allocs/op).
- `router.ServeHTTP` caches the composed global middleware chain and
  invalidates it on `Use`, removing per-request slice copies and wrapper
  construction (~7 fewer allocs/op with five global middlewares).
- `di.keyFor` memoizes derived keys per `reflect.Type`, so `ResolveAs` no
  longer runs reflection and allocates a key string on every call
  (~4x faster singleton resolves, 0 allocs/op).

### Fixed

- The CI gofmt step now fails when files are unformatted (`gofmt -l`
  alone exits successfully).
- `.env` parsing is split into a reader-based helper so it can be
  exercised directly by fuzz tests.

## [0.2.0]

See the v0.2.0 section in the README roadmap for the full feature list.
