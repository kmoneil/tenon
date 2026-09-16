package tenon_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestReflectionStaysInTheInteropPackage scans the module's Go source, tests
// aside, for imports. Only gotenon reflects on Go values, and it does so from
// outside, through tenon's exported API alone.
func TestReflectionStaysInTheInteropPackage(t *testing.T) {
	fset := token.NewFileSet()
	interop := 0
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		inInterop := filepath.Dir(path) == "gotenon"
		if inInterop {
			interop++
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			switch {
			case imported == "reflect" && !inInterop:
				t.Errorf("%s imports reflect, which only gotenon may", path)
			case strings.HasPrefix(imported, "tenon/internal/") && inInterop:
				t.Errorf("%s imports %s; gotenon uses tenon's exported API alone", path, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if interop == 0 {
		t.Error("found no files in gotenon; the walk does not see the module")
	}
}
