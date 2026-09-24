package tenon

import "github.com/kmoneil/tenon/internal/decimal"

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
// operands as Add does: a product whose operands are not known is bounded by
// what their bounds allow, so a factor of zero gives zero however little is
// known of the other.
func Mul(a, b Value) Value { return mulOp.apply(a, b) }

// Div returns the quotient of two Number values, rounded to the fixed
// precision that the specification gives when it does not terminate, and
// exactly when it does. Division by zero is an error value with code
// CodeNumberDivideByZero. Div treats its operands as Mul does, bounding a
// quotient where the divisor's bounds keep it away from zero; a divisor that
// may come as near zero as it likes leaves the quotient unbounded. Since a
// quotient is rounded, a bound on one includes its own value.
func Div(a, b Value) Value { return divOp.apply(a, b) }

// Mod returns the remainder of dividing two Number values, whose sign follows
// the dividend. A zero divisor is an error value with code
// CodeNumberModuloByZero. Mod treats its operands as Mul does: a remainder
// lies between zero and the dividend, and is smaller in magnitude than the
// divisor can be.
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
		narrow:   mulBounds,
	})
	divOp = register(&op{
		name:     "Div",
		operands: alike(2, numberOperand, false),
		result:   fixedResult(Type{numberType}),
		known:    func(args []Value) Value { return arithmetic(decOf(args[0]).Div(decOf(args[1]))) },
		narrow:   divBounds,
	})
	modOp = register(&op{
		name:     "Mod",
		operands: alike(2, numberOperand, false),
		result:   fixedResult(Type{numberType}),
		known:    func(args []Value) Value { return arithmetic(decOf(args[0]).Mod(decOf(args[1]))) },
		narrow:   modBounds,
	})
)

// numberOperand is what the arithmetic operations accept.
var numberOperand = Exactly(Type{numberType})

// decOf returns the number that a known Number value holds.
func decOf(v Value) decimal.Dec { return v.n.data.(decimal.Dec) }

