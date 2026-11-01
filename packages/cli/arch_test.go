package cli

import (
	"strings"
	"testing"
)

func TestParseArchRules(t *testing.T) {
	rules, err := parseArchRules(`
# architecture contract
forbid_cycles: false
modules:
  users:
    allow_imports: [auth, "database"]
  billing:
    deny_imports:
      - users
      - "*"
`)
	if err != nil {
		t.Fatal(err)
	}
	if rules.ForbidCycles {
		t.Fatal("ForbidCycles = true, want false")
	}
	if got := rules.Modules["users"].AllowImports; strings.Join(got, ",") != "auth,database" {
		t.Fatalf("users allow = %v", got)
	}
	if got := rules.Modules["billing"].DenyImports; strings.Join(got, ",") != "users,*" {
		t.Fatalf("billing deny = %v", got)
	}
}

func TestArchRulesCheck(t *testing.T) {
	rules := &ArchRules{
		ForbidCycles: true,
		Modules: map[string]ArchModuleRule{
			"users":   {AllowImports: []string{"auth"}},
			"billing": {DenyImports: []string{"users"}},
		},
	}
	res := GraphResult{
		Modules: []GraphModule{{Name: "users"}, {Name: "billing"}, {Name: "auth"}},
		Edges: []GraphEdge{
			{From: "users", To: "billing"},
			{From: "billing", To: "users"},
		},
	}
	problems := rules.Check(res)
	if len(problems) != 2 {
		t.Fatalf("problems = %v, want 2", problems)
	}
	if !strings.Contains(problems[0], "allow_imports") || !strings.Contains(problems[1], "forbidden") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestArchRulesUnknownModule(t *testing.T) {
	rules := &ArchRules{Modules: map[string]ArchModuleRule{"ghost": {}}}
	problems := rules.Check(GraphResult{Modules: []GraphModule{{Name: "users"}}})
	if len(problems) != 1 || !strings.Contains(problems[0], "unknown module") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestLoadArchRulesMissingIsOptIn(t *testing.T) {
	rules, err := LoadArchRules(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if rules != nil {
		t.Fatalf("rules = %+v, want nil", rules)
	}
}

func TestDoctorEnforcesArchRules(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"modules/users/module.go": `package users

import (
	grove "github.com/brandonbert8/grove/packages/core"
	"example.com/demo/modules/billing"
)

var UsersModule = &grove.ModuleDef{
	Name: "users",
	Imports: []*grove.ModuleDef{billing.BillingModule},
}
`,
		"modules/billing/module.go": `package billing

import grove "github.com/brandonbert8/grove/packages/core"

var BillingModule = &grove.ModuleDef{Name: "billing"}
`,
		"grove.arch.yaml": `modules:
  users:
    allow_imports: [auth]
`,
	})
	stdout, code := run(t, dir, "doctor")
	if code != 1 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "allow_imports") {
		t.Fatalf("output:\n%s", stdout)
	}
}

func TestDoctorRejectsInvalidArchYAML(t *testing.T) {
	dir := setupProject(t)
	writeGraphFixture(t, dir, map[string]string{
		"grove.arch.yaml": "modules:\n\tusers:\n",
	})
	stdout, code := run(t, dir, "doctor")
	if code != 1 {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "tabs are not allowed") {
		t.Fatalf("output:\n%s", stdout)
	}
}
