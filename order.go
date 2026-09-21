package tenon

import "github.com/kmoneil/tenon/internal/decimal"

// LessThan returns whether a comes before b, as a Bool value. It is defined for
// Number and String values only: numbers compare numerically, whatever way each
// was written, and strings by Unicode scalar value over the normalized form
// that every string value has.
//
// The other comparisons follow from this one, since a after b is b before a,
// and either of those with Equals gives the rest.
//
// An error operand gives an error value, and so does a null operand, since
// there is no order for null. Where an operand is not known, the answer is
// what the ranges settle: known where every value one operand may have comes
// before every value the other may have, or none does, by the bounds of a
// number and the prefix of a string, and an unknown Bool otherwise.
//
// LessThan panics on a value of any other type, and on two operands whose types
// differ, because there is no order between them.
func LessThan(a, b Value) Value { return lessThanOp.apply(a, b) }

var lessThanOp = register(&op{
	name:     "LessThan",
	operands: alike(2, OneOf(Exactly(Type{numberType}), Exactly(Type{stringType})), false),
	agree:    true,
	result:   fixedResult(Type{boolType}),
	known: func(args []Value) Value {
		a, b := args[0].n, args[1].n
		if a.typ.t.kind == KindNumber {
			return Bool(a.data.(decimal.Dec).Cmp(b.data.(decimal.Dec)) < 0)
		}
		// Comparing the bytes of the normalized form is comparing its scalar
		// values, because UTF-8 keeps them in order.
		return Bool(a.data.(string) < b.data.(string))
	},
	decided: lessThanDecided,
})

// lessThanDecided answers where the operands' ranges settle the order: where
// every value the first may have comes before every value the second may
// have, or none does. It reads the values a range holds other than null, as
// the arithmetic does, since a null operand is an error that appears once the
// value is known and is no answer a range holds.
func lessThanDecided(args []Value) (Value, bool) {
	a, b := args[0].n, args[1].n
	kind := Kind(0)
	for _, n := range []*node{a, b} {
		if n.state.resolved() {
			kind = n.typ.t.kind
		}
	}
	switch kind {
	case KindNumber:
		alo, ahi := numberBounds(args[0])
		blo, bhi := numberBounds(args[1])
		if ahi.set && blo.set {
			if c := ahi.v.Cmp(blo.v); c < 0 || c == 0 && !(ahi.incl && blo.incl) {
				return Bool(true), true
			}
		}
		if alo.set && bhi.set && alo.v.Cmp(bhi.v) >= 0 {
			return Bool(false), true
		}
	case KindString:
		pa, wholeA := stringPrefix(a)
		pb, wholeB := stringPrefix(b)
		// The first difference within the part both prefixes cover orders
		// every value of one against every value of the other.
		for i := range min(len(pa), len(pb)) {
			if pa[i] != pb[i] {
				return Bool(pa[i] < pb[i]), true
			}
		}
		// Otherwise one prefix begins the other, and a whole string that
		// properly begins every value of the other operand comes before them.
		switch {
		case wholeA && len(pa) < len(pb):
			return Bool(true), true
		case wholeB && len(pb) < len(pa):
			return Bool(false), true
		}
	}
	return Value{}, false
}

// stringPrefix returns what every value a String operand may have begins
// with, and whether that is the whole of every such value, as it is of a
// known string.
func stringPrefix(n *node) (string, bool) {
	switch n.state {
	case stateKnown:
		return n.data.(string), true
	case stateUnknown:
		return n.data.(*rangeData).pfx, false
	}
	return "", false
}
