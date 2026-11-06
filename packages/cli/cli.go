package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/brandonbert8/grove/packages/cli/ui"
)

// globalOpts holds persistent flags. One instance per root command —
// never package globals, so tests can build isolated trees.
type globalOpts struct {
	noColor bool
	verbose bool
}

// UsageError marks argument problems. Main maps it to exit code 2.
// Usage optionally names the help command that explains correct usage.
type UsageError struct {
	Msg   string
	Usage string
}

// Error implements error.
func (e *UsageError) Error() string { return e.Msg }

// exitError carries an already-rendered failure to Main with its code.
type exitError struct {
	code int
	err  error
}

// Error implements error.
func (e *exitError) Error() string { return e.err.Error() }

// Unwrap exposes the wrapped error.
func (e *exitError) Unwrap() error { return e.err }

// fail renders an actionable block and returns it for exit mapping.
func fail(r *ui.Renderer, code int, title, detail, hint string, err error) error {
	r.Failure(title, detail, hint)
	if err != nil {
		r.Verbosef("underlying error: %v", err)
	}
	return &exitError{code: code, err: errors.New(title)}
}

const (
	groupCreate  = "create"
	groupDevelop = "develop"
	groupInspect = "inspect"
	groupOther   = "other"
)

// NewRootCommand builds the Grove command tree:
//
//	grove
//	├── new [project] [--force] [--grove-version]
//	├── generate (g) module|controller|service|resource [name]
//	├── dev|start|build|test
//	├── routes|graph|doctor|lint|wire
//	└── version
func NewRootCommand() *cobra.Command {
	g := &globalOpts{}
	root := &cobra.Command{
		Use:     "grove",
		Short:   "A structured backend framework for Go",
		Long:    "A structured backend framework for Go.\n\nCreate a project, generate modules, and inspect routes without leaving\nthe terminal. The g alias is shorthand for generate.",
		Version: CLIVersion(),
		Example: "  grove new api\n  cd api\n  grove dev\n  grove g module users",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommandError(cmd, args[0])
			}
			return nil
		},
		SilenceErrors: true,
		SilenceUsage:  true,
		// A Run keeps bare `grove` on help; anything else fails arg
		// validation as "unknown command" (exit 2) instead of
		// silently showing help like parent commands do by default.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	// No shell-completion command: noise for a three-command CLI.
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().BoolVar(&g.noColor, "no-color", false, "disable colored output (also: NO_COLOR, non-TTY)")
	root.PersistentFlags().BoolVar(&g.verbose, "verbose", false, "show debug details (timings, paths, errors)")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &UsageError{Msg: err.Error(), Usage: cmd.CommandPath() + " --help"}
	})
	root.SetHelpTemplate(helpTemplate)
	root.AddGroup(
		&cobra.Group{ID: groupCreate, Title: "Create"},
		&cobra.Group{ID: groupDevelop, Title: "Develop"},
		&cobra.Group{ID: groupInspect, Title: "Inspect"},
		&cobra.Group{ID: groupOther, Title: "Other"},
	)

	newCmd := newNewCommand(g)
	newCmd.GroupID = groupCreate
	generateCmd := newGenerateCommand(g)
	generateCmd.GroupID = groupCreate
	devCmd := newDevCommand(g)
	devCmd.GroupID = groupDevelop
	startCmd := newStartCommand(g)
	startCmd.GroupID = groupDevelop
	buildCmd := newBuildCommand(g)
	buildCmd.GroupID = groupDevelop
	testCmd := newTestCommand(g)
	testCmd.GroupID = groupDevelop
	routesCmd := newRoutesCommand(g)
	routesCmd.GroupID = groupInspect
	graphCmd := newGraphCommand(g)
	graphCmd.GroupID = groupInspect
	doctorCmd := newDoctorCommand(g)
	doctorCmd.GroupID = groupInspect
	lintCmd := newLintCommand(g)
	lintCmd.GroupID = groupInspect
	wireCmd := newWireCommand(g)
	wireCmd.GroupID = groupInspect
	versionCmd := newVersionCommand()
	versionCmd.GroupID = groupOther

	root.AddCommand(newCmd, generateCmd, devCmd, startCmd, buildCmd, testCmd, routesCmd, graphCmd, doctorCmd, lintCmd, wireCmd, versionCmd)
	root.InitDefaultHelpCmd()
	for _, c := range root.Commands() {
		if c.Name() == "help" {
			c.GroupID = groupOther
		}
	}
	return root
}

