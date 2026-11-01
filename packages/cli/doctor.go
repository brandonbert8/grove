package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newGraphCommand builds `grove graph`: source-level module import graph.
func newGraphCommand(g *globalOpts) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Print the module import graph",
		Long: "Print the module import graph from *grove.ModuleDef literals.\n" +
			"Source-level and conservative: modules built dynamically at runtime\n" +
			"may be missing. Use App.ModuleGraph() for the runtime view.",
		Example: "  grove graph\n  grove graph --format dot\n  grove graph --format json",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := renderer(cmd, g)
			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not graph", err.Error(), "", err)
			}
			res, err := Graph(cwd)
			if err != nil {
				return fail(r, 1, "Could not graph", err.Error(), "Run inside a Grove project (a directory with go.mod).", err)
			}
			for _, n := range res.Notices {
				r.Warning(n)
			}
			out := cmd.OutOrStdout()
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "mermaid", "mmd":
				fmt.Fprint(out, RenderGraphMermaid(res))
			case "dot", "graphviz":
				fmt.Fprint(out, RenderGraphDOT(res))
			case "json":
				s, err := RenderGraphJSON(res)
				if err != nil {
					return fail(r, 1, "Could not render graph", err.Error(), "", err)
				}
				fmt.Fprint(out, s)
			default:
				r.Failure("Unknown graph format", fmt.Sprintf("format %q is not supported", format), "Use --format mermaid|dot|json.")
				return &exitError{code: 2, err: &UsageError{Msg: "unknown graph format"}}
			}
			if len(res.Modules) == 0 {
				r.Muted("No modules found.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "mermaid", "output format: mermaid|dot|json")
	return cmd
}

// newDoctorCommand builds `grove doctor`: one audit across modules,
// wiring, docs and configuration.
func newDoctorCommand(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Audit modules, wiring, docs and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := renderer(cmd, g)
			r.Header("")
			cwd, err := os.Getwd()
			if err != nil {
				return fail(r, 1, "Could not run doctor", err.Error(), "", err)
			}

			var problems []string
			lintRes, err := Lint(cwd)
			if err != nil {
				return fail(r, 1, "Could not run doctor", err.Error(), "Run inside a Grove project (a directory with go.mod).", err)
			}
			problems = append(problems, lintRes.Warnings...)

			wireRes, err := WireCheck(cwd)
			if err != nil {
				return fail(r, 1, "Could not run doctor", err.Error(), "", err)
			}
			problems = append(problems, wireRes.Warnings...)

			graphRes, err := Graph(cwd)
			if err != nil {
				return fail(r, 1, "Could not run doctor", err.Error(), "", err)
			}
			if len(graphRes.Modules) == 0 {
				problems = append(problems, "no *grove.ModuleDef literals found (source graph is empty)")
			}
			archRules, err := LoadArchRules(cwd)
			if err != nil {
				return fail(r, 1, "Could not read grove.arch.yaml", err.Error(), "Fix the YAML or remove the file.", err)
			}
			if archRules == nil || archRules.ForbidCycles {
				problems = append(problems, graphCycleProblems(graphRes)...)
			}
			problems = append(problems, archRules.Check(graphRes)...)
			problems = append(problems, graphRes.Notices...)

			for _, p := range problems {
				r.Warning(p)
			}
			if len(problems) > 0 {
				r.Failure(fmt.Sprintf("%d problem(s)", len(problems)), "Fix the warnings above, then rerun grove doctor.", "")
				return &exitError{code: 1, err: fmt.Errorf("doctor found problems")}
			}
			r.Success(fmt.Sprintf("Doctor clean: %d module(s), %d provider(s)", len(graphRes.Modules), wireRes.Providers))
			return nil
		},
	}
}

// graphCycleProblems reports cycles in a source-level graph.
func graphCycleProblems(res GraphResult) []string {
	adj := make(map[string][]string, len(res.Modules))
	for _, e := range res.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(res.Modules))
	var stack []string
	var problems []string

	var visit func(string)
	visit = func(n string) {
		state[n] = visiting
		stack = append(stack, n)
		for _, next := range adj[n] {
			switch state[next] {
			case unvisited:
				visit(next)
			case visiting:
				start := 0
				for i, v := range stack {
					if v == next {
						start = i
						break
					}
				}
				cycle := append(append([]string(nil), stack[start:]...), next)
				problems = append(problems, "module import cycle: "+strings.Join(cycle, " -> "))
			}
		}
		stack = stack[:len(stack)-1]
		state[n] = visited
	}

	for _, m := range res.Modules {
		if state[m.Name] == unvisited {
			visit(m.Name)
		}
	}
	return problems
}
