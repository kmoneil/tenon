package stdlib

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon"
)

// number is the constraint of a Number parameter or result.
var number = tenon.Exactly(tenon.NumberType())

// operand returns a parameter taking a number that admits what an operation
// answers itself: values not known yet, whose ranges the operation narrows
// its answer by, and marks, which it carries as it carries them. A null is
// refused at the call, located at the argument.
func operand(name, description string, c tenon.Constraint) tenon.Param {
	return tenon.Param{Name: name, Description: description, Constraint: c, AllowUnknown: true, AllowMarked: true}
}

// binary returns a function of two operands of constraint c whose answer is
// op's, of result constraint result, never null.
func binary(name, description string, c, result tenon.Constraint, op func(a, b tenon.Value) tenon.Value) tenon.Function {
	return tenon.NewFunction(tenon.FunctionSpec{
		Name:        name,
		Description: description,
		Params:      []tenon.Param{operand("a", "The first operand.", c), operand("b", "The second operand.", c)},
		Result:      result,
		NotNull:     true,
		Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
			return op(args[0], args[1]), nil
		},
	})
}

// AddFunc is the sum of two numbers, exact: tenon's Add.
var AddFunc = binary("Add", "Returns the sum of the two given numbers.", number, number, tenon.Add)

// SubtractFunc is the difference of two numbers, exact: tenon's Sub.
var SubtractFunc = binary("Subtract", "Returns the difference between the two given numbers.", number, number, tenon.Sub)

// MultiplyFunc is the product of two numbers, exact: tenon's Mul.
var MultiplyFunc = binary("Multiply", "Returns the product of the two given numbers.", number, number, tenon.Mul)

// DivideFunc is the quotient of two numbers, exact where it terminates and
// rounded to 96 significant digits where it does not: tenon's Div. Division
// by zero fails with tenon.CodeNumberDivideByZero, where go-cty answers an
// infinity.
var DivideFunc = binary("Divide", "Divides the first given number by the second.", number, number, tenon.Div)

// ModuloFunc is the remainder of dividing two numbers, exact, with the sign
// of the first: tenon's Mod. A divisor of zero fails with
// tenon.CodeNumberModuloByZero.
var ModuloFunc = binary("Modulo", "Divides the first given number by the second and then returns the remainder.", number, number, tenon.Mod)

// zero is the number 0.
var zero = tenon.NumberFromInt(0)

// NegateFunc is the number negated: tenon's Sub of it from zero.
var NegateFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Negate",
	Description: "Multiplies the given number by -1.",
	Params:      []tenon.Param{operand("num", "The number to negate.", number)},
	Result:      number,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return tenon.Sub(zero, args[0]), nil
	},
})

// boolean is the constraint of a Bool parameter or result.
var boolean = tenon.Exactly(tenon.BoolType())

// LessThanFunc reports whether the first number is less than the second:
// tenon's LessThan, over numbers alone, as a language's operands convert to
// numbers before they are compared.
var LessThanFunc = binary("LessThan", "Returns true if and only if the second number is greater than the first.", number, boolean, tenon.LessThan)

// GreaterThanFunc reports whether the first number is greater than the
// second: the second less than the first.
var GreaterThanFunc = binary("GreaterThan", "Returns true if and only if the second number is less than the first.", number, boolean,
	func(a, b tenon.Value) tenon.Value { return tenon.LessThan(b, a) })

// LessThanOrEqualToFunc reports whether the first number is less than or
// equal to the second: the second not less than the first.
var LessThanOrEqualToFunc = binary("LessThanOrEqualTo", "Returns true if and only if the second number is greater than or equal to the first.", number, boolean,
	func(a, b tenon.Value) tenon.Value { return tenon.Not(tenon.LessThan(b, a)) })

// GreaterThanOrEqualToFunc reports whether the first number is greater than
// or equal to the second: the first not less than the second.
var GreaterThanOrEqualToFunc = binary("GreaterThanOrEqualTo", "Returns true if and only if the second number is less than or equal to the first.", number, boolean,
	func(a, b tenon.Value) tenon.Value { return tenon.Not(tenon.LessThan(a, b)) })

// one is the number 1.
var one = tenon.NumberFromInt(1)

// unary returns a function of one number, never null, whose answer for a
// known number is known and for one not known yet is narrowed to what its
// range gives.
func unary(name, description string, known func(x tenon.Value) tenon.Value, unknown func(lo, hi end) tenon.Value) tenon.Function {
	return tenon.NewFunction(tenon.FunctionSpec{
		Name:        name,
		Description: description,
		Params:      []tenon.Param{{Name: "num", Description: "The number.", Constraint: number, AllowUnknown: true}},
		Result:      number,
		NotNull:     true,
		Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
			x := args[0]
			if x.IsKnown() {
				return known(x), nil
			}
			lo, hi := ends(x)
			return unknown(lo, hi), nil
		},
	})
}

