// Package stdlib is tenon's standard library of functions: what a
// configuration language built on tenon offers its users to call, each a
// [tenon.Function]. It is go-cty's cty/function/stdlib, function for function,
// with tenon's call boundary, exact numbers, strings measured and cut by
// grapheme cluster, and every answer stated by the specification, so that a
// configuration evaluates alike wherever it runs.
//
// Each function is a variable named as go-cty names it, AssertNotNullFunc for
// go-cty's AssertNotNullFunc, so a host's table of functions moves from
// go-cty by its import path and its element type:
//
//	functions := map[string]tenon.Function{
//		"assertnotnull": stdlib.AssertNotNullFunc,
//	}
//
// A host calls one with [tenon.Call], which converts each argument to its
// parameter's constraint under the policy the call is given and answers every
// state the function does not admit, as it does for any function. A host still
// evaluating with go-cty, HCL's evaluator among them, calls these in place
// through the ctytenon module's Bridge.FunctionToCty.
//
// No function here is volatile. A function's failures carry tenon's codes,
// never the text of a Go error: an argument outside what the function has a
// meaning for is CodeFunctionInvalidArgument, located at the argument, and a
// result that would pass the bound the function states is
// CodeFunctionTooLarge, decided before the work.
package stdlib
