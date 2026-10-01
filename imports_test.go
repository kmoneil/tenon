package tenon_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestReflectionStaysInTheInteropPackage scans the module's Go source, tests
// aside, for imports. Only gotenon reflects on Go values, and it does so from
// outside, through tenon's exported API alone, as ctytenon, the bridge to
// go-cty, does too.
func TestReflectionStaysInTheInteropPackage(t *testing.T) {
	fset := token.NewFileSet()
	interop, bridge := 0, 0
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
		inBridge := filepath.Dir(path) == "ctytenon"
		if inInterop {
			interop++
		}
		if inBridge {
			bridge++
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			switch {
			case imported == "reflect" && !inInterop:
				t.Errorf("%s imports reflect, which only gotenon may", path)
			case strings.HasPrefix(imported, "github.com/kmoneil/tenon/internal/") && (inInterop || inBridge):
				t.Errorf("%s imports %s; %s uses tenon's exported API alone", path, imported, filepath.Dir(path))
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
	if bridge == 0 {
		t.Error("found no files in ctytenon; the walk does not see the bridge")
	}
}

// TestTenonRequiresNoModule holds tenon's go.mod to requiring no module at
// all. go-cty above all stays out: the bench module and ctytenon require it in
// go.mod files of their own, so a program that imports tenon alone never
// downloads it.
func TestTenonRequiresNoModule(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == "require" {
			t.Errorf("go.mod:%d requires a module: %s", i+1, line)
		}
	}
	if strings.Contains(string(data), "zclconf/go-cty") {
		t.Error("go.mod names go-cty")
	}
}

// TestOneGoCty holds the bench module, whose tests assert go-cty's behavior
// in its open issues, and ctytenon, which bridges to go-cty, to requiring one
// version of it, so that what the probes say of go-cty is what the bridge
// meets.
func TestOneGoCty(t *testing.T) {
	version := func(gomod string) string {
		data, err := os.ReadFile(gomod)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "github.com/zclconf/go-cty" {
				return fields[1]
			}
		}
		t.Fatalf("%s requires no go-cty", gomod)
		return ""
	}
	if bench, bridge := version(filepath.Join("bench", "go.mod")), version(filepath.Join("ctytenon", "go.mod")); bench != bridge {
		t.Errorf("the bench module requires go-cty %s, and ctytenon %s", bench, bridge)
	}
}