// arithmetic returns the value of an arithmetic result, or the error value
// saying why there is none.
func arithmetic(d decimal.Dec, err error) Value {
	if err == nil {
		return numberValue(d)
	}
	code := numberCode(err.(decimal.Error))
	var message string
	switch code {
	case CodeNumberDivideByZero:
		message = "a number cannot be divided by zero"
	case CodeNumberModuloByZero:
		message = "a number has no remainder modulo zero"
	case CodeNumberOutOfRange:
		message = "the result is outside the range of numbers"
	default:
		internalPanic("an arithmetic operation reported %v, which none reports", err)
	}
	return errorValue(Diagnostic{Code: code, Message: message})
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

// end is an endpoint of the values a Number operand may have: one of its
// bounds, or, where that bound is not set, the infinity on that side, which
// inf holds as -1 or +1.
type end struct {
	bound
	inf int
}

// endsOf returns the endpoints of a Number operand's values.
func endsOf(v Value) (lo, hi end) {
	l, h := numberBounds(v)
	lo, hi = end{bound: l}, end{bound: h}
	if !l.set {
		lo.inf = -1
	}
	if !h.set {
		hi.inf = +1
	}
	return lo, hi
}

// sign returns the sign of an endpoint, an infinity's included.
func (e end) sign() int {
	if e.inf != 0 {
		return e.inf
	}
	return e.v.Sign()
}

// attainedZero reports whether e is zero, and zero is a value the operand may
// have.
func (e end) attainedZero() bool { return e.inf == 0 && e.incl && e.v.Sign() == 0 }

// cmp orders two endpoints along the line, infinities at its ends.
func (e end) cmp(f end) int {
	switch {
	case e.inf != 0 || f.inf != 0:
		return e.inf - f.inf
	}
	return e.v.Cmp(f.v)
}

// hull returns the least and the greatest of the corners as bounds: unset
// where that corner is an infinity, and inclusive where any corner of that
// value is one the result may have.
func hull(corners []end) (lo, hi bound) {
	least, greatest := corners[0], corners[0]
	for _, c := range corners[1:] {
		switch d := c.cmp(least); {
		case d < 0:
			least = c
		case d == 0:
			least.incl = least.incl || c.incl
		}
		switch d := c.cmp(greatest); {
		case d > 0:
			greatest = c
		case d == 0:
			greatest.incl = greatest.incl || c.incl
		}
	}
	if least.inf == 0 {
		lo = least.bound
	}
	if greatest.inf == 0 {
		hi = greatest.bound
	}
	return lo, hi
}

// mulBounds bounds the result of Mul. Over the values each operand may have,
// a product is least and greatest at the corners, the products of the bounds,
// since it moves one way as either factor does while the other keeps its
// sign; an infinite bound makes an infinite corner, except against a factor
// of zero, which makes every product zero.
func mulBounds(args []Value, r Value) Value {
	alo, ahi := endsOf(args[0])
	blo, bhi := endsOf(args[1])
	corners := make([]end, 0, 4)
	for _, x := range []end{alo, ahi} {
		for _, y := range []end{blo, bhi} {
			c, ok := product(x, y)
			if !ok {
				return r
			}
			corners = append(corners, c)
		}
	}
	lo, hi := hull(corners)
	return boundedBy(r, lo, hi)
}

// product returns the corner of two endpoints, and false where it is a number
// that falls outside the digit window, which bounds nothing.
func product(x, y end) (end, bool) {
	switch {
	case x.inf == 0 && y.inf == 0:
		v, err := x.v.Mul(y.v)
		if err != nil {
			return end{}, false
		}
		// A product is one the result may have where both factors are, and
		// where either factor may be zero, which makes it zero whatever the
		// other is.
		return end{bound: bound{v: v, set: true, incl: x.incl && y.incl || x.attainedZero() || y.attainedZero()}}, true
	case x.inf == 0 && x.v.Sign() == 0:
		return end{bound: x.bound}, true
	case y.inf == 0 && y.v.Sign() == 0:
		return end{bound: y.bound}, true
	}
	return end{inf: x.sign() * y.sign()}, true
}

// divBounds bounds the result of Div where the divisor's bounds keep it away
// from zero: then an exact quotient moves one way as either operand does, and
// is least and greatest at the corners, as a product is. A divisor that may
// come as near zero as it likes makes a quotient as large as it likes, and
// bounds nothing.
//
// What Div gives is not the exact quotient, and not a monotone function of it
// either: a quotient that terminates is exact at any length, and one that does
// not is rounded to 96 digits ([NU-011], [NU-012]), so a quotient just inside a
// corner can come out beyond the corner's own rounded value. Each corner
// therefore stands as its quotient rounded down and rounded up at 96 digits,
// which every result on either side of it respects, and which are the corner
// itself where it terminates within them. Every bound is inclusive, since a
// quotient the corner excludes may round onto it.
func divBounds(args []Value, r Value) Value {
	alo, ahi := endsOf(args[0])
	blo, bhi := endsOf(args[1])
	positive := blo.inf == 0 && blo.v.Sign() > 0
	negative := bhi.inf == 0 && bhi.v.Sign() < 0
	if !positive && !negative {
		return r
	}
	corners := make([]end, 0, 8)
	for _, x := range []end{alo, ahi} {
		for _, y := range []end{blo, bhi} {
			switch {
			case x.inf == 0 && y.inf == 0:
				down, err := x.v.DivBound(y.v, false)
				if err != nil {
					return r
				}
				up, err := x.v.DivBound(y.v, true)
				if err != nil {
					return r
				}
				corners = append(corners, end{bound: bound{v: down, set: true, incl: true}}, end{bound: bound{v: up, set: true, incl: true}})
			case x.inf == 0:
				// A number over a divisor that grows without bound tends to
				// zero, from one side or the other.
				corners = append(corners, end{bound: bound{v: decimal.Dec{}, set: true, incl: true}})
			case y.inf == 0:
				corners = append(corners, end{inf: x.sign() * y.sign()})
			}
			// An infinity over an infinity adds nothing that the corners
			// beside it do not: the quotient tends to whatever lies between
			// zero and the infinity those give.
		}
	}
	lo, hi := hull(corners)
	return boundedBy(r, lo, hi)
}

// modBounds bounds the result of Mod. A remainder has the sign of the
// dividend and no greater magnitude ([NU-017]), so it lies between zero and
// the dividend's own bounds, which always holds zero; and its magnitude is
// less than the divisor's, which is at most the greater magnitude of the
// divisor's bounds where both are set.
func modBounds(args []Value, r Value) Value {
	alo, ahi := numberBounds(args[0])
	blo, bhi := numberBounds(args[1])
	zero := bound{v: decimal.Dec{}, set: true, incl: true}
	lo, hi := zero, zero
	switch {
	case !alo.set:
		lo = bound{}
	case alo.v.Sign() < 0:
		lo = alo
	}
	switch {
	case !ahi.set:
		hi = bound{}
	case ahi.v.Sign() > 0:
		hi = ahi
	}
	if blo.set && bhi.set {
		m := magnitude(blo.v)
		if h := magnitude(bhi.v); h.Cmp(m) > 0 {
			m = h
		}
		// A divisor that can only be zero leaves no remainder at all, every
		// outcome being an error, and bounds nothing, as it does a quotient.
		if m.Sign() > 0 {
			lo = tighterLo(lo, bound{v: m.Neg(), set: true})
			hi = tighterHi(hi, bound{v: m, set: true})
		}
	}
	return boundedBy(r, lo, hi)
}

// magnitude returns d without its sign.
func magnitude(d decimal.Dec) decimal.Dec {
	if d.Sign() < 0 {
		return d.Neg()
	}
	return d
}

// tighterLo returns the greater of two lower bounds, which is what both say
// together: the exclusive one where their values are the same.
func tighterLo(a, b bound) bound {
	switch {
	case !a.set:
		return b
	case !b.set:
		return a
	}
	switch c := a.v.Cmp(b.v); {
	case c > 0:
		return a
	case c < 0:
		return b
	}
	return bound{v: a.v, set: true, incl: a.incl && b.incl}
}

// tighterHi returns the lesser of two upper bounds, as tighterLo does the
// greater of two lower ones.
func tighterHi(a, b bound) bound {
	switch {
	case !a.set:
		return b
	case !b.set:
		return a
	}
	switch c := a.v.Cmp(b.v); {
	case c < 0:
		return a
	case c > 0:
		return b
	}
	return bound{v: a.v, set: true, incl: a.incl && b.incl}
}
