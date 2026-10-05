package tenon_test

import (
	"fmt"

	"github.com/kmoneil/tenon"
)

// A function is defined once, from a specification, and called with
// arguments in whatever state the configuration has them: the call converts
// each argument to its parameter's constraint under the policy, answers the
// states the implementation does not admit, and reports every failing
// argument, located by its zero-based index.
func ExampleCall() {
	num := tenon.Exactly(tenon.NumberType())
	scale := tenon.NewFunction(tenon.FunctionSpec{
		Name: "Scale",
		Params: []tenon.Param{
			{Name: "count", Constraint: num},
			{Name: "by", Constraint: num},
		},
		Result:  num,
		NotNull: true,
		Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
			return tenon.Mul(args[0], args[1]), nil
		},
	})

	fmt.Println(tenon.Call(scale, []tenon.Value{tenon.NumberFromInt(3), tenon.NumberFromInt(4)}, tenon.Safe))

	// A string of digits is a number argument under the unsafe policy, as a
	// configuration hands numbers around.
	fmt.Println(tenon.Call(scale, []tenon.Value{tenon.String("8080"), tenon.NumberFromInt(2)}, tenon.Unsafe))

	// An argument not yet known answers with the unknown of the result,
	// without the implementation running, and the result is declared never
	// null, so the answer says so.
	fmt.Println(tenon.Call(scale, []tenon.Value{tenon.Unknown(tenon.NumberType()), tenon.NumberFromInt(2)}, tenon.Safe))

	// Every failing argument reports, each diagnostic located by the
	// argument's index.
	r := tenon.Call(scale, []tenon.Value{tenon.Null(tenon.NumberType()), tenon.Bool(true)}, tenon.Safe)
	for _, d := range r.Diagnostics() {
		fmt.Printf("%s at %s: %s\n", d.Code, d.Path, d.Message)
	}
	// Output:
	// 12
	// 16160
	// unknown(number, not null)
	// operation.null_operand at .[0]: argument 1 (count) of Scale is null, which Scale cannot use
	// convert.no_conversion at .[1]: bool does not convert to exactly(number)
}
