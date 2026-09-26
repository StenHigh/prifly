package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// packageIdentity is the shape of a workflow, package or step identifier. The
// engine's own live in the core namespace; any other one in engine code is a
// mechanism that works for one package only.
var packageIdentity = regexp.MustCompile(`^([a-z][a-z0-9-]*):(workflow|package|step)/`)

// productMarkers are names engine code once carried for one product. They are
// written here, in a test, because naming the offender is how the guard proves
// it can see it; the engine itself must not know them.
var productMarkers = []string{"aif:", "aif-", "ai factory"}

// The engine executes and the workflow directs. project continue and project
// recover once compared workflow IDs, looked stages up by name and read commit
// fields out of an artifact, so they worked for one package and no other. This
// reads every string literal of the engine's non-test code and refuses any that
// names a workflow, package or step outside the core namespace.
func TestEngineNamesNoWorkflowOfItsOwn(t *testing.T) {
	files, literals := 0, 0
	var found []string
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			files++
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					return true
				}
				literals++
				if match := packageIdentity.FindStringSubmatch(value); match != nil && match[1] != "core" {
					found = append(found, path+": "+value)
				}
				lower := strings.ToLower(value)
				for _, marker := range productMarkers {
					if strings.Contains(lower, marker) {
						found = append(found, path+": "+value)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// A walk that read nothing reports the same green as a clean one.
	if files < 100 || literals < 1000 {
		t.Fatalf("read %d files and %d string literals; the walk did not reach the engine", files, literals)
	}
	t.Logf("read %d files, %d string literals", files, literals)
	if len(found) != 0 {
		t.Fatalf("engine code names a workflow, package or step it does not own:\n%s", strings.Join(found, "\n"))
	}
}
