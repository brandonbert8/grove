package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// Version is the fallback CLI version, bumped per release. Real builds
// override it: -ldflags wins, then go install build metadata.
//
//	ldflags: go build -ldflags "-X github.com/brandonbert8/grove/packages/cli.versionOverride=v0.2.0" ./cmd/grove
const Version = "0.2.0"

// versionOverride is set at link time for releases (see above).
var versionOverride string

// GroveModule is the framework module scaffolded projects require.
const GroveModule = "github.com/brandonbert8/grove"

// semver matches v1.2.3 with optional prerelease, v prefix optional.
var semver = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?$`)

// CLIVersion resolves the effective CLI version: explicit ldflags,
// then a clean release tag from build metadata (go install @vX.Y.Z),
// then the Version fallback.
func CLIVersion() string {
	if versionOverride != "" {
		return versionOverride
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if isCleanRelease(bi.Main.Version) {
			return bi.Main.Version
		}
	}
	return Version
}

// isCleanRelease reports vX.Y.Z with no prerelease/dev markers — a tag
// the framework necessarily published, since CLI and framework share
// the repo and its tags.
func isCleanRelease(v string) bool {
	m := semver.FindStringSubmatch(v)
	return m != nil && m[4] == "" && strings.HasPrefix(v, "v")
}

// normalizeVersion validates user input (--grove-version) and returns
// the canonical v-prefixed form.
func normalizeVersion(v string) (string, error) {
	v = strings.TrimSpace(v)
	m := semver.FindStringSubmatch(v)
	if m == nil {
		return "", fmt.Errorf("invalid version %q: want vX.Y.Z", v)
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v, nil
}

// VersionLister returns published framework versions, newest last.
type VersionLister func(ctx context.Context) ([]string, error)

// defaultVersionLister asks the Go toolchain (module proxy). It needs
// network; callers surface a clear error offline. It runs in a fresh
// empty directory so the caller's go.mod (which may require an
// unresolvable Grove) can never poison the query.
func defaultVersionLister(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	scratch, err := os.MkdirTemp("", "grove-versions-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-versions", GroveModule)
	cmd.Dir = scratch
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cannot reach the Go module proxy: %w", err)
	}
	var vers []string
	for _, tok := range strings.Fields(string(out)) {
		if semver.MatchString(tok) {
			if !strings.HasPrefix(tok, "v") {
				tok = "v" + tok
			}
			vers = append(vers, tok)
		}
	}
	if len(vers) == 0 {
		return nil, fmt.Errorf("no published versions found for %s", GroveModule)
	}
	return vers, nil
}

// maxVersion picks the highest stable version, falling back to the
// highest prerelease when nothing stable exists.
func maxVersion(vers []string) string {
	best, bestPre := "", ""
	for _, v := range vers {
		m := semver.FindStringSubmatch(v)
		if m == nil {
			continue
		}
		if m[4] == "" {
			if versionKey(v) > versionKey(best) {
				best = v
			}
		} else if versionKey(v) > versionKey(bestPre) {
			bestPre = v
		}
	}
	if best != "" {
		return best
	}
	return bestPre
}

// versionKey renders a comparable key for v ("": lowest).
func versionKey(v string) string {
	m := semver.FindStringSubmatch(v)
	if m == nil {
		return ""
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return fmt.Sprintf("%010d.%010d.%010d", maj, min, patch)
}

// ResolveGroveVersion picks the framework version new projects require:
//
//  1. explicit override (--grove-version / GROVE_VERSION),
//  2. a clean release CLI (same repo, same tags: the tag must exist),
//  3. newest published version via the toolchain (needs network).
//
// Anything else is a hard, actionable error — never a require line
// pointing at a revision that does not exist.
func ResolveGroveVersion(ctx context.Context, cliVersion, override string, list VersionLister) (string, error) {
	if strings.TrimSpace(override) != "" {
		v, err := normalizeVersion(override)
		if err != nil {
			return "", err
		}
		return v, nil
	}
	if isCleanRelease(cliVersion) {
		return cliVersion, nil
	}
	if list == nil {
		list = defaultVersionLister
	}
	vers, err := list(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot determine a published Grove version (dev CLI, offline?): %w — pass --grove-version vX.Y.Z or set GROVE_VERSION", err)
	}
	if best := maxVersion(vers); best != "" {
		return best, nil
	}
	return "", fmt.Errorf("no usable Grove versions published")
}
