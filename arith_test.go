package tenon_test

import (
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

func TestArithmeticAgreesWithTheDecimalItWraps(t *testing.T) {
	for _, tt := range []struct {
		name       string
		op         func(a, b tenon.Value) tenon.Value
		a, b, want string
	}{
		{"Add", tenon.Add, "1.5", "2.25", "3.75"},
		{"Add across zero", tenon.Add, "-1.5", "1.5", "0"},
		{"Sub", tenon.Sub, "1.5", "2.25", "-0.75"},
		{"Mul", tenon.Mul, "1.5", "2.25", "3.375"},
		{"Div exact", tenon.Div, "3", "4", "0.75"},
		{"Div rounded", tenon.Div, "1", "3", "0.333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333333"},
		{"Mod", tenon.Mod, "7", "3", "1"},
		{"Mod of a negative", tenon.Mod, "-7", "3", "-1"},
	} {
		got := tt.op(tenon.NumberFromText(tt.a), tenon.NumberFromText(tt.b))
		if !got.IsKnown() || got.Type() != tenon.NumberType() {
			t.Errorf("%s: %s and %s gave %v, want a known number", tt.name, tt.a, tt.b, got)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("%s: %s and %s = %s, want %s", tt.name, tt.a, tt.b, got, tt.want)
		}
	}
}

func TestArithmeticThatHasNoAnswer(t *testing.T) {
	one, zero := tenon.NumberFromInt(1), tenon.NumberFromInt(0)
	huge := tenon.NumberFromText("1e999999")
	wide := tenon.Add(one, tenon.NumberFromText("1e-999999"))
	if !wide.IsKnown() {
		t.Fatalf("1 + 1e-999999 gave %v, want a known number", wide)
	}
	for _, tt := range []struct {
		name string
		got  tenon.Value
		code tenon.Code
	}{
		{"divide by zero", tenon.Div(one, zero), tenon.CodeNumberDivideByZero},
		{"modulo by zero", tenon.Mod(one, zero), tenon.CodeNumberModuloByZero},
		{"out of range", tenon.Mul(huge, huge), tenon.CodeNumberOutOfRange},
		// A number whose digits span the whole window squares to one whose
		// digits cannot fit it, so repeated multiplication cannot grow a
		// number's digits without bound.
		{"a wide number squared", tenon.Mul(wide, wide), tenon.CodeNumberOutOfRange},
		{"a digit below the window", tenon.Mul(tenon.NumberFromText("1e-999999"), tenon.NumberFromText("1.5")), tenon.CodeNumberOutOfRange},
		{"a null operand", tenon.Add(tenon.NullVal(tenon.NumberType()), one), tenon.CodeOperationNullOperand},
	} {
		if !tt.got.IsError() {
			t.Errorf("%s gave %v, want an error value", tt.name, tt.got)
			continue
		}
		if code := tt.got.Diagnostics()[0].Code; code != tt.code {
			t.Errorf("%s gave code %s, want %s", tt.name, code, tt.code)
		}
	}
	// An operand of another type is the calling program's mistake.
	mustPanicUsage(t, "Add: the second operand is a value of type string, which does not satisfy exactly(number)", func() {
		tenon.Add(one, tenon.String("1"))
	})
}

func TestConformance_UN007_ArithmeticBoundsWhatItCan(t *testing.T) {
	conformance.Covers(t, "UN-007")
	num := tenon.NumberType()
	zero, one, two, ten := tenon.NumberFromInt(0), tenon.NumberFromInt(1), tenon.NumberFromInt(2), tenon.NumberFromInt(10)
	// A number between 1 and 10, and one between 0 and 2.
	x := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(one, true), tenon.NumberMax(ten, true))
	y := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(zero, true), tenon.NumberMax(two, true))
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		{"a sum of bounds", tenon.Add(x, two), "unknown(number, not null, >= 3, <= 12)"},
		{"a sum of two ranges", tenon.Add(x, y), "unknown(number, not null, >= 1, <= 12)"},
		{"a difference turns the bounds around", tenon.Sub(x, y), "unknown(number, not null, >= -1, <= 10)"},
		{"the bounds of the subtrahend", tenon.Sub(two, x), "unknown(number, not null, >= -8, <= 1)"},
		// An operand that says nothing leaves the result saying nothing, and
		// the rule allows a result to say less than it might.
		{"an operand with no bounds", tenon.Add(x, tenon.Unknown(num)), "unknown(number, not null)"},
		{"a pending operand", tenon.Add(x, tenon.Pending(tenon.Exactly(num))), "unknown(number, not null)"},
		// A product is least and greatest at the products of the bounds.
		{"a product of bounds", tenon.Mul(x, two), "unknown(number, not null, >= 2, <= 20)"},
		{"a product of two ranges", tenon.Mul(x, y), "unknown(number, not null, >= 0, <= 20)"},
		{"a factor that is negative", tenon.Mul(x, tenon.NumberFromInt(-2)), "unknown(number, not null, >= -20, <= -2)"},
		{"a factor that spans zero", tenon.Mul(x, tenon.Sub(y, one)), "unknown(number, not null, >= -10, <= 10)"},
		{"a factor that is unbounded above", tenon.Mul(x, tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(two, true))), "unknown(number, not null, >= 2)"},
		// A factor of zero makes every product zero, however little is known
		// of the other, and a range of one value is that value.
		{"a factor of zero", tenon.Mul(zero, tenon.Unknown(num)), "0"},
		// A quotient is bounded where the divisor keeps away from zero.
		{"a quotient of bounds", tenon.Div(x, two), "unknown(number, not null, >= 0.5, <= 5)"},
		{"a divisor that is a range", tenon.Div(x, tenon.Add(y, two)), "unknown(number, not null, >= 0.25, <= 5)"},
		{"a divisor that is negative", tenon.Div(x, tenon.NumberFromInt(-4)), "unknown(number, not null, >= -2.5, <= -0.25)"},
		{"a divisor that may be zero", tenon.Div(x, y), "unknown(number, not null)"},
		{"a divisor that may come near zero", tenon.Div(x, tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(zero, false))), "unknown(number, not null)"},
		// A remainder has the sign of the dividend and is smaller in magnitude
		// than the divisor can be.
		{"a remainder", tenon.Mod(x, tenon.NumberFromInt(3)), "unknown(number, not null, >= 0, < 3)"},
		{"a remainder of a negative dividend", tenon.Mod(tenon.Sub(zero, x), tenon.NumberFromInt(3)), "unknown(number, not null, > -3, <= 0)"},
		{"a remainder no greater than its dividend", tenon.Mod(y, tenon.NumberFromInt(7)), "unknown(number, not null, >= 0, <= 2)"},
		{"a divisor that is a range", tenon.Mod(x, tenon.Add(y, one)), "unknown(number, not null, >= 0, < 3)"},
	} {
		if got := tt.got.String(); got != tt.want {
			t.Errorf("%s = %s, want %s", tt.name, got, tt.want)
		}
	}
	// A bound that is exclusive stays exclusive, since no sum reaches it.
	above := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(one, false))
	if got, want := tenon.Add(above, two).String(), "unknown(number, not null, > 3)"; got != want {
		t.Errorf("a sum of an exclusive bound = %s, want %s", got, want)
	}
	// And a product's, where no factor that reaches its bound is zero.
	if got, want := tenon.Mul(above, two).String(), "unknown(number, not null, > 2)"; got != want {
		t.Errorf("a product of an exclusive bound = %s, want %s", got, want)
	}
	// A quotient's includes its own value, since a quotient is rounded and a
	// value past the bound may round onto it.
	if got, want := tenon.Div(above, two).String(), "unknown(number, not null, >= 0.5)"; got != want {
		t.Errorf("a quotient of an exclusive bound = %s, want %s", got, want)
	}
	// The bounds hold every outcome: a value drawn from the range, added, is
	// in the result's range.
	sum := tenon.Add(x, two)
	for _, text := range []string{"1", "5.5", "10"} {
		v := tenon.NumberFromText(text)
		if got := tenon.Narrow(sum, tenon.NumberMin(tenon.Add(v, two), true), tenon.NumberMax(tenon.Add(v, two), true)); got.IsError() {
			t.Errorf("%s plus 2 is outside the bounds of the sum: %v", text, got)
		}
	}
}
