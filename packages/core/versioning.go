package grove

import (
	"strings"

	"github.com/brandonbert8/grove/packages/router"
)

// VersioningType selects how clients request an API version.
type VersioningType int

const (
	// VersioningURI puts the version in the path: /v1/users.
	VersioningURI VersioningType = iota
	// VersioningHeader reads the version from a request header
	// (default Accept-Version).
	VersioningHeader
	// VersioningMediaType reads the version from Accept:
	// application/vnd.grove.v1+json.
	VersioningMediaType
)

type versioningConfig struct {
	enabled         bool
	typ             VersioningType
	defaultVersion  string
	versionPrefix   string
	header          string
	mediaTypePrefix string
}

// VersioningOption customizes EnableVersioning.
type VersioningOption func(*versioningConfig)

// WithVersioningType selects URI, header, or media-type versioning.
func WithVersioningType(t VersioningType) VersioningOption {
	return func(c *versioningConfig) { c.typ = t }
}

// WithDefaultVersion sets the version used when a controller/endpoint
// has none and the client sends no version.
func WithDefaultVersion(v string) VersioningOption {
	return func(c *versioningConfig) { c.defaultVersion = v }
}

// WithVersionPrefix sets the URI segment prefix (default "v", so version
// "1" mounts at /v1). Empty disables the prefix.
func WithVersionPrefix(p string) VersioningOption {
	return func(c *versioningConfig) { c.versionPrefix = p }
}

// WithVersionHeader sets the header name for VersioningHeader
// (default "Accept-Version").
func WithVersionHeader(name string) VersioningOption {
	return func(c *versioningConfig) { c.header = name }
}

// WithVersionMediaType sets the Accept media-type prefix for
// VersioningMediaType (default "application/vnd.grove").
func WithVersionMediaType(prefix string) VersioningOption {
	return func(c *versioningConfig) { c.mediaTypePrefix = prefix }
}

// EnableVersioning turns on API versioning. Per-controller and
// per-endpoint Version fields override the default.
//
//	app.EnableVersioning(grove.WithDefaultVersion("1"))
//	// GET /v1/users, GET /v2/users when an endpoint sets Version: "2"
func (a *App) EnableVersioning(opts ...VersioningOption) {
	cfg := versioningConfig{
		enabled:         true,
		typ:             VersioningURI,
		versionPrefix:   "v",
		header:          "Accept-Version",
		mediaTypePrefix: "application/vnd.grove",
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	a.versioning = cfg
}

// VersioningEnabled reports whether EnableVersioning was called.
func (a *App) VersioningEnabled() bool { return a.versioning.enabled }

// VersioningType returns the configured strategy (URI by default).
func (a *App) VersioningType() VersioningType { return a.versioning.typ }

// DefaultVersion returns the configured default version, if any.
func (a *App) DefaultVersion() string { return a.versioning.defaultVersion }

// effectiveVersion resolves endpoint > controller > app default.
func (a *App) effectiveVersion(c ControllerDef, e Endpoint) string {
	if !a.versioning.enabled {
		return ""
	}
	v := e.Version
	if v == "" {
		v = c.Version
	}
	if v == "" {
		v = a.versioning.defaultVersion
	}
	return a.normalizeVersion(v)
}

// normalizeVersion trims whitespace and the configured "v" prefix.
func (a *App) normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if p := a.versioning.versionPrefix; p != "" {
		v = strings.TrimPrefix(v, p)
	}
	return v
}

// versionSegment renders "1" as "v1" (or "1" with an empty prefix).
func (a *App) versionSegment(v string) string {
	v = a.normalizeVersion(v)
	if v == "" {
		return ""
	}
	if p := a.versioning.versionPrefix; p != "" && !strings.HasPrefix(v, p) {
		return p + v
	}
	return v
}

// versionedPrefix applies global prefix + URI version + controller
// prefix, unless the controller prefix is excluded (health/docs).
func (a *App) versionedPrefix(controllerPrefix, version string) string {
	if !a.versioning.enabled || a.versioning.typ != VersioningURI || version == "" {
		return a.prefixedPath(controllerPrefix)
	}
	if a.isExcluded(controllerPrefix) {
		return a.prefixedPath(controllerPrefix)
	}
	return joinPaths(a.globalPrefix, a.versionSegment(version), controllerPrefix)
}

// requestVersion extracts the normalized version from the request.
func (a *App) requestVersion(c router.Context) string {
	switch a.versioning.typ {
	case VersioningHeader:
		return a.normalizeVersion(c.Header(a.versioning.header))
	case VersioningMediaType:
		prefix := a.versioning.mediaTypePrefix
		if prefix == "" {
			return ""
		}
		for _, part := range strings.Split(c.Header("Accept"), ",") {
			mt := strings.TrimSpace(part)
			if i := strings.IndexByte(mt, ';'); i >= 0 {
				mt = strings.TrimSpace(mt[:i])
			}
			if !strings.HasPrefix(mt, prefix) {
				continue
			}
			rest := strings.TrimPrefix(mt, prefix)
			rest = strings.TrimPrefix(rest, ".")
			if i := strings.IndexByte(rest, '+'); i >= 0 {
				rest = rest[:i]
			}
			return a.normalizeVersion(rest)
		}
	}
	return ""
}

// versionDispatcher merges multiple header/media-type versions that
// share one method+path into a single router handler.
type versionDispatcher struct {
	app        *App
	versions   map[string]router.HandlerFunc
	registered bool
}

func (d *versionDispatcher) add(version string, h router.HandlerFunc) {
	if d.versions == nil {
		d.versions = make(map[string]router.HandlerFunc)
	}
	d.versions[version] = h
}

func (d *versionDispatcher) handle(c router.Context) error {
	v := d.app.requestVersion(c)
	if v == "" {
		v = d.app.normalizeVersion(d.app.versioning.defaultVersion)
	}
	if v == "" && len(d.versions) == 1 {
		for only := range d.versions {
			v = only
		}
	}
	h, ok := d.versions[v]
	if !ok {
		return NewHttpErrorCode(404, "API_VERSION_NOT_FOUND", "api version not supported")
	}
	return h(c)
}

func (a *App) versionDispatcher(key string) *versionDispatcher {
	if a.versionDispatchers == nil {
		a.versionDispatchers = make(map[string]*versionDispatcher)
	}
	d := a.versionDispatchers[key]
	if d == nil {
		d = &versionDispatcher{app: a, versions: make(map[string]router.HandlerFunc)}
		a.versionDispatchers[key] = d
	}
	return d
}

// joinPaths joins path segments with router.JoinPath.
func joinPaths(parts ...string) string {
	out := ""
	for _, p := range parts {
		out = router.JoinPath(out, p)
	}
	return out
}
