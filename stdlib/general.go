package stdlib

import "github.com/kmoneil/tenon"

// AssertNotNullFunc returns its argument as it is, marks included, and fails
// where the argument is null: a null of a type, or a pending value known to
// be null, with tenon.CodeOperationNullOperand located at the argument. Its
// result is never null, so an argument not known yet answers with itself
// narrowed not null, which a language uses to promise that a value it
// cannot see yet will not be null when it is.
var AssertNotNullFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "AssertNotNull",
	Description: "Returns the given value as it is, failing where it is null.",
	Params: []tenon.Param{{
		Name:         "value",
		Description:  "The value that must not be null.",
		Constraint:   tenon.Any(),
		AllowUnknown: true,
		AllowPending: true,
		AllowMarked:  true,
	}},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		return typeOf(args[0]), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return args[0], nil
	},
})

// typeOf returns the constraint v's type gives: exactly that type, or the
// constraint a pending value carries. A parameter admitting marks hands over
// a marked value, whose type a redacting mark withholds from a reader, so
// the marks come off first: the type is the function's to know, and the
// answer carries them.
func typeOf(v tenon.Value) tenon.Constraint {
	v, _ = tenon.Unmark(v)
	if v.IsPending() {
		return v.Constraint()
	}
	return tenon.Exactly(v.Type())
}

// equalityOperand returns a parameter of an equality, which has an answer for
// every value: null, values not known yet, pending ones, marked ones.
func equalityOperand(name, description string) tenon.Param {
	return tenon.Param{
		Name: name, Description: description, Constraint: tenon.Any(),
		AllowNull: true, AllowUnknown: true, AllowPending: true, AllowMarked: true,
	}
}

// EqualFunc reports whether two values are equal: tenon's Equals, after a
// language's untyped null is settled. A pending operand whose constraint
// admits the other operand's type is resolved to it first, so a null whose
// type was never given compares with a null of any type as equal, and with
// any other value as not; and two such nulls are equal. That is what x ==
// null asks of x.
var EqualFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Equal",
	Description: "Returns true if the two given values are equal, or false otherwise.",
	Params:      []tenon.Param{equalityOperand("a", "The first value."), equalityOperand("b", "The second value.")},
	Result:      boolean,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return equal(args[0], args[1]), nil
	},
})

// NotEqualFunc reports whether two values differ: EqualFunc's answer,
// negated.
var NotEqualFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "NotEqual",
	Description: "Returns false if the two given values are equal, or true otherwise.",
	Params:      []tenon.Param{equalityOperand("a", "The first value."), equalityOperand("b", "The second value.")},
	Result:      boolean,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return tenon.Not(equal(args[0], args[1])), nil
	},
})

// equal is EqualFunc's answer: Equals, once a pending operand is resolved
// to the type of the other where its constraint admits it, and true of two
// pending values both known to be null.
func equal(a, b tenon.Value) tenon.Value {
	ua, _ := tenon.Unmark(a)
	ub, _ := tenon.Unmark(b)
	switch {
	case ua.IsPending() && ub.IsPending():
		if ua.IsNull() && ub.IsNull() {
			return tenon.WithMarks(tenon.Bool(true), propagating(a, b)...)
		}
	case ua.IsPending() && ub.IsResolved():
		if tenon.Satisfies(ua.Constraint(), ub.Type()) {
			a = tenon.Resolve(a, ub.Type())
		}
	case ub.IsPending() && ua.IsResolved():
		if tenon.Satisfies(ub.Constraint(), ua.Type()) {
			b = tenon.Resolve(b, ua.Type())
		}
	}
	return tenon.Equals(a, b)
}

// propagating returns the marks of vs that reach what is derived from them:
// those that propagate, and redacting ones, whatever they say.
func propagating(vs ...tenon.Value) []tenon.Mark {
	var out []tenon.Mark
	for _, v := range vs {
		_, ms := tenon.Unmark(v)
		for _, m := range ms {
			if m.Propagation() == tenon.Propagate || m.Redacting() {
				out = append(out, m)
			}
		}
	}
	return out
}