// end is one end of the range of a number: a bound, whether the bound is
// itself in the range, and whether there is one at all.
type end struct {
	v             tenon.Value
	inclusive, ok bool
}

// ends returns the ends of the range of the number x, known or not.
func ends(x tenon.Value) (lo, hi end) {
	r := x.Range()
	lo.v, lo.inclusive, lo.ok = r.NumberMin()
	hi.v, hi.inclusive, hi.ok = r.NumberMax()
	return lo, hi
}

// within returns the unknown number, not null, narrowed to the ends given.
func within(lo, hi end) tenon.Value {
	ns := []tenon.Narrowing{tenon.NotNull()}
	if lo.ok {
		ns = append(ns, tenon.NumberMin(lo.v, lo.inclusive))
	}
	if hi.ok {
		ns = append(ns, tenon.NumberMax(hi.v, hi.inclusive))
	}
	return tenon.Narrow(tenon.Unknown(tenon.NumberType()), ns...)
}

// less reports whether the known number a is less than the known number b.
func less(a, b tenon.Value) bool { return tenon.LessThan(a, b).AsBool() }

// negated returns the number x negated.
func negated(x tenon.Value) tenon.Value { return tenon.Sub(zero, x) }

// AbsoluteFunc is the magnitude of a number. A number not known yet answers
// with the range its own gives: what lies at or above zero, the negation of
// what lies at or below it, or from zero to the farther end where the range
// takes in both signs.
var AbsoluteFunc = unary("Absolute", "If the given number is negative then returns its positive equivalent, or otherwise returns the given number unchanged.",
	func(x tenon.Value) tenon.Value {
		if less(x, zero) {
			return negated(x)
		}
		return x
	},
	func(lo, hi end) tenon.Value {
		switch {
		case lo.ok && !less(lo.v, zero):
			return within(lo, hi)
		case hi.ok && !less(zero, hi.v):
			return within(end{negated(hi.v), hi.inclusive, true}, end{negated(lo.v), lo.inclusive, lo.ok})
		}
		top := end{}
		if lo.ok && hi.ok {
			top = hi
			switch {
			case less(hi.v, negated(lo.v)):
				top = end{negated(lo.v), lo.inclusive, true}
			case hi.v.Equal(negated(lo.v)):
				top.inclusive = hi.inclusive || lo.inclusive
			}
		}
		return within(end{zero, true, true}, top)
	})

// SignumFunc is -1, 0 or 1 as the number is negative, zero or positive. A
// number not known yet answers known where its range takes in one sign
// alone, and otherwise within the signs it does.
var SignumFunc = unary("Signum", "Returns 0 if the given number is zero, 1 if the given number is positive, or -1 if the given number is negative.",
	func(x tenon.Value) tenon.Value {
		switch {
		case less(x, zero):
			return tenon.NumberFromInt(-1)
		case less(zero, x):
			return one
		}
		return zero
	},
	func(lo, hi end) tenon.Value {
		minus := tenon.NumberFromInt(-1)
		switch {
		case lo.ok && (less(zero, lo.v) || lo.v.Equal(zero) && !lo.inclusive):
			return one
		case hi.ok && (less(hi.v, zero) || hi.v.Equal(zero) && !hi.inclusive):
			return minus
		case lo.ok && !less(lo.v, zero):
			return within(end{zero, true, true}, end{one, true, true})
		case hi.ok && !less(zero, hi.v):
			return within(end{minus, true, true}, end{zero, true, true})
		}
		return within(end{minus, true, true}, end{one, true, true})
	})

// IntFunc is the integer part of a number: the number truncated toward zero.
var IntFunc = rounding("Int", "Discards any fractional portion of the given number.", truncated)

// CeilFunc is the least integer not less than the number.
var CeilFunc = rounding("Ceil", "Returns the smallest whole number that is greater than or equal to the given value.", func(x tenon.Value) tenon.Value {
	if t := truncated(x); fractional(x) && less(zero, x) {
		return tenon.Add(t, one)
	} else {
		return t
	}
})

// FloorFunc is the greatest integer not greater than the number.
var FloorFunc = rounding("Floor", "Returns the greatest whole number that is less than or equal to the given value.", func(x tenon.Value) tenon.Value {
	if t := truncated(x); fractional(x) && less(x, zero) {
		return tenon.Sub(t, one)
	} else {
		return t
	}
})