// unknownCommandError renders a typo-friendly usage error with
// suggestions for the closest command names.
func unknownCommandError(cmd *cobra.Command, name string) error {
	msg := fmt.Sprintf("unknown command %q for %q", name, cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(name); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?"
		for _, s := range suggestions {
			msg += "\n\t" + s
		}
	}
	return &UsageError{Msg: msg, Usage: cmd.CommandPath() + " --help"}
}

// noArgs rejects positional arguments with command-specific guidance.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return &UsageError{
		Msg:   fmt.Sprintf("unexpected argument %q for %q", args[0], cmd.CommandPath()),
		Usage: cmd.CommandPath() + " --help",
	}
}

// renderer builds the ui Renderer for a command from global flags.
func renderer(cmd *cobra.Command, g *globalOpts) *ui.Renderer {
	return rendererFor(cmd.OutOrStdout(), g)
}

// diagnostics renders warnings and failures on stderr so machine-readable
// stdout (routes --format json, graph --format mermaid) stays clean.
func diagnostics(cmd *cobra.Command, g *globalOpts) *ui.Renderer {
	return rendererFor(cmd.ErrOrStderr(), g)
}

func rendererFor(out io.Writer, g *globalOpts) *ui.Renderer {
	return ui.New(out,
		ui.WithColor(ui.DetectColor(out, g.noColor)),
		ui.WithVerbose(g.verbose),
	)
}

// Main runs the CLI and maps errors to exit codes. Commands render
// their own output; Main never prints except for pre-command failures
// (flag parsing, unknown commands).
func Main(args []string) int {
	root := NewRootCommand()
	root.SetArgs(args)
	return runRoot(root)
}

// runRoot executes an already-configured tree, mapping errors to exit
// codes: 0 success, 2 usage error, 1 runtime failure. Tests build the
// tree with NewRootCommand, redirect output, and call runRoot.
func runRoot(root *cobra.Command) int {
	if err := root.Execute(); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			return ee.code
		}
		errOut := root.ErrOrStderr()
		r := ui.New(errOut, ui.WithColor(ui.DetectColor(errOut, false)))
		msg := err.Error()
		var ue *UsageError
		if errors.As(err, &ue) {
			usage := ue.Usage
			if usage == "" {
				usage = "grove --help"
			}
			r.Failure("Command failed", ue.Msg, fmt.Sprintf(`Run "%s" for usage.`, usage))
			return 2
		}
		code := 1
		if isUsageMessage(msg) {
			code = 2
		}
		r.Failure("Command failed", msg, `Run "grove --help" for usage.`)
		return code
	}
	return 0
}

