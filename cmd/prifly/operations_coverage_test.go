package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/examples"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func operationsFlagInventory() (map[string]map[string]bool, error) {
	inventory := map[string]map[string]bool{}
	files, err := filepath.Glob("*.go")
	if err != nil {
		return nil, err
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			return nil, err
		}
		for _, declaration := range f.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				receiver, ok := sel.X.(*ast.Ident)
				if !ok || receiver.Name != "f" {
					return true
				}
				index := 0
				switch sel.Sel.Name {
				case "String", "Bool", "Int", "Int64", "Uint64", "Duration":
				case "Var", "StringVar", "BoolVar", "IntVar", "Int64Var":
					index = 1
				default:
					return true
				}
				if len(call.Args) <= index {
					return true
				}
				value, ok := call.Args[index].(*ast.BasicLit)
				if !ok || value.Kind != token.STRING {
					return true
				}
				flag, err := strconv.Unquote(value.Value)
				if err != nil {
					return true
				}
				if inventory[fn.Name.Name] == nil {
					inventory[fn.Name.Name] = map[string]bool{}
				}
				inventory[fn.Name.Name][flag] = true
				return true
			})
		}
	}
	if len(inventory) == 0 {
		return nil, fmt.Errorf("CLI flag inventory read nothing")
	}
	return inventory, nil
}

func operationsFlagCoverage(page string, inventory map[string]map[string]bool) []string {
	documented := map[string]map[string]bool{}
	group := ""
	marker := regexp.MustCompile(`^<!-- cli-flags:([A-Za-z]+) -->$`)
	row := regexp.MustCompile("^\\| `--([a-z0-9-]+)` \\|")
	for _, line := range strings.Split(page, "\n") {
		if m := marker.FindStringSubmatch(line); len(m) != 0 {
			group = m[1]
			documented[group] = map[string]bool{}
		}
		if m := row.FindStringSubmatch(line); len(m) != 0 && group != "" {
			documented[group][m[1]] = true
		}
	}
	var failures []string
	for function, flags := range inventory {
		for flag := range flags {
			if !documented[function][flag] {
				failures = append(failures, function+" --"+flag+" is undocumented")
			}
		}
	}
	for function, flags := range documented {
		for flag := range flags {
			if !inventory[function][flag] {
				failures = append(failures, function+" --"+flag+" is no longer a public flag")
			}
		}
	}
	slices.Sort(failures)
	return failures
}

func TestOperationsHandbookCoversEveryCLIFlagAndHelpEntry(t *testing.T) {
	t.Parallel()
	page, err := examples.Handbook.ReadFile("operations/commands.md")
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := operationsFlagInventory()
	if err != nil {
		t.Fatal(err)
	}
	if failures := operationsFlagCoverage(string(page), inventory); len(failures) != 0 {
		t.Fatalf("flag coverage: %v", failures)
	}
	entries := 0
	for _, entry := range helpEntries() {
		if len(entry) == 0 {
			continue
		}
		entries++
		if !strings.Contains(string(page), entry[0]) {
			t.Errorf("public command form absent: %s", entry[0])
		}
	}
	if entries < 30 {
		t.Fatalf("help coverage read only %d entries", entries)
	}
	damaged := strings.Replace(string(page), "| `--state-root` |", "| omitted flag |", 1)
	if failures := operationsFlagCoverage(damaged, inventory); len(failures) == 0 {
		t.Fatal("coverage accepted a missing live flag")
	}
}

func TestOperationsCapabilityRoutesMatchInstalledEngine(t *testing.T) {
	t.Parallel()
	page, err := examples.Handbook.ReadFile("operations/capabilities.md")
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]bool{}
	pattern := regexp.MustCompile("(?m)^\\| `([a-z_.]+)` \\| \\[Процедура\\]\\(([^)]+)\\)")
	for _, match := range pattern.FindAllStringSubmatch(string(page), -1) {
		rows[match[1]] = true
		procedure, err := examples.Handbook.ReadFile("operations/" + match[2])
		if err != nil || len(procedure) < 500 {
			t.Errorf("capability %s has no substantial procedure: %v", match[1], err)
		}
	}
	declared := prifly.Capabilities().Profiles[1].Capabilities
	if len(rows) != len(declared) {
		t.Fatalf("routes=%d declared=%d", len(rows), len(declared))
	}
	for _, capability := range declared {
		if !rows[capability] {
			t.Errorf("missing operational route: %s", capability)
		}
	}
	for capability := range rows {
		if !slices.Contains(declared, capability) {
			t.Errorf("undelivered capability promised: %s", capability)
		}
	}
}

