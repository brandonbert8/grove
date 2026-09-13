package ui

import (
	"strings"
	"testing"
)

func TestPlainRendererHasNoANSI(t *testing.T) {
	var out strings.Builder
	r := New(&out, WithColor(false), WithVerbose(true))
	r.Header("v0.2.0")
	r.Success("ok")
	r.Failure("Title", "Detail line.", "Hint line.")
	r.Warning("careful")
	r.Info("note")
	r.Muted("dim")
	r.Verbosef("debug %d", 1)
	r.FileChanges([]FileChange{
		{Action: Create, Path: "a.go"},
		{Action: Update, Path: "b.go"},
		{Action: Skip, Path: "c.go"},
		{Action: Delete, Path: "d.go"},
	})
	got := out.String()
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("plain output must not contain ANSI:\n%q", got)
	}
	for _, want := range []string{
		"◆ Grove v0.2.0", "✔ ok", "✖ Title", "Detail line.", "Hint line.",
		"⚠ careful", "→ note", "dim", "[verbose] debug 1",
		"CREATE  a.go", "UPDATE  b.go", "SKIP    c.go", "DELETE  d.go",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestColoredRendererEmitsANSI(t *testing.T) {
	var out strings.Builder
	r := New(&out, WithColor(true))
	r.Success("ok")
	r.FileChanges([]FileChange{{Action: Create, Path: "a.go"}})
	got := out.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("colored output must contain ANSI:\n%q", got)
	}
	// Labels survive styling (ANSI wraps, never splits, the text).
	for _, want := range []string{"ok", "CREATE", "a.go"} {
		if !strings.Contains(got, want) {
			t.Fatalf("styled output missing %q:\n%q", want, got)
		}
	}
}

func TestVerboseOffHidesDebug(t *testing.T) {
	var out strings.Builder
	r := New(&out)
	r.Verbosef("hidden")
	if out.String() != "" {
		t.Fatalf("non-verbose must stay silent, got %q", out.String())
	}
}

func TestColorEnabledMatrix(t *testing.T) {
	if !colorEnabled(false, false, true) {
		t.Fatal("TTY without overrides must color")
	}
	for _, tc := range []struct {
		force, env, tty bool
	}{
		{true, false, true},
		{false, true, true},
		{false, false, false},
	} {
		if colorEnabled(tc.force, tc.env, tc.tty) {
			t.Fatalf("must be plain: %+v", tc)
		}
	}
}

func TestDetectColorNonFile(t *testing.T) {
	var out strings.Builder
	if DetectColor(&out, false) {
		t.Fatal("buffers are never TTYs")
	}
	if DetectColor(&out, true) {
		t.Fatal("force flag must win")
	}
}
