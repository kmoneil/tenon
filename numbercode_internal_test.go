package tenon

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/kmoneil/tenon/internal/decimal"
)

// TestDecimalErrorsMapToOneCode enumerates the error constants
// internal/decimal declares, from its source, and holds numberCode to an arm
// for each: a constant the package gains without an arm fails here, where
// the call sites once fell to whichever code their default named. The sites
// are then held to their codes through the public surface.
func TestDecimalErrorsMapToOneCode(t *testing.T) {
	constants := decimalErrorConstants(t)
	if len(constants) < 4 {
		t.Fatalf("found %d error constants in internal/decimal, which declares more", len(constants))
	}
	want := map[string]Code{
		"ErrSyntax":       CodeNumberInvalidSyntax,
		"ErrOutOfRange":   CodeNumberOutOfRange,
		"ErrDivideByZero": CodeNumberDivideByZero,
		"ErrModuloByZero": CodeNumberModuloByZero,
		"ErrTooLong":      CodeNumberTooLong,
	}
	for i, name := range constants {
		err := decimal.Error(i + 1) // the constants count from one, in order
		code, ok := want[name]
		if !ok {
			t.Errorf("internal/decimal declares %s, which this test does not know; map it in numberCode and here", name)
			continue
		}
		got := func() (c Code) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("numberCode(%s) panicked: %v", name, r)
				}
			}()
			return numberCode(err)
		}()
		if got != code {
			t.Errorf("numberCode(%s) = %s, want %s", name, got, code)
		}
	}
	mustPanicInternal(t, "no diagnostic code maps", func() {
		numberCode(decimal.Error(len(constants) + 1))
	})

	// Each site reports the code the mapping gives, through the public
	// surface: text parsed directly, text parsed by conversion, and
	// arithmetic.
	long := strings.Repeat("9", 10_001)
	for _, tt := range []struct {
		name string
		v    Value
		code Code
	}{
		{"NumberFromText of no number", NumberFromText("x"), CodeNumberInvalidSyntax},
		{"NumberFromText out of range", NumberFromText("1e1000000"), CodeNumberOutOfRange},
		{"NumberFromText too long", NumberFromText(long), CodeNumberTooLong},
		{"Convert of no number", Convert(String("x"), Exactly(Type{numberType}), Unsafe), CodeNumberInvalidSyntax},
		{"Convert out of range", Convert(String("1e1000000"), Exactly(Type{numberType}), Unsafe), CodeNumberOutOfRange},
		{"Convert too long", Convert(String(long), Exactly(Type{numberType}), Unsafe), CodeNumberTooLong},
		{"Div by zero", Div(NumberFromInt(1), NumberFromInt(0)), CodeNumberDivideByZero},
		{"Mod by zero", Mod(NumberFromInt(1), NumberFromInt(0)), CodeNumberModuloByZero},
		{"Mul out of range", Mul(NumberFromText("1e999999"), NumberFromText("1e999999")), CodeNumberOutOfRange},
	} {
		if !tt.v.IsError() {
			t.Errorf("%s gave %v, want an error value", tt.name, tt.v)
			continue
		}
		if d := tt.v.Diagnostics(); len(d) != 1 || d[0].Code != tt.code {
			t.Errorf("%s has diagnostics %v, want the code %s", tt.name, d, tt.code)
		}
	}
}

// decimalErrorConstants returns the names of internal/decimal's Error
// constants, in declaration order, read from the source so that a constant
// added without a switch arm anywhere is still seen.
func decimalErrorConstants(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "internal/decimal/dec.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for _, name := range vs.Names {
				if strings.HasPrefix(name.Name, "Err") {
					names = append(names, name.Name)
				}
			}
		}
	}
	return names
}