func TestCanonicalOperationsLinksExist(t *testing.T) {
	t.Parallel()
	pages, err := filepath.Glob("../../examples/operations/*.md")
	if err != nil || len(pages) < 13 {
		t.Fatalf("manual pages unavailable: %v", err)
	}
	for _, name := range pages {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range runnerMarkdownLink.FindAllStringSubmatch(string(data), -1) {
			target := strings.SplitN(match[1], "#", 2)[0]
			if target == "" || strings.Contains(target, ":") {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(name), filepath.FromSlash(target))); err != nil {
				t.Errorf("%s -> %s: %v", name, target, err)
			}
		}
	}
}

var operationsDispatchGroups = map[string]string{
	"run": "", "runCommand": "run", "projectCommand": "project", "projectWorkflowsCommand": "project workflows", "projectRunners": "project runners", "projectQueue": "project queue", "session": "session", "packages": "package", "claims": "claim", "control": "control", "approval": "approval", "grant": "grant", "capacity": "capacity", "artifact": "artifact", "source": "source", "task": "task", "telemetry": "telemetry", "step": "step",
}

func operationsDispatchSelector(expr ast.Expr) bool {
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name == "operation"
	}
	indexed, ok := expr.(*ast.IndexExpr)
	if !ok {
		return false
	}
	owner, ok := indexed.X.(*ast.Ident)
	if !ok || owner.Name != "args" {
		return false
	}
	index, ok := indexed.Index.(*ast.BasicLit)
	return ok && index.Kind == token.INT && index.Value == "0"
}

func operationsDispatchInventory() (map[string]map[string]bool, error) {
	result := map[string]map[string]bool{}
	files, err := filepath.Glob("*.go")
	if err != nil {
		return nil, err
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			return nil, err
		}
		for _, declaration := range f.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if _, ok := operationsDispatchGroups[fn.Name.Name]; !ok {
				continue
			}
			add := func(expr ast.Expr) {
				literal, ok := expr.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return
				}
				value, _ := strconv.Unquote(literal.Value)
				if value == "" || strings.Contains(value, " ") {
					return
				}
				if result[fn.Name.Name] == nil {
					result[fn.Name.Name] = map[string]bool{}
				}
				result[fn.Name.Name][value] = true
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.SwitchStmt:
					if node.Tag != nil && operationsDispatchSelector(node.Tag) {
						for _, statement := range node.Body.List {
							for _, expr := range statement.(*ast.CaseClause).List {
								add(expr)
							}
						}
					}
				case *ast.BinaryExpr:
					if node.Op == token.EQL || node.Op == token.NEQ {
						if operationsDispatchSelector(node.X) {
							add(node.Y)
						}
						if operationsDispatchSelector(node.Y) {
							add(node.X)
						}
					}
				}
				return true
			})
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("dispatch guard read nothing")
	}
	return result, nil
}

func operationsDispatchCoverage(page string, inventory map[string]map[string]bool) []string {
	documented := map[string]map[string]bool{}
	group := ""
	marker := regexp.MustCompile(`^<!-- cli-dispatch:([A-Za-z]+) -->$`)
	row := regexp.MustCompile("^\\| `([^`]+)` \\|$")
	for _, line := range strings.Split(page, "\n") {
		if match := marker.FindStringSubmatch(line); len(match) != 0 {
			group = match[1]
			documented[group] = map[string]bool{}
		}
		if match := row.FindStringSubmatch(line); len(match) != 0 && group != "" {
			name := match[1]
			prefix := operationsDispatchGroups[group]
			if prefix != "" {
				name = strings.TrimPrefix(name, prefix+" ")
			}
			documented[group][name] = true
		}
	}
	var failures []string
	for function, commands := range inventory {
		for command := range commands {
			if !documented[function][command] {
				failures = append(failures, function+": undocumented "+command)
			}
		}
	}
	for function, commands := range documented {
		for command := range commands {
			if !inventory[function][command] {
				failures = append(failures, function+": stale command "+command)
			}
		}
	}
	slices.Sort(failures)
	return failures
}

func TestOperationsDispatchCoverageIncludesCommandsOmittedFromShortHelp(t *testing.T) {
	t.Parallel()
	page, err := examples.Handbook.ReadFile("operations/commands.md")
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := operationsDispatchInventory()
	if err != nil {
		t.Fatal(err)
	}
	if failures := operationsDispatchCoverage(string(page), inventory); len(failures) != 0 {
		t.Fatalf("command coverage: %v", failures)
	}
	damaged := strings.Replace(string(page), "| `project queue add` |", "| absent entry |", 1)
	if failures := operationsDispatchCoverage(damaged, inventory); len(failures) == 0 {
		t.Fatal("dispatch guard accepted a missing live queue operation")
	}
}
