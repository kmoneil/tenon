// Package stdlib is tenon's standard library of functions: what a
// configuration language built on tenon offers its users to call, each a
// [tenon.Function]. It is go-cty's cty/function/stdlib, function for function,
// with tenon's call boundary, exact numbers, strings measured by grapheme
// cluster, and every answer stated by the specification, so that a
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
// The package holds go-cty's functions over values: the operators; the
// functions over numbers, Log and Pow correctly rounded to 96 significant
// digits; the general ones, Coalesce, MakeToFunc and AssertNotNull; and those
// over lists, sets, maps, tuples and objects, Range and SetProduct among
// them. It holds go-cty's functions over text too, under the Unicode version
// tenon states whatever the Go toolchain's: Upper, Lower and Title by
// Unicode's full case mappings; Strlen, Substr and Reverse by grapheme
// cluster; Split, Replace and the trims, which match and cut only where a
// cluster ends; Join, Sort, Chomp and Indent; Format and FormatList, which
// write numbers from their exact values; and Regex, RegexAll and
// RegexReplace, whose patterns are read as Go 1.26 reads them, with Unicode
// classes and case folding of that version. go-cty's functions over
// encodings and time are not here yet.
// A function reads a collection as it stands: where an argument is not known
// yet, it answers from what is, the members a list holds, the lengths an
// unknown one records, the attributes an object's type names, so its answer
// is as narrow as its arguments allow.
//
// The operator functions, AddFunc through NegateFunc, the four orderings,
// EqualFunc, NotEqualFunc, AndFunc, OrFunc and NotFunc, are what an
// expression language's operators call, each the tenon operation of its
// name. HCL's operators are go-cty functions held in hclsyntax.OpAdd and the
// rest, whose Impl field a host may set to these crossed by FunctionToCty;
// the repository's proof module does so, and says what that gives and what
// it does not.
//
// No function here is volatile. A function's failures carry tenon's codes,
// never the text of a Go error: an argument outside what the function has a
// meaning for is CodeFunctionInvalidArgument, located at the argument, and a
// result that would pass the bound the function states is
// CodeFunctionTooLarge, decided before the work.
package stdlib
