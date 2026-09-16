package tenon_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"tenon/conformance"
)

// TestConformance_ER001_OnlyTheHelperPanics scans the module's Go source,
// tests included, for calls to panic. Every usage error goes through the
// helper in usage.go, beside the one for an internal defect, which keeps the
// panic surface in one file.
func TestConformance_ER001_OnlyTheHelperPanics(t *testing.T) {
	conformance.Covers(t, "ER-001")
	fset := token.NewFileSet()
	scanned := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != "." && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || path == "usage.go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		scanned++
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "panic" {
					t.Errorf("%s: panic called outside the usage-error helper", fset.Position(call.Pos()))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 20 {
		t.Errorf("scanned only %d Go files; the walk does not see the module", scanned)
	}
}