// rounding returns a function rounding a number to an integer by round,
// which never decreases as its argument grows. A number not known yet
// answers within what round gives its range's ends: an end that the range
// leaves out and that is an integer is stepped past where round would give
// that end for numbers inside the range, which it does not.
func rounding(name, description string, round func(x tenon.Value) tenon.Value) tenon.Function {
	return unary(name, description, round, func(lo, hi end) tenon.Value {
		if lo.ok {
			v := round(lo.v)
			if !lo.inclusive && !fractional(lo.v) && v.Equal(lo.v) && !round(tenon.Add(lo.v, tenon.NumberFromText("0.5"))).Equal(v) {
				v = tenon.Add(v, one)
			}
			lo = end{v, true, true}
		}
		if hi.ok {
			v := round(hi.v)
			if !hi.inclusive && !fractional(hi.v) && v.Equal(hi.v) && !round(tenon.Sub(hi.v, tenon.NumberFromText("0.5"))).Equal(v) {
				v = tenon.Sub(v, one)
			}
			hi = end{v, true, true}
		}
		return within(lo, hi)
	})
}

// fractional reports whether the known number x is not an integer: its
// canonical text (NU-020) puts a digit after its point that its exponent
// does not carry back before it.
func fractional(x tenon.Value) bool { return scale(x) < 0 }

// scale returns the power of ten that the last digit of the known number x
// stands for: 0 for 120, 1 for 1.2e1, -2 for 1.25. The canonical text holds
// no digit beyond the last one that is not zero after its point, so x is an
// integer exactly where the scale is not negative.
func scale(x tenon.Value) int {
	s := strings.TrimPrefix(x.String(), "-")
	mantissa, exp, _ := strings.Cut(s, "e")
	e := 0
	if exp != "" {
		e, _ = strconv.Atoi(exp)
	}
	if _, frac, ok := strings.Cut(mantissa, "."); ok {
		e -= len(frac)
	}
	return e
}

// truncated returns the known number x truncated toward zero. An integer is
// itself and a number between -1 and 1 is zero, so the work is that of the
// digits x has after its point, whatever its magnitude.
func truncated(x tenon.Value) tenon.Value {
	switch {
	case !fractional(x):
		return x
	case less(negated(one), x) && less(x, one):
		return zero
	}
	return tenon.Sub(x, tenon.Mod(x, one))
}

// extreme returns a function of one number or more answering the one that
// comes first by before: the least, or the greatest.
func extreme(name, description string, before func(a, b tenon.Value) bool) tenon.Function {
	param := tenon.Param{Name: "numbers", Description: "The numbers to choose from.", Constraint: number, AllowUnknown: true}
	return tenon.NewFunction(tenon.FunctionSpec{
		Name:        name,
		Description: description,
		Params:      []tenon.Param{param},
		VarParam:    &param,
		Result:      number,
		NotNull:     true,
		Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
			return extremeOf(args, before), nil
		},
	})
}

// extremeOf answers Min or Max of args: of known numbers, the one that
// comes first by before; where one argument's range comes before every
// other's whatever they turn out to be, that argument as it is; and
// otherwise the unknown number between the ends that come first among the
// arguments' near ends and their far ends.
func extremeOf(args []tenon.Value, before func(a, b tenon.Value) bool) tenon.Value {
	// near is the end of a range on before's side, far the other.
	near := func(x tenon.Value) end {
		lo, hi := ends(x)
		if before(zero, one) {
			return lo
		}
		return hi
	}
	far := func(x tenon.Value) end {
		lo, hi := ends(x)
		if before(zero, one) {
			return hi
		}
		return lo
	}
	for j, x := range args {
		f := far(x)
		if !f.ok {
			continue
		}
		first := true
		for i, y := range args {
			if n := near(y); i != j && (!n.ok || before(n.v, f.v)) {
				first = false
				break
			}
		}
		if first {
			return x
		}
	}
	// The near end of the answer is the nearest of the near ends, and its
	// far end the nearest of the far ends: each argument bounds it.
	var n, f end
	for i, x := range args {
		xn, xf := near(x), far(x)
		switch {
		case !xn.ok:
			n.ok = false
		case i == 0 || n.ok && before(xn.v, n.v):
			n = xn
		case n.ok && xn.v.Equal(n.v):
			n.inclusive = n.inclusive || xn.inclusive
		}
		if xf.ok {
			switch {
			case !f.ok || before(xf.v, f.v):
				f = xf
			case xf.v.Equal(f.v):
				f.inclusive = f.inclusive || xf.inclusive
			}
		}
	}
	if before(zero, one) {
		return within(n, f)
	}
	return within(f, n)
}

