package stdlib

import "github.com/kmoneil/tenon"

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
