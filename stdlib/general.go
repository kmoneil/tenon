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
