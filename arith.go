package tenon

import "tenon/internal/decimal"

// Add returns the sum of two Number values. The sum is exact: it is never
// rounded, and a result outside the range of numbers is an error value with
// code CodeNumberOutOfRange rather than a rounded one.
//
// If either operand is an error value the result is an error value. A null
// operand is an error value too, with code CodeOperationNullOperand, since a
// sum has no answer for null. When an operand is not known, the result is a
// Number bounded by what the operands' bounds allow.
//
// Add panics if an operand is a value of another type.
func Add(a, b Value) Value { return addOp.apply(a, b) }

// Sub returns the difference of two Number values, exactly, and treats its
// operands as Add does.
func Sub(a, b Value) Value { return subOp.apply(a, b) }

// Mul returns the product of two Number values, exactly, and treats its
// operands as Add does. It does not bound a result whose operands are not
// known, though the bounds of the operands would allow it.
func Mul(a, b Value) Value { return mulOp.apply(a, b) }

// Div returns the quotient of two Number values, rounded to the fixed
// precision that the specification gives when it does not terminate, and
// exactly when it does. Division by zero is an error value with code
// CodeNumberDivideByZero. Div treats its operands as Mul does.
func Div(a, b Value) Value { return divOp.apply(a, b) }

// Mod returns the remainder of dividing two Number values, whose sign follows
// the dividend. A zero divisor is an error value with code
// CodeNumberModuloByZero. Mod treats its operands as Mul does.
func Mod(a, b Value) Value { return modOp.apply(a, b) }

var (
	addOp = register(&op{
		name:     "Add",
		operands: alike(2, numberOperand, false),
		result:   fixedResult(Type{numberType}),
		known:    func(args []Value) Value { return arithmetic(decOf(args[0]).Add(decOf(args[1]))) },
		narrow:   addBounds,
	})
	subOp = register(&op{
		name:     "Sub",
		operands: alike(2, numberOperand, false),
		result:   fixedResult(Type{numberType}),
		known:    func(args []Value) Value { return arithmetic(decOf(args[0]).Sub(decOf(args[1]))) },
		narrow:   subBounds,
	})
	mulOp = register(&op{
		name:     "Mul",
		operands: alike(2, numberOperand, false),
		result:   fixedResult(Type{numberType}),
		known:    func(args []Value) Value { return arithmetic(decOf(args[0]).Mul(decOf(args[1]))) },
	})
	divOp = register(&op{
		name:     "Div",
		operands: alike(2, numberOperand, false),
		result:   fixedResult(Type{numberType}),
		known:    func(args []Value) Value { return arithmetic(decOf(args[0]).Div(decOf(args[1]))) },
	})
	modOp = register(&op{
		name:     "Mod",
		operands: alike(2, numberOperand, false),
		result:   fixedResult(Type{numberType}),
		known:    func(args []Value) Value { return arithmetic(decOf(args[0]).Mod(decOf(args[1]))) },
	})
)

// numberOperand is what the arithmetic operations accept.
var numberOperand = Exactly(Type{numberType})

// decOf returns the number that a known Number value holds.
func decOf(v Value) decimal.Dec { return v.n.data.(decimal.Dec) }

// arithmetic returns the value of an arithmetic result, or the error value
// saying why there is none.
func arithmetic(d decimal.Dec, err error) Value {
	switch err {
	case nil:
		return numberValue(d)
	case decimal.ErrDivideByZero:
		return errorValue(Diagnostic{Code: CodeNumberDivideByZero, Message: "a number cannot be divided by zero"})
	case decimal.ErrModuloByZero:
		return errorValue(Diagnostic{Code: CodeNumberModuloByZero, Message: "a number has no remainder modulo zero"})
	}
	return errorValue(Diagnostic{Code: CodeNumberOutOfRange, Message: "the result is outside the range of numbers"})
}

// addBounds bounds the result of Add by the bounds of its operands: a sum lies
// between the sums of the bounds, and includes them only where both did.
func addBounds(args []Value, r Value) Value {
	alo, ahi := numberBounds(args[0])
	blo, bhi := numberBounds(args[1])
	return boundedBy(r, combine(alo, blo, false), combine(ahi, bhi, false))
}

// subBounds bounds the result of Sub, which is least when the first operand is
// least and the second is greatest, and greatest the other way about.
func subBounds(args []Value, r Value) Value {
	alo, ahi := numberBounds(args[0])
	blo, bhi := numberBounds(args[1])
	return boundedBy(r, combine(alo, bhi, true), combine(ahi, blo, true))
}

// numberBounds returns the bounds of a Number operand. A known number is both
// of its own bounds; an operand that says nothing about its value, as a pending
// one does, leaves them unset.
func numberBounds(v Value) (lo, hi bound) {
	switch n := v.n; n.state {
	case stateKnown:
		b := bound{v: n.data.(decimal.Dec), incl: true, set: true}
		return b, b
	case stateUnknown:
		r := n.data.(*rangeData)
		return r.lo, r.hi
	}
	return bound{}, bound{}
}

// combine adds b to a, or subtracts it, giving a bound that is set only when
// both of its operands were and the arithmetic stayed in range. A bound that
// cannot be computed is left out, and the result says that much less.
func combine(a, b bound, subtract bool) bound {
	if !a.set || !b.set {
		return bound{}
	}
	var (
		v   decimal.Dec
		err error
	)
	if subtract {
		v, err = a.v.Sub(b.v)
	} else {
		v, err = a.v.Add(b.v)
	}
	if err != nil {
		return bound{}
	}
	return bound{v: v, incl: a.incl && b.incl, set: true}
}

// boundedBy narrows r by whichever of the bounds were computed.
func boundedBy(r Value, lo, hi bound) Value {
	var ns []Narrowing
	if lo.set {
		ns = append(ns, NumberMin(numberValue(lo.v), lo.incl))
	}
	if hi.set {
		ns = append(ns, NumberMax(numberValue(hi.v), hi.incl))
	}
	return Narrow(r, ns...)
}
