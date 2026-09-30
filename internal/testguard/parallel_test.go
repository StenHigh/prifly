// Package testguard holds checks over the repository's own tests.
package testguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A parallel test shares its process with every other one. Go refuses
// t.Setenv in it by itself; nothing refuses a test that writes a package
// variable or reads the delta of a package-wide counter, and such a test is
// green alone and wrong beside a neighbour. This guard names each one.
func TestParallelTestsTouchNoProcessWideState(t *testing.T) {
	root := filepath.Join("..", "..")
	files, parallel := 0, 0
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
				return err
			}
			files++
			parallel += checkFile(t, path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// A guard that read nothing reports the same green as one that found nothing.
	if files < 100 || parallel < 500 {
		t.Fatalf("read %d test files and %d parallel tests; the walk no longer reaches the tests it guards", files, parallel)
	}
	t.Logf("%d test files read, %d parallel tests checked", files, parallel)
}

func checkFile(t *testing.T, path string) int {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	globals := packageVars(t, filepath.Dir(path))
	writes := ""
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				return true
			}
			for _, lhs := range x.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Obj == nil && globals[id.Name] {
					writes = "writes package variable " + id.Name
				}
			}
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && id.Name == "os" && (x.Sel.Name == "Setenv" || x.Sel.Name == "Unsetenv" || x.Sel.Name == "Chdir") {
				// TestMain may set the process environment once before any test runs.
				writes = "calls os." + x.Sel.Name
			}
		}
		return true
	})
	parallel := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "TestMain" || !startsParallel(fn) {
			continue
		}
		parallel++
		if writes != "" && !onlyInTestMain(file, writes) {
			t.Errorf("%s: %s is parallel, but its file %s", fset.Position(fn.Pos()), fn.Name.Name, writes)
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Load" {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Obj == nil && globals[id.Name] {
				t.Errorf("%s: %s is parallel, but reads the package-wide counter %s", fset.Position(call.Pos()), fn.Name.Name, id.Name)
			}
			return true
		})
	}
	return parallel
}

func startsParallel(fn *ast.FuncDecl) bool {
	if len(fn.Body.List) == 0 {
		return false
	}
	stmt, ok := fn.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := stmt.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Parallel"
}

// onlyInTestMain reports whether every process-wide write of the file sits in
// TestMain, which runs before any test starts.
func onlyInTestMain(file *ast.File, _ string) bool {
	outside := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name.Name == "TestMain" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && (sel.Sel.Name == "Setenv" || sel.Sel.Name == "Unsetenv" || sel.Sel.Name == "Chdir") {
					outside = true
				}
			}
			if assign, ok := n.(*ast.AssignStmt); ok && assign.Tok != token.DEFINE {
				for _, lhs := range assign.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && id.Obj == nil && id.Name != "_" {
						outside = true
					}
				}
			}
			return true
		})
	}
	return !outside
}

var varCache = map[string]map[string]bool{}

// packageVars lists the package-level variables of a directory's package,
// test files included.
func packageVars(t *testing.T, dir string) map[string]bool {
	t.Helper()
	if vars, ok := varCache[dir]; ok {
		return vars
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	vars := map[string]bool{}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				for _, spec := range gen.Specs {
					for _, name := range spec.(*ast.ValueSpec).Names {
						if name.Name != "_" {
							vars[name.Name] = true
						}
					}
				}
			}
		}
	}
	varCache[dir] = vars
	return vars
}
