package tenon_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoBinaryFloatsByType keeps binary floating point out of number
// semantics by type: it type-checks the non-test files of the root package,
// internal/decimal, internal/cbor and internal/uni, and fails on any
// expression or declaration whose type holds a float or a complex number,
// however it is spelled. The lexical scan in internal/decimal stays beside
// it and keeps covering that package's tests; this one sees what spelling
// hides, such as a float reached through a math function or an identifier
// the lexical list does not name. gotenon is the declared edge where Go
// floats cross, and is exempt.
func TestNoBinaryFloatsByType(t *testing.T) {
	packages := []struct{ path, dir string }{
		{"github.com/kmoneil/tenon", "."},
		{"github.com/kmoneil/tenon/internal/decimal", "internal/decimal"},
		{"github.com/kmoneil/tenon/internal/cbor", "internal/cbor"},
		{"github.com/kmoneil/tenon/internal/uni", "internal/uni"},
	}
	fset := token.NewFileSet()
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	scanned := 0
	for _, p := range packages {
		names, err := filepath.Glob(filepath.Join(p.dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		var files []*ast.File
		for _, name := range names {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, name, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, f)
			scanned++
		}
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
		if _, err := conf.Check(p.path, fset, files, info); err != nil {
			t.Fatalf("type-checking %s: %v", p.path, err)
		}
		for expr, tv := range info.Types {
			if holdsFloat(tv.Type, map[types.Type]bool{}) {
				t.Errorf("%s: type %s", fset.Position(expr.Pos()), tv.Type)
			}
		}
	}
	// A glob that stopped matching would pass as cleanly.
	if scanned < 40 {
		t.Errorf("only %d files were scanned; the packages hold more", scanned)
	}
}

// holdsFloat reports whether a value of this type holds a float or complex
// number anywhere within it, or a function of it takes or returns one. A
// named type is looked through to what it is made of, not to its methods:
// a method set is not what a value holds, and big.Int would otherwise be a
// float for the sake of Float64.
func holdsFloat(t types.Type, seen map[types.Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	switch u := t.(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Float32, types.Float64, types.Complex64, types.Complex128,
			types.UntypedFloat, types.UntypedComplex:
			return true
		}
	case *types.Pointer:
		return holdsFloat(u.Elem(), seen)
	case *types.Slice:
		return holdsFloat(u.Elem(), seen)
	case *types.Array:
		return holdsFloat(u.Elem(), seen)
	case *types.Chan:
		return holdsFloat(u.Elem(), seen)
	case *types.Map:
		return holdsFloat(u.Key(), seen) || holdsFloat(u.Elem(), seen)
	case *types.Named:
		return holdsFloat(u.Underlying(), seen)
	case *types.Alias:
		return holdsFloat(types.Unalias(u), seen)
	case *types.Struct:
		for i := range u.NumFields() {
			if holdsFloat(u.Field(i).Type(), seen) {
				return true
			}
		}
	case *types.Signature:
		return holdsFloat(u.Params(), seen) || holdsFloat(u.Results(), seen)
	case *types.Tuple:
		for i := range u.Len() {
			if holdsFloat(u.At(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}
