package stdlib

import "github.com/kmoneil/tenon"

// NotFunc is the negation of a Bool: tenon's Not.
var NotFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Not",
	Description: "Applies the logical NOT operation to the given boolean value.",
	Params:      []tenon.Param{operand("val", "The boolean value to negate.", boolean)},
	Result:      boolean,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return tenon.Not(args[0]), nil
	},
})

// AndFunc is the conjunction of two Bools: tenon's And, in which a false
// operand decides the answer and an error operand is never passed over.
var AndFunc = binary("And", "Applies the logical AND operation to the given boolean values.", boolean, boolean, tenon.And)

// OrFunc is the disjunction of two Bools: tenon's Or, in which a true
// operand decides the answer and an error operand is never passed over.
var OrFunc = binary("Or", "Applies the logical OR operation to the given boolean values.", boolean, boolean, tenon.Or)
