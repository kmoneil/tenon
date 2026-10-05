// Package proof holds tenon's standard library to HCL's evaluator: it
// evaluates expressions with HCL as it is, and again with tenon's functions
// in its place, and says where and why the answers differ.
//
// HCL calls a go-cty function for each operator, held in the exported
// variables hclsyntax.OpAdd and the rest. A host puts tenon's in their place
// by setting each one's Impl field to the library function crossed by
// ctytenon's Bridge.FunctionToCty, once, before any evaluation:
//
//	f, err := ctytenon.Bridge{}.FunctionToCty(stdlib.AddFunc, tenon.Unsafe)
//	hclsyntax.OpAdd.Impl = f
//
// Set the field, never the variable: HCL's parser holds the original
// operations, so a replaced variable changes nothing. The setting is the
// process's, so it belongs in an init function, before any goroutine
// evaluates. What it gives and what it does not:
//
//   - Each operation answers as tenon's: 1/0 fails with
//     number.divide_by_zero, 5 % 0 with number.modulo_by_zero, 0.1 + 0.2 is
//     0.3, and an operand not yet known keeps its bounds in the answer.
//   - Where tenon settles an answer from what is known, it is known, though
//     HCL's specification says unknown: an unknown number and an unknown
//     string are never equal, an unknown number times zero is zero, tenon
//     having no infinity, and [u, 1] == [1, 2] is false.
//   - An untyped null takes the other side's type first, inside a tuple or
//     an object as at the top, so [n] == [null] is true for a null string
//     n, where HCL as it is answers false.
//   - tenon has one zero: "${-0}" is "0".
//   - HCL converts each operand to the operation's type with go-cty before
//     the call, so go-cty's reading of strings as numbers stays ("1p4" is
//     16), but a string go-cty reads as an infinity, "inf" or "Inf", has no
//     tenon number to cross to, and the operation fails.
//   - Each answer crosses back to go-cty, which holds a number in 512 binary
//     bits, so exactness is the operation's, not the expression's: a number
//     of more than 153 digits rounds between one operation and the next.
//   - HCL decides && and || by its own callback where an operand decides
//     them, before the function is called, and takes a null operand there
//     as false.
//
// The module is not published; it runs as `make proof`.
package proof
