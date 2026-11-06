package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newLintCommand builds `grove lint`: docs coverage + config schema audit.
func newLintCommand(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Short: "Audit docs coverage and configuration",
		Long: "Check controller docs coverage and configuration schema.\n" +
			"Exits 1 when warnings are found so CI can gate on it.",
		Example: "  grove lint",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := renderer(cmd, g)
			r.Header(CLIVersion())
			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not lint", err.Error(), "", err)
			}
			res, err := Lint(cwd)
			if err != nil {
				return fail(r, 1, "Could not lint", err.Error(), "Run inside a Grove project (a directory with go.mod).", err)
			}
			for _, w := range res.Warnings {
				r.Warning(w)
			}
			if len(res.Warnings) == 0 {
				r.Success("Lint clean: docs and config look good")
				return nil
			}
			r.Failure(fmt.Sprintf("%d warning(s)", len(res.Warnings)), "Add grove.WithSummary/WithResponses or fix config.", "")
			return &exitError{code: 1, err: fmt.Errorf("lint warnings")}
		},
	}
}

// newWireCommand builds `grove wire`: explain/verify DI wiring.
// v1 is runtime-only on purpose: `wire` is a wiring smoke check that
// counts grove.Provide* calls. `--emit` is experimental scaffolding for
// a possible post-v1 compile-time codegen and must not be required for
// production builds.
func newWireCommand(g *globalOpts) *cobra.Command {
	var emit bool
	cmd := &cobra.Command{
		Use:     "wire",
		Short:   "Count DI providers (wiring smoke check)",
		Long:    "Heuristically counts grove.Provide* calls without running the app.\n--emit is experimental (post-v1 research): it generates wire_gen.go\nscaffolding that is NOT required for v1 runtime DI (grove.Inject/Wire\nplus request-scoped grove.ProvideRequest/grove.InjectRequest).\nDuplicate keys fail fast at runtime in ModuleDef.Register.",
		Example: "  grove wire\n  grove wire --emit",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := renderer(cmd, g)
			r.Header(CLIVersion())
			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not wire", err.Error(), "", err)
			}
			if emit {
				r.Warning("--emit is experimental and not needed for v1 runtime DI")
				files, notices, err := WireEmit(cwd)
				if err != nil {
					return fail(r, 1, "Could not emit wiring", err.Error(), "", err)
				}
				for _, n := range notices {
					r.Warning(n)
				}
				if len(files) == 0 {
					r.Muted("Nothing to emit: no emittable constructors found.")
					return nil
				}
				for _, f := range files {
					r.Muted("UPDATE " + f)
				}
				r.Success(fmt.Sprintf("Emitted %d wire_gen.go file(s)", len(files)))
				return nil
			}
			res, err := WireCheck(cwd)
			if err != nil {
				return fail(r, 1, "Could not wire", err.Error(), "", err)
			}
			for _, w := range res.Warnings {
				r.Warning(w)
			}
			r.Success(fmt.Sprintf("Wiring OK: %d provider(s) checked", res.Providers))
			return nil
		},
	}
	cmd.Flags().BoolVar(&emit, "emit", false, "experimental: generate wire_gen.go scaffolding (not needed for v1)")
	return cmd
}

// newBuildCommand builds `grove build`: vet + test + compile the project.
func newBuildCommand(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Vet, test and compile the project",
		Long: "Run go vet, go test and go build in order.\n" +
			"Stops at the first failing step.",
		Example: "  grove build",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := renderer(cmd, g)
			r.Header(CLIVersion())
			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not build", err.Error(), "", err)
			}
			for _, step := range [][]string{{"vet", "./..."}, {"test", "./..."}, {"build", "./..."}} {
				r.Muted("go " + joinArgs(step))
				if err := runGo(cmd, cwd, step...); err != nil {
					return fail(r, 1, "Build failed at go "+joinArgs(step), err.Error(), "Fix the errors above and retry.", err)
				}
			}
			r.Success("Build clean: vet + test + compile passed")
			return nil
		},
	}
}

// newStartCommand builds `grove start`: run the app (go run .).
func newStartCommand(g *globalOpts) *cobra.Command {
	var prod bool
	cmd := &cobra.Command{
		Use:   "start [-- app args...]",
		Short: "Run the application",
		Long: "Run the application with go run.\n" +
			"Pass application flags after --.",
		Args:    cobra.ArbitraryArgs,
		Example: "  grove start\n  grove start -- --port 8080\n  grove start --prod -- --port 8080",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := renderer(cmd, g)
			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not start", err.Error(), "", err)
			}
			if prod {
				r.Muted("Starting in production mode (ENV=production)")
				_ = os.Setenv("ENV", "production")
			}
			goArgs := append([]string{"run", "."}, args...)
			r.Muted("go " + joinArgs(goArgs))
			if err := runGo(cmd, cwd, goArgs...); err != nil {
				return fail(r, 1, "Application exited with an error", err.Error(), "", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&prod, "prod", false, "run with ENV=production")
	return cmd
}

// newTestCommand builds `grove test`: run the project test suite.
func newTestCommand(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "test [-- go test args...]",
		Short: "Run the project tests",
		Long: "Run go test ./... by default.\n" +
			"Pass a package or go test flags after --.",
		Example: "  grove test\n  grove test ./modules/users\n  grove test -- -race -run TestUsers",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := renderer(cmd, g)
			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not test", err.Error(), "", err)
			}
			goArgs := append([]string{"test"}, args...)
			if len(args) == 0 {
				goArgs = append(goArgs, "./...")
			}
			r.Muted("go " + joinArgs(goArgs))
			if err := runGo(cmd, cwd, goArgs...); err != nil {
				return fail(r, 1, "Tests failed", err.Error(), "", err)
			}
			r.Success("Tests passed")
			return nil
		},
	}
}

// newDevCommand builds `grove dev`: restart on source/config changes.
// The watcher is stdlib polling (mtime+size hash every interval): no
// new dependencies, coherent with the stdlib-first rule.
func newDevCommand(g *globalOpts) *cobra.Command {
	var interval int
	cmd := &cobra.Command{
		Use:   "dev [-- app args...]",
		Short: "Run with restart on file changes",
		Long: "Run the application and restart it when source or config files change.\n" +
			"Pass application flags after --.",
		Args:    cobra.ArbitraryArgs,
		Example: "  grove dev\n  grove dev -- --port 8080",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := renderer(cmd, g)
			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not start dev", err.Error(), "", err)
			}
			if interval <= 0 {
				interval = 500
			}
			r.Muted("Watching source and config files (poll every " + itoa(interval) + "ms, Ctrl+C to stop)...")
			return watchRun(cmd.Context(), cwd, interval, args)
		},
	}
	cmd.Flags().IntVar(&interval, "interval", 500, "poll interval in milliseconds")
	return cmd
}
