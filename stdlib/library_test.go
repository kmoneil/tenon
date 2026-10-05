package stdlib_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// library is every function the package exports, by the name the
// specification gives it. TestLibraryIsWhole holds the package to it, so a
// function added without its entry here fails the run.
var library = map[string]tenon.Function{
	"AssertNotNull":          stdlib.AssertNotNullFunc,
	"Merge":                  stdlib.MergeFunc,
	"Upper":                  stdlib.UpperFunc,
	"Lower":                  stdlib.LowerFunc,
	"Title":                  stdlib.TitleFunc,
	"Strlen":                 stdlib.StrlenFunc,
	"Reverse":                stdlib.ReverseFunc,
	"Substr":                 stdlib.SubstrFunc,
	"Split":                  stdlib.SplitFunc,
	"Replace":                stdlib.ReplaceFunc,
	"TrimPrefix":             stdlib.TrimPrefixFunc,
	"TrimSuffix":             stdlib.TrimSuffixFunc,
	"Range":                  stdlib.RangeFunc,
	"SetProduct":             stdlib.SetProductFunc,
	"Contains":               stdlib.ContainsFunc,
	"SetHasElement":          stdlib.SetHasElementFunc,
	"SetUnion":               stdlib.SetUnionFunc,
	"SetIntersection":        stdlib.SetIntersectionFunc,
	"SetSubtract":            stdlib.SetSubtractFunc,
	"SetSymmetricDifference": stdlib.SetSymmetricDifferenceFunc,
	"Keys":                   stdlib.KeysFunc,
	"Values":                 stdlib.ValuesFunc,
	"Zipmap":                 stdlib.ZipmapFunc,
	"Lookup":                 stdlib.LookupFunc,
	"Flatten":                stdlib.FlattenFunc,
	"Compact":                stdlib.CompactFunc,
	"Distinct":               stdlib.DistinctFunc,
	"CoalesceList":           stdlib.CoalesceListFunc,
	"Slice":                  stdlib.SliceFunc,
	"ReverseList":            stdlib.ReverseListFunc,
	"Concat":                 stdlib.ConcatFunc,
	"Chunklist":              stdlib.ChunklistFunc,
	"Length":                 stdlib.LengthFunc,
	"HasIndex":               stdlib.HasIndexFunc,
	"Index":                  stdlib.IndexFunc,
	"Element":                stdlib.ElementFunc,
	"Log":                    stdlib.LogFunc,
	"Pow":                    stdlib.PowFunc,
	"Absolute":               stdlib.AbsoluteFunc,
	"Signum":                 stdlib.SignumFunc,
	"Int":                    stdlib.IntFunc,
	"Ceil":                   stdlib.CeilFunc,
	"Floor":                  stdlib.FloorFunc,
	"Min":                    stdlib.MinFunc,
	"Max":                    stdlib.MaxFunc,
	"ParseInt":               stdlib.ParseIntFunc,
	"Coalesce":               stdlib.CoalesceFunc,
	"MakeTo":                 stdlib.MakeToFunc(tenon.Exactly(tenon.NumberType())),
	"Add":                    stdlib.AddFunc,
	"Subtract":               stdlib.SubtractFunc,
	"Multiply":               stdlib.MultiplyFunc,
	"Divide":                 stdlib.DivideFunc,
	"Modulo":                 stdlib.ModuloFunc,
	"Negate":                 stdlib.NegateFunc,
	"LessThan":               stdlib.LessThanFunc,
	"LessThanOrEqualTo":      stdlib.LessThanOrEqualToFunc,
	"GreaterThan":            stdlib.GreaterThanFunc,
	"GreaterThanOrEqualTo":   stdlib.GreaterThanOrEqualToFunc,
	"Equal":                  stdlib.EqualFunc,
	"NotEqual":               stdlib.NotEqualFunc,
	"Not":                    stdlib.NotFunc,
	"And":                    stdlib.AndFunc,
	"Or":                     stdlib.OrFunc,
}

// TestLibraryIsWhole holds library to the package: every exported variable
// named for a function is in it, and nothing else is.
func TestLibraryIsWhole(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var declared []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				for _, id := range spec.(*ast.ValueSpec).Names {
					if id.IsExported() {
						declared = append(declared, id.Name)
					}
				}
			}
		}
	}
	var listed []string
	for name := range library {
		if name != "MakeTo" { // a factory, declared as a function
			listed = append(listed, name+"Func")
		}
	}
	slices.Sort(declared)
	slices.Sort(listed)
	if !slices.Equal(declared, listed) {
		t.Errorf("the package declares %v, and library lists %v", declared, listed)
	}
}

func TestConformance_LB001_FunctionsAreNamed(t *testing.T) {
	conformance.Covers(t, "LB-001")
	for name, f := range library {
		if f.Name() != name {
			t.Errorf("%sFunc is named %q, want %q, go-cty's name without the suffix", name, f.Name(), name)
		}
		if f.Description() == "" {
			t.Errorf("%s has no description", name)
		}
		for i, p := range f.Params() {
			if p.Name == "" || p.Description == "" {
				t.Errorf("%s: parameter %d is not named and described", name, i+1)
			}
		}
		if vp := f.VarParam(); vp != nil && (vp.Name == "" || vp.Description == "") {
			t.Errorf("%s: the variadic parameter is not named and described", name)
		}
	}
}

func TestConformance_LB002_NothingIsVolatile(t *testing.T) {
	conformance.Covers(t, "LB-002")
	for name, f := range library {
		if f.Volatile() {
			t.Errorf("%s declares volatility", name)
		}
	}
}
