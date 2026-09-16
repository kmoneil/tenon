package tenon

import "tenon/internal/decimal"

// LessThan returns whether a comes before b, as a Bool value. It is defined for
// Number and String values only: numbers compare numerically, whatever way each
// was written, and strings by Unicode scalar value over the normalized form
// that every string value has.
//
// The other comparisons follow from this one, since a after b is b before a,
// and either of those with Equals gives the rest.
//
// An error operand gives an error value, and so does a null operand, since
// there is no order for null. Where an operand is not known the answer is an
// unknown Bool: the bounds a range carries could settle some of those, and
// LessThan does not try, which the rule on narrowing allows.
//
// LessThan panics on a value of any other type, and on two operands whose types
// differ, because there is no order between them.
func LessThan(a, b Value) Value { return lessThanOp.apply(a, b) }

var lessThanOp = &op{
	name:    "LessThan",
	operand: OneOf(Exactly(Type{numberType}), Exactly(Type{stringType})),
	agree:   true,
	result:  fixedResult(Type{boolType}),
	known: func(args []Value) Value {
		a, b := args[0].n, args[1].n
		if a.typ.t.kind == KindNumber {
			return Bool(a.data.(decimal.Dec).Cmp(b.data.(decimal.Dec)) < 0)
		}
		// Comparing the bytes of the normalized form is comparing its scalar
		// values, because UTF-8 keeps them in order.
		return Bool(a.data.(string) < b.data.(string))
	},
}
