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

// watchRun runs `go run .` and restarts it whenever a .go file under dir
// changes. Polling compares a mtime+size fingerprint: stdlib only, no
// fsnotify dependency.
func watchRun(ctx context.Context, dir string, intervalMs int) error {
	if intervalMs <= 0 {
		intervalMs = 500
	}
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()
	last, err := fingerprintGoFiles(dir)
	if err != nil {
		return err
	}
	for {
		child, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- runGoChild(child, dir) }()
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
				cur, ferr := fingerprintGoFiles(dir)
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
					cur, ferr := fingerprintGoFiles(dir)
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

// runGoChild runs `go run .` attached to the terminal until it exits
// or ctx is cancelled (the child is killed on cancellation).
func runGoChild(ctx context.Context, dir string) error {
	c := exec.CommandContext(ctx, "go", "run", ".")
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	c.Env = os.Environ()
	return c.Run()
}

// fingerprintGoFiles hashes .go paths + mtime + size under dir,
// skipping vendor/ and dot-directories.
func fingerprintGoFiles(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == "vendor" || (strings.HasPrefix(name, ".") && path != dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
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
