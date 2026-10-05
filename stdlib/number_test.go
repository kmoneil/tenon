package stdlib_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// at returns the path of argument i.
func at(i int) tenon.Path { return tenon.Path{}.Index(tenon.NumberFromInt(int64(i))) }

// operands are numbers an operator is called with: known ones, spelled more
// than one way, unknown ones narrowed by a range, and a marked one.
func operands() []tenon.Value {
	num := tenon.NumberType()
	return []tenon.Value{
		tenon.NumberFromInt(7),
		tenon.NumberFromText("-2.50"),
		tenon.NumberFromText("1e30"),
		tenon.NumberFromInt(0),
		tenon.NumberFromText("0.1"),
		tenon.Unknown(num),
		tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(tenon.NumberFromInt(1), true), tenon.NumberMax(tenon.NumberFromInt(5), false)),
		tenon.WithMarks(tenon.NumberFromInt(3), secret{}),
	}
}

// sameAs holds f, applied to every pair of operands, to the answer of op.
func sameAs(t *testing.T, name string, f tenon.Function, op func(a, b tenon.Value) tenon.Value) {
	t.Helper()
	for _, a := range operands() {
		for _, b := range operands() {
			got, want := call(f, a, b), op(a, b)
			if !tenon.Identical(got, want) {
				t.Errorf("%s(%v, %v) = %v, want %v", name, a, b, got, want)
			}
		}
	}
}

func TestConformance_LN001_Arithmetic(t *testing.T) {
	conformance.Covers(t, "LN-001")
	for name, op := range map[string]func(a, b tenon.Value) tenon.Value{
		"Add": tenon.Add, "Subtract": tenon.Sub, "Multiply": tenon.Mul, "Divide": tenon.Div, "Modulo": tenon.Mod,
	} {
		f := library[name]
		sameAs(t, name, f, op)
		// A null operand fails at the call, located at it.
		for i := range 2 {
			args := []tenon.Value{tenon.NumberFromInt(1), tenon.NumberFromInt(1)}
			args[i] = tenon.Null(tenon.NumberType())
			got := call(f, args...)
			if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeOperationNullOperand || !got.Diagnostics()[0].Path.Equal(at(i)) {
				t.Errorf("%s with a null operand %d gave %v, want %s at %s", name, i, got, tenon.CodeOperationNullOperand, at(i))
			}
		}
	}
	// What go-cty answers in binary floating point is exact here, and a
	// division by zero fails where go-cty answers an infinity.
	if got := call(stdlib.AddFunc, tenon.NumberFromText("0.1"), tenon.NumberFromText("0.2")); !got.Equal(tenon.NumberFromText("0.3")) {
		t.Errorf("Add(0.1, 0.2) = %v, want 0.3", got)
	}
	if got := call(stdlib.DivideFunc, tenon.NumberFromInt(1), tenon.NumberFromInt(0)); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeNumberDivideByZero {
		t.Errorf("Divide(1, 0) = %v, want %s", got, tenon.CodeNumberDivideByZero)
	}
	// A string of digits is a number under the unsafe policy.
	if got := tenon.Call(stdlib.AddFunc, []tenon.Value{tenon.String("2"), tenon.NumberFromInt(3)}, tenon.Unsafe); !got.Equal(tenon.NumberFromInt(5)) {
		t.Errorf("Add(\"2\", 3) under Unsafe = %v, want 5", got)
	}
}

func TestConformance_LN002_Negate(t *testing.T) {
	conformance.Covers(t, "LN-002")
	for _, a := range operands() {
		if got, want := call(stdlib.NegateFunc, a), tenon.Sub(tenon.NumberFromInt(0), a); !tenon.Identical(got, want) {
			t.Errorf("Negate(%v) = %v, want %v", a, got, want)
		}
	}
	if got := call(stdlib.NegateFunc, tenon.NumberFromText("-2.5")); !got.Equal(tenon.NumberFromText("2.5")) {
		t.Errorf("Negate(-2.5) = %v", got)
	}
}

func TestConformance_LN010_Orderings(t *testing.T) {
	conformance.Covers(t, "LN-010")
	sameAs(t, "LessThan", stdlib.LessThanFunc, tenon.LessThan)
	sameAs(t, "GreaterThan", stdlib.GreaterThanFunc, func(a, b tenon.Value) tenon.Value { return tenon.LessThan(b, a) })
	sameAs(t, "LessThanOrEqualTo", stdlib.LessThanOrEqualToFunc, func(a, b tenon.Value) tenon.Value { return tenon.Not(tenon.LessThan(b, a)) })
	sameAs(t, "GreaterThanOrEqualTo", stdlib.GreaterThanOrEqualToFunc, func(a, b tenon.Value) tenon.Value { return tenon.Not(tenon.LessThan(a, b)) })
	// Strings are compared as the numbers they convert to, as a language
	// converts its operands: "10" is greater than "9".
	if got := tenon.Call(stdlib.GreaterThanFunc, []tenon.Value{tenon.String("10"), tenon.String("9")}, tenon.Unsafe); !got.Equal(tenon.Bool(true)) {
		t.Errorf("GreaterThan(\"10\", \"9\") under Unsafe = %v, want true", got)
	}
	if got := call(stdlib.GreaterThanFunc, tenon.String("10"), tenon.String("9")); !got.IsError() {
		t.Errorf("GreaterThan(\"10\", \"9\") under Safe = %v, want the conversion's failure", got)
	}
}

func TestConformance_LB011_KnownFailuresFailNow(t *testing.T) {
	conformance.Covers(t, "LB-011")
	// A divisor of zero fails every outcome, whatever the dividend turns
	// out to be, so the call fails now.
	unknown := tenon.Unknown(tenon.NumberType())
	for name, code := range map[string]tenon.Code{"Divide": tenon.CodeNumberDivideByZero, "Modulo": tenon.CodeNumberModuloByZero} {
		got := call(library[name], unknown, tenon.NumberFromInt(0))
		if !got.IsError() || got.Diagnostics()[0].Code != code {
			t.Errorf("%s(unknown, 0) = %v, want %s now", name, got, code)
		}
	}
}