// isUsageMessage classifies cobra argument errors that do not carry a
// UsageError yet still mean "wrong invocation" (exit 2, not 1).
func isUsageMessage(msg string) bool {
	for _, marker := range []string{
		"unknown command",
		"unknown flag",
		"flag needs an argument",
		"invalid argument",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// helpTemplate is the compact branded help. It mirrors cobra's default
// sections without the noise.
const helpTemplate = `◆ Grove

{{with .Long}}{{. | trimTrailingWhitespaces}}{{else}}{{with .Short}}{{. | trimTrailingWhitespaces}}{{end}}{{end}}

{{if or .Runnable .HasAvailableSubCommands}}Usage:
{{if .Runnable}}  {{.UseLine}}
{{end}}{{if .HasAvailableSubCommands}}  {{.CommandPath}} [command]
{{end}}{{end}}
{{if gt (len .Aliases) 0}}Aliases:
  {{.NameAndAliases}}
{{end}}{{if .HasAvailableSubCommands}}{{if eq (len .Groups) 0}}Commands:
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}  {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}
{{else}}{{range $group := .Groups}}{{.Title}}:
{{range $cmd := $.Commands}}{{if (and (eq $cmd.GroupID $group.ID) (or $cmd.IsAvailableCommand (eq $cmd.Name "help")))}}  {{rpad $cmd.Name $cmd.NamePadding }} {{$cmd.Short}}
{{end}}{{end}}
{{end}}{{if not .AllChildCommandsHaveGroup}}Additional Commands:
{{range .Commands}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}  {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}
{{end}}{{end}}{{end}}{{if .HasExample}}Examples:
{{.Example}}
{{end}}
{{if .HasAvailableLocalFlags}}Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}{{if .HasAvailableInheritedFlags}}Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}{{if .HasAvailableSubCommands}}Use "{{.CommandPath}} [command] --help" for more information about a command.
{{end}}`

// newNewCommand builds `grove new`.
func newNewCommand(g *globalOpts) *cobra.Command {
	var force bool
	var groveVersion string
	cmd := &cobra.Command{
		Use:   "new <project>",
		Short: "Create a new Grove application",
		Long: "Create a ready-to-run Grove application with a module, routes\n" +
			"and docs. The project is created in the current directory.",
		Example: "  grove new api\n  grove new github.com/acme/api --force",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := renderer(cmd, g)
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				r.Failure("Missing project name",
					"Usage: grove new <project>\nExample: grove new my-api",
					`Run "grove new --help" for details.`)
				return &exitError{code: 2, err: &UsageError{Msg: "project name required"}}
			}
			if len(args) > 1 {
				r.Failure("Too many arguments",
					"grove new accepts exactly one project name.",
					`Run "grove new --help" for usage.`)
				return &exitError{code: 2, err: &UsageError{Msg: "too many arguments"}}
			}
			name := args[0]
			r.Header(CLIVersion())
			r.Verbosef("command=new project=%q force=%v", name, force)

			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not create project", err.Error(), "", err)
			}
			// Fail fast on a taken destination before any slow step
			// (version resolution hits the network).
			base, _, err := resolveBase(cwd, name)
			if err != nil {
				var ue *UsageError
				if errors.As(err, &ue) {
					return fail(r, 2, "Cannot create project", ue.Msg,
						"Usage: grove new <project> [--force].", err)
				}
				return fail(r, 1, "Could not create project", err.Error(), "", err)
			}
			if _, err := os.Stat(base); err == nil && !force {
				ee := &ExistsError{Path: base}
				return fail(r, 1, "Could not create project",
					fmt.Sprintf("Directory %q already exists.", displayPath(cwd, base)),
					"Choose another project name or remove the directory (or --force).", ee)
			}
			r.Muted(fmt.Sprintf("Creating Grove application %q...", displayName(name)))

			override := groveVersion
			if override == "" {
				override = os.Getenv("GROVE_VERSION")
			}
			gv, err := ResolveGroveVersion(cmd.Context(), CLIVersion(), override, nil)
			if err != nil {
				return fail(r, 1, "Could not resolve Grove version", err.Error(),
					"Pass --grove-version vX.Y.Z or set GROVE_VERSION.", err)
			}
			r.Verbosef("grove dependency version: %s", gv)

			base, changes, err := Scaffold(cwd, name, force, gv)
			if err != nil {
				var ee *ExistsError
				if errors.As(err, &ee) {
					// Raced by a concurrent creation; same message as pre-check.
					return fail(r, 1, "Could not create project",
						fmt.Sprintf("Directory %q already exists.", displayPath(cwd, base)),
						"Choose another project name or remove the directory (or --force).", err)
				}
				return fail(r, 1, "Could not create project", err.Error(), "", err)
			}
			r.FileChanges(changes)
			r.Verbosef("project dir: %s", base)
			r.Success("Project created successfully")
			r.Section("Next steps:", []string{
				r.Command("cd " + displayPath(cwd, base)),
				r.Command("go mod tidy"),
				r.Command("grove dev"),
				"",
				"→ " + r.URL("http://localhost:3000"),
				"",
				r.Command("grove g module users"),
				r.Command("grove routes"),
			})
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "replace the directory if it exists")
	cmd.Flags().StringVar(&groveVersion, "grove-version", "", "framework version to require (default: auto-resolve)")
	return cmd
}