// MinFunc is the least of one number or more.
var MinFunc = extreme("Min", "Returns the numerically smallest of all of the given numbers.", less)

// MaxFunc is the greatest of one number or more.
var MaxFunc = extreme("Max", "Returns the numerically greatest of all of the given numbers.", func(a, b tenon.Value) bool { return less(b, a) })

// parseIntLimit is the most bytes of text ParseIntFunc reads, as NU-024
// limits a number's text: digits cost the square of their number to read.
const parseIntLimit = 10000

// ParseIntFunc reads a string as an integer in a base from 2 to 62: an
// optional sign, - or +, then one digit or more of the base, 0 to 9, then
// the letters, a to z and A to Z alike up to base 36, and above it a to z for
// 10 to 35 and A to Z for 36 to 61. Nothing else is read: no prefix, no
// separator, no space. Text of more than 10,000 bytes fails with
// tenon.CodeNumberTooLong before any of it is read, text of no integer with
// tenon.CodeNumberInvalidSyntax, and a base outside 2 to 62 with
// tenon.CodeFunctionInvalidArgument. A value that is not a string fails, as
// go-cty's does, rather than having the digits of its text read. Where the
// text carries a redacting mark, a failure names it by the placeholder.
var ParseIntFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "ParseInt",
	Description: "Parses the given string as a number of the given base, or raises an error if the string contains invalid characters.",
	Params: []tenon.Param{
		{Name: "number", Description: "The text to read.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true, AllowMarked: true},
		{Name: "base", Description: "The base, from 2 to 62.", Constraint: number, AllowUnknown: true},
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if v, _ := tenon.Unmark(args[0]); v.IsResolved() && !v.Type().Equal(tenon.StringType()) {
			return tenon.Constraint{}, tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{
				Code:    tenon.CodeOperationWrongType,
				Message: "the text ParseInt reads must be a string, not " + v.Type().String(),
				Path:    argument(0),
			}))
		}
		return number, nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		text, marks := tenon.Unmark(args[0])
		answer := func(v tenon.Value) (tenon.Value, error) { return tenon.WithMarks(v, propagating(args[0])...), nil }
		base, known := 62, false
		if b := args[1]; b.IsKnown() {
			n, ok := b.AsInt64()
			if !ok || n < 2 || n > 62 {
				return tenon.ErrorVal(tenon.Diagnostic{
					Code:    tenon.CodeFunctionInvalidArgument,
					Message: "the base of ParseInt must be a whole number from 2 to 62, not " + b.String(),
					Path:    argument(1),
				}), nil
			}
			base, known = int(n), true
		}
		if !text.IsKnown() {
			return answer(tenon.Unknown(tenon.NumberType()))
		}
		s := text.AsString()
		what := strconv.Quote(s)
		for _, m := range marks {
			if m.Redacting() {
				what = args[0].String()
				break
			}
		}
		if len(s) > parseIntLimit {
			return answer(tenon.ErrorVal(tenon.Diagnostic{
				Code:    tenon.CodeNumberTooLong,
				Message: "the text ParseInt reads is longer than 10,000 bytes, and is not read",
				Path:    argument(0),
			}))
		}
		i, ok := parseInteger(s, base)
		if !ok {
			// Under a base not known yet, text the greatest base reads may
			// still be read by the base it turns out to be.
			message := what + " is no integer in base " + strconv.Itoa(base)
			if !known {
				message = what + " is no integer in any base"
			}
			return answer(tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeNumberInvalidSyntax, Message: message, Path: argument(0)}))
		}
		if !known {
			return answer(tenon.Unknown(tenon.NumberType()))
		}
		return answer(tenon.NumberFromBigInt(i))
	},
})

// parseInteger reads s as ParseIntFunc reads it in base.
func parseInteger(s string, base int) (*big.Int, bool) {
	digits := strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if len(s)-len(digits) > 1 || digits == "" {
		return nil, false
	}
	for _, c := range []byte(digits) {
		if digit(c, base) < 0 {
			return nil, false
		}
	}
	return new(big.Int).SetString(s, base)
}

// digit returns the value of the digit c in base, or -1 where c is none.
func digit(c byte, base int) int {
	var d int
	switch {
	case '0' <= c && c <= '9':
		d = int(c - '0')
	case 'a' <= c && c <= 'z':
		d = int(c-'a') + 10
	case 'A' <= c && c <= 'Z':
		d = int(c-'A') + 10
		if base > 36 {
			d += 26
		}
	default:
		return -1
	}
	if d >= base {
		return -1
	}
	return d
}

// argument returns the path of argument i of a call.
func argument(i int) tenon.Path { return tenon.Path{}.Index(tenon.NumberFromInt(int64(i))) }
