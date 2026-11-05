package cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// runGo executes `go args...` in dir, streaming output to the command's
// writer. It honors ctx cancellation (Ctrl+C propagates to the child).
func runGo(cmd *cobra.Command, dir string, args ...string) error {
	c := exec.CommandContext(cmd.Context(), "go", args...)
	c.Dir = dir
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	c.Stdin = os.Stdin
	c.Env = os.Environ()
	return c.Run()
}

// joinArgs renders args for log lines.
func joinArgs(args []string) string { return strings.Join(args, " ") }

func itoa(n int) string { return strconv.Itoa(n) }

// watchRun runs `go run .` (plus app args) and restarts it whenever a
// watched source/config file under dir changes. Polling compares a
// mtime+size fingerprint: stdlib only, no fsnotify dependency.
func watchRun(ctx context.Context, dir string, intervalMs int, appArgs []string) error {
	if intervalMs <= 0 {
		intervalMs = 500
	}
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()
	last, err := fingerprintFiles(dir)
	if err != nil {
		return err
	}
	for {
		child, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- runGoChild(child, dir, appArgs) }()
		restart := false
	loop:
		for {
			select {
			case <-ctx.Done():
				cancel()
				<-done
				return nil
			case err := <-done:
				// App exited on its own: report and wait for a change
				// before restarting (crash loops rest on change).
				cancel()
				if err != nil {
					_, _ = fmt.Fprintln(os.Stderr, "dev: app exited:", err)
				}
				break loop
			case <-ticker.C:
				cur, ferr := fingerprintFiles(dir)
				if ferr != nil {
					cancel()
					<-done
					return ferr
				}
				if cur != last {
					last = cur
					_, _ = fmt.Fprintln(os.Stderr, "dev: change detected, restarting...")
					cancel()
					<-done
					restart = true
					break loop
				}
			}
		}
		if !restart {
			// Wait for the next change before relaunching.
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					cur, ferr := fingerprintFiles(dir)
					if ferr != nil {
						return ferr
					}
					if cur != last {
						last = cur
						_, _ = fmt.Fprintln(os.Stderr, "dev: change detected, restarting...")
						goto relaunch
					}
				}
			}
		}
	relaunch:
	}
}

// runGoChild runs `go run .` (plus app args) attached to the terminal
// until it exits or ctx is cancelled (the child is killed on cancellation).
func runGoChild(ctx context.Context, dir string, appArgs []string) error {
	args := append([]string{"run", "."}, appArgs...)
	c := exec.CommandContext(ctx, "go", args...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	c.Env = os.Environ()
	return c.Run()
}

// fingerprintFiles hashes watched paths + mtime + size under dir,
// skipping vendor/, dot-directories, generated output directories
// (build/, dist/, ...), and _test.go files (test edits do not change the
// app binary, so they should not restart the server).
func fingerprintFiles(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if skipGraphDir(name) || (strings.HasPrefix(name, ".") && path != dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !watchedFile(name) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		_, _ = fmt.Fprintf(h, "%s:%d:%d;", path, info.ModTime().UnixNano(), info.Size())
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// watchedFile reports whether a file change should restart `grove dev`.
func watchedFile(name string) bool {
	if strings.HasSuffix(name, "_test.go") {
		return false
	}
	switch {
	case strings.HasSuffix(name, ".go"),
		name == "go.mod",
		name == "go.sum",
		name == "go.work",
		name == "go.work.sum",
		strings.HasPrefix(name, ".env"),
		strings.HasSuffix(name, ".yaml"),
		strings.HasSuffix(name, ".yml"),
		strings.HasSuffix(name, ".json"),
		strings.HasSuffix(name, ".toml"),
		strings.HasSuffix(name, ".ini"),
		strings.HasSuffix(name, ".conf"):
		return true
	}
	return false
}