// newGenerateCommand builds `grove generate` (alias `g`) with kind
// subcommands: module, controller, service.
func newGenerateCommand(g *globalOpts) *cobra.Command {
	parent := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"g"},
		Short:   "Generate a Grove component",
		Long: "Generate modules, controllers, services, resources, guards,\n" +
			"pipes, filters or interceptors from templates.",
		Example: "  grove generate module users\n  grove g controller users\n  grove g service users\n  grove g resource billing\n  grove g guard jwt\n  grove g pipe lower\n  grove g filter domain\n  grove g interceptor tracing",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommandError(cmd, args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	for _, kind := range []string{"module", "controller", "service", "resource", "guard", "pipe", "filter", "interceptor"} {
		kind := kind
		example := generateExampleName(kind)
		sub := &cobra.Command{
			Use:     kind + " <name>",
			Short:   fmt.Sprintf("Generate %s %s", article(kind), kind),
			Example: fmt.Sprintf("  grove g %s %s\n  grove generate %s %s", kind, example, kind, example),
			Args:    cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				r := renderer(cmd, g)
				if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
					r.Failure("Missing name",
						fmt.Sprintf("Usage: grove g %s <name>\nExample: grove g %s %s", kind, kind, example),
						fmt.Sprintf(`Run "grove g %s --help" for details.`, kind))
					return &exitError{code: 2, err: &UsageError{Msg: "component name required"}}
				}
				if len(args) > 1 {
					r.Failure("Too many arguments",
						fmt.Sprintf("grove g %s accepts exactly one name.", kind),
						fmt.Sprintf(`Run "grove g %s --help" for usage.`, kind))
					return &exitError{code: 2, err: &UsageError{Msg: "too many arguments"}}
				}
				return runGenerate(cmd, r, kind, args[0])
			},
		}
		parent.AddCommand(sub)
	}
	return parent
}

// article returns the indefinite article for a generator kind.
func article(kind string) string {
	if strings.EqualFold(strings.TrimSpace(kind), "interceptor") {
		return "an"
	}
	return "a"
}

// generateExampleName returns a realistic sample name for help text.
func generateExampleName(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "guard":
		return "jwt"
	case "pipe":
		return "lower"
	case "filter":
		return "domain"
	case "interceptor":
		return "tracing"
	default:
		return "users"
	}
}

// runGenerate executes one generator and renders its FileChanges.
func runGenerate(cmd *cobra.Command, r *ui.Renderer, kind, name string) error {
	r.Header(CLIVersion())
	r.Verbosef("command=generate kind=%s name=%q", kind, name)
	cwd, err := os.Getwd()
	if err != nil {
		return fail(r, 1, "Could not generate", err.Error(), "", err)
	}
	res, err := Generate(cwd, kind, name)
	if err != nil {
		var ue *UsageError
		if errors.As(err, &ue) {
			return fail(r, 2, "Cannot generate", ue.Msg,
				fmt.Sprintf("Usage: grove generate %s <name> (lowercase letters, digits, underscores).", kind), err)
		}
		return fail(r, 1, "Could not generate",
			err.Error(), "Run inside a Grove project (a directory with go.mod).", err)
	}
	r.FileChanges(res.Changes)
	for _, n := range res.Notices {
		r.Warning(n)
	}
	r.Success(fmt.Sprintf("%s %q generated", title(strings.ToLower(kind)), name))
	return nil
}

// newVersionCommand builds `grove version` (detailed; --version stays terse).
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Grove CLI version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "◆ Grove")
			fmt.Fprintf(out, "%-10s%s\n", "Grove CLI", CLIVersion())
			fmt.Fprintf(out, "%-10s%s\n", "Go", runtime.Version())
			fmt.Fprintf(out, "%-10s%s/%s\n", "OS", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}

// displayName renders the project label for creation messages.
func displayName(name string) string {
	return strings.TrimSuffix(name, "/")
}

// displayPath shortens p against cwd for output.
func displayPath(cwd, p string) string {
	if rel, err := filepath.Rel(cwd, p); err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}
