package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// LintResult is the `grove lint` report.
type LintResult struct {
	Warnings []string
}

// Lint audits the project: go.mod presence plus heuristic docs checks
// (controllers using grove.GET/POST without WithSummary/WithResponses).
// It is a fast AST scan, not a full docs-coverage vet: use
// openapi.Lint(app) in tests for authoritative per-route coverage.
func Lint(dir string) (LintResult, error) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return LintResult{}, fmt.Errorf("no go.mod in %s", dir)
	}
	var out []string
	fset := token.NewFileSet()
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		usesGrove := false
		hasSummary := false
		hasResponses := false
		ast.Inspect(src, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "grove" {
					usesGrove = true
					if sel.Sel.Name == "WithSummary" {
						hasSummary = true
					}
					if sel.Sel.Name == "WithResponses" {
						hasResponses = true
					}
				}
			}
			return true
		})
		if usesGrove && strings.Contains(path, "controller") && !hasSummary {
			rel, _ := filepath.Rel(dir, path)
			out = append(out, rel+": controller without grove.WithSummary (docs coverage)")
		}
		if usesGrove && strings.Contains(path, "controller") && !hasResponses {
			rel, _ := filepath.Rel(dir, path)
			out = append(out, rel+": controller without grove.WithResponses (docs coverage)")
		}
		return nil
	})
	return LintResult{Warnings: out}, nil
}

// WireResult is the `grove wire` report.
type WireResult struct {
	Providers int
	Warnings  []string
}

// WireCheck heuristically counts grove.Provide* wiring calls.
// It is a smoke scan (counts providers, warns when none found), not a
// duplicate-key prover: duplicate registration fails fast at runtime in
// ModuleDef.Register with module+provider names.
func WireCheck(dir string) (WireResult, error) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return WireResult{}, fmt.Errorf("no go.mod in %s", dir)
	}
	providers := 0
	var warnings []string
	fset := token.NewFileSet()
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		ast.Inspect(src, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "grove" {
					if strings.HasPrefix(sel.Sel.Name, "Provide") {
						providers++
					}
				}
			}
			return true
		})
		return nil
	})
	if providers == 0 {
		warnings = append(warnings, "no grove.Provide calls found (nothing to wire)")
	}
	return WireResult{Providers: providers, Warnings: warnings}, nil
}
