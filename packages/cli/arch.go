package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ArchModuleRule constrains one module's imports. AllowImports is a
// whitelist (empty means any); DenyImports is a blacklist ("*" denies
// every import).
type ArchModuleRule struct {
	AllowImports []string
	DenyImports  []string
}

// ArchRules is the optional grove.arch.yaml contract enforced by
// `grove doctor`. It is intentionally small: module import rules and an
// optional cycle check, parsed without a YAML dependency.
type ArchRules struct {
	ForbidCycles bool
	Modules      map[string]ArchModuleRule
}

// LoadArchRules reads grove.arch.yaml from dir. A missing file returns
// (nil, nil): architecture rules are opt-in.
func LoadArchRules(dir string) (*ArchRules, error) {
	path := filepath.Join(dir, "grove.arch.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rules, err := parseArchRules(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rules, nil
}

// parseArchRules understands the constrained grove.arch.yaml subset:
//
//	forbid_cycles: true
//	modules:
//	  users:
//	    allow_imports: [auth, database]
//	  billing:
//	    deny_imports:
//	      - users
func parseArchRules(src string) (*ArchRules, error) {
	rules := &ArchRules{ForbidCycles: true, Modules: make(map[string]ArchModuleRule)}
	var (
		inModules   bool
		module      string
		listKey     string
		listIndent  int
		moduleRules = map[string]*ArchModuleRule{}
	)
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimRight(stripYAMLComment(raw), " \r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.ContainsRune(line, '\t') {
			return nil, fmt.Errorf("line %d: tabs are not allowed for indentation", i+1)
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		text := strings.TrimSpace(line)

		if listKey != "" && indent > listIndent && strings.HasPrefix(text, "- ") {
			item := unquoteYAML(strings.TrimSpace(strings.TrimPrefix(text, "- ")))
			appendArchRule(moduleRules[module], listKey, item)
			continue
		}
		listKey = ""

		if indent == 0 {
			inModules = false
			module = ""
			key, value, ok := splitYAMLPair(text)
			if !ok {
				continue
			}
			switch key {
			case "forbid_cycles":
				rules.ForbidCycles = parseYAMLBool(value)
			case "modules":
				inModules = true
			}
			continue
		}
		if !inModules {
			continue
		}
		if indent == 2 {
			key, value, ok := splitYAMLPair(text)
			if !ok || strings.TrimSpace(value) != "" {
				continue
			}
			module = unquoteYAML(key)
			if module == "" {
				continue
			}
			if _, ok := moduleRules[module]; !ok {
				moduleRules[module] = &ArchModuleRule{}
			}
			continue
		}
		if module == "" {
			continue
		}
		key, value, ok := splitYAMLPair(text)
		if !ok {
			continue
		}
		switch key {
		case "allow_imports", "deny_imports":
			if strings.TrimSpace(value) == "" {
				listKey = key
				listIndent = indent
				continue
			}
			for _, item := range parseYAMLList(value) {
				appendArchRule(moduleRules[module], key, item)
			}
		}
	}
	for name, r := range moduleRules {
		rules.Modules[name] = *r
	}
	return rules, nil
}

// Check reports violations of the module import rules.
func (r *ArchRules) Check(res GraphResult) []string {
	if r == nil {
		return nil
	}
	imports := make(map[string][]string, len(res.Modules))
	for _, e := range res.Edges {
		imports[e.From] = append(imports[e.From], e.To)
	}
	known := make(map[string]bool, len(res.Modules))
	for _, m := range res.Modules {
		known[m.Name] = true
	}
	var problems []string
	for _, m := range res.Modules {
		rule, ok := r.Modules[m.Name]
		if !ok {
			continue
		}
		for _, imp := range imports[m.Name] {
			if len(rule.AllowImports) > 0 && !archContains(rule.AllowImports, imp) {
				problems = append(problems, fmt.Sprintf("module %q imports %q but allow_imports permits %s", m.Name, imp, strings.Join(rule.AllowImports, ", ")))
			}
			if archContains(rule.DenyImports, "*") || archContains(rule.DenyImports, imp) {
				problems = append(problems, fmt.Sprintf("module %q imports forbidden module %q", m.Name, imp))
			}
		}
	}
	for name := range r.Modules {
		if !known[name] {
			problems = append(problems, fmt.Sprintf("grove.arch.yaml references unknown module %q", name))
		}
	}
	return problems
}

func appendArchRule(r *ArchModuleRule, key, item string) {
	if r == nil || item == "" {
		return
	}
	switch key {
	case "allow_imports":
		r.AllowImports = append(r.AllowImports, item)
	case "deny_imports":
		r.DenyImports = append(r.DenyImports, item)
	}
}

func archContains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func splitYAMLPair(s string) (key, value string, ok bool) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
}

func parseYAMLList(s string) []string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := unquoteYAML(strings.TrimSpace(part)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func unquoteYAML(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseYAMLBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "on", "1":
		return true
	default:
		return false
	}
}

func stripYAMLComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return s[:i]
		}
	}
	return s
}
