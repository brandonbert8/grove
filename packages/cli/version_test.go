package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIsCleanRelease(t *testing.T) {
	for _, v := range []string{"v0.2.0", "v10.20.30"} {
		if !isCleanRelease(v) {
			t.Fatalf("%q must be a clean release", v)
		}
	}
	for _, v := range []string{"0.2.0", "(devel)", "", "v0.2.0-rc.1", "v0.2", "latest", "v0.2.0-dirty"} {
		if isCleanRelease(v) {
			t.Fatalf("%q must not be a clean release", v)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	for in, want := range map[string]string{"v1.2.3": "v1.2.3", "1.2.3": "v1.2.3", "v0.2.0-rc.1": "v0.2.0-rc.1"} {
		got, err := normalizeVersion(in)
		if err != nil || got != want {
			t.Fatalf("normalize(%q) = %q,%v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "v1.2", "1.2", "latest"} {
		if _, err := normalizeVersion(bad); err == nil {
			t.Fatalf("normalize(%q) must fail", bad)
		}
	}
}

func TestMaxVersion(t *testing.T) {
	vers := []string{"v0.1.0", "v0.10.0", "v0.2.0", "v0.10.0-rc.1", "junk"}
	if got := maxVersion(vers); got != "v0.10.0" {
		t.Fatalf("max = %q, want v0.10.0 (stable beats newer rc, numeric order)", got)
	}
	if got := maxVersion([]string{"v0.2.0-rc.1"}); got != "v0.2.0-rc.1" {
		t.Fatalf("rc fallback = %q", got)
	}
	if got := maxVersion(nil); got != "" {
		t.Fatalf("empty = %q", got)
	}
}

func TestResolveGroveVersion(t *testing.T) {
	ctx := context.Background()
	fake := func(vers []string, err error) VersionLister {
		return func(context.Context) ([]string, error) { return vers, err }
	}

	// Explicit override always wins (normalized).
	got, err := ResolveGroveVersion(ctx, "v0.2.0", "1.5.0", fake(nil, errors.New("unused")))
	if err != nil || got != "v1.5.0" {
		t.Fatalf("override = %q,%v", got, err)
	}
	if _, err := ResolveGroveVersion(ctx, "v0.2.0", "bogus", fake(nil, nil)); err == nil {
		t.Fatal("bad override must fail")
	}
	// Clean release CLI pins itself: same repo, same tags.
	got, err = ResolveGroveVersion(ctx, "v0.2.0", "", fake(nil, errors.New("unused")))
	if err != nil || got != "v0.2.0" {
		t.Fatalf("release CLI = %q,%v", got, err)
	}
	// Dev CLI asks the toolchain.
	got, err = ResolveGroveVersion(ctx, "(devel)", "", fake([]string{"v0.1.0", "v0.2.0"}, nil))
	if err != nil || got != "v0.2.0" {
		t.Fatalf("dev CLI = %q,%v", got, err)
	}
	// Offline dev CLI fails with guidance, never a fake pin.
	_, err = ResolveGroveVersion(ctx, "(devel)", "", fake(nil, errors.New("network down")))
	if err == nil || !strings.Contains(err.Error(), "--grove-version") {
		t.Fatalf("offline must guide, got %v", err)
	}
	_, err = ResolveGroveVersion(ctx, "0.2.0", "", fake(nil, nil))
	if err == nil {
		t.Fatal("const fallback is not a tag: must not pin")
	}
}
