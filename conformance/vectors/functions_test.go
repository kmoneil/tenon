package vectors_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// library is the standard library's functions, by the names the
// specification gives them.
var library = map[string]tenon.Function{
	"AssertNotNull":          stdlib.AssertNotNullFunc,
	"Keys":                   stdlib.KeysFunc,
	"Values":                 stdlib.ValuesFunc,
	"Zipmap":                 stdlib.ZipmapFunc,
	"Lookup":                 stdlib.LookupFunc,
	"Merge":                  stdlib.MergeFunc,
	"Upper":                  stdlib.UpperFunc,
	"Lower":                  stdlib.LowerFunc,
	"Title":                  stdlib.TitleFunc,
	"Strlen":                 stdlib.StrlenFunc,
	"Reverse":                stdlib.ReverseFunc,
	"Substr":                 stdlib.SubstrFunc,
	"Split":                  stdlib.SplitFunc,
	"Replace":                stdlib.ReplaceFunc,
	"TrimPrefix":             stdlib.TrimPrefixFunc,
	"TrimSuffix":             stdlib.TrimSuffixFunc,
	"Trim":                   stdlib.TrimFunc,
	"TrimSpace":              stdlib.TrimSpaceFunc,
	"Chomp":                  stdlib.ChompFunc,
	"Indent":                 stdlib.IndentFunc,
	"Range":                  stdlib.RangeFunc,
	"SetProduct":             stdlib.SetProductFunc,
	"Contains":               stdlib.ContainsFunc,
	"SetHasElement":          stdlib.SetHasElementFunc,
	"SetUnion":               stdlib.SetUnionFunc,
	"SetIntersection":        stdlib.SetIntersectionFunc,
	"SetSubtract":            stdlib.SetSubtractFunc,
	"SetSymmetricDifference": stdlib.SetSymmetricDifferenceFunc,
	"Flatten":                stdlib.FlattenFunc,
	"Compact":                stdlib.CompactFunc,
	"Distinct":               stdlib.DistinctFunc,
	"CoalesceList":           stdlib.CoalesceListFunc,
	"Slice":                  stdlib.SliceFunc,
	"ReverseList":            stdlib.ReverseListFunc,
	"Concat":                 stdlib.ConcatFunc,
	"Chunklist":              stdlib.ChunklistFunc,
	"Length":                 stdlib.LengthFunc,
	"HasIndex":               stdlib.HasIndexFunc,
	"Index":                  stdlib.IndexFunc,
	"Element":                stdlib.ElementFunc,
	"Log":                    stdlib.LogFunc,
	"Pow":                    stdlib.PowFunc,
	"Absolute":               stdlib.AbsoluteFunc,
	"Signum":                 stdlib.SignumFunc,
	"Int":                    stdlib.IntFunc,
	"Ceil":                   stdlib.CeilFunc,
	"Floor":                  stdlib.FloorFunc,
	"Min":                    stdlib.MinFunc,
	"Max":                    stdlib.MaxFunc,
	"ParseInt":               stdlib.ParseIntFunc,
	"Coalesce":               stdlib.CoalesceFunc,
	"MakeTo":                 stdlib.MakeToFunc(tenon.Exactly(tenon.NumberType())),
	"Add":                    stdlib.AddFunc,
	"Subtract":               stdlib.SubtractFunc,
	"Multiply":               stdlib.MultiplyFunc,
	"Divide":                 stdlib.DivideFunc,
	"Modulo":                 stdlib.ModuloFunc,
	"Negate":                 stdlib.NegateFunc,
	"LessThan":               stdlib.LessThanFunc,
	"LessThanOrEqualTo":      stdlib.LessThanOrEqualToFunc,
	"GreaterThan":            stdlib.GreaterThanFunc,
	"GreaterThanOrEqualTo":   stdlib.GreaterThanOrEqualToFunc,
	"Equal":                  stdlib.EqualFunc,
	"NotEqual":               stdlib.NotEqualFunc,
	"Not":                    stdlib.NotFunc,
	"And":                    stdlib.AndFunc,
	"Or":                     stdlib.OrFunc,
}

// callVector is a call of a library function with arguments under a policy.
type callVector struct {
	name     string
	function string
	args     []tenon.Value
	policy   tenon.Policy
	// to is the constraint MakeTo makes its function for, and is zero for
	// every other function.
	to tenon.Constraint
}

// callVectors are the calls functions.json records: what each answers, or
// how it fails. A name begins with the function's and, for a call that
// fails, "refused/" after it.
var callVectors = []callVector{
	{"AssertNotNull/a number", "AssertNotNull", []tenon.Value{n(1)}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/marks", "AssertNotNull", []tenon.Value{tenon.WithMarks(s("hunter2"), secret{}, plain)}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/a tuple holding a null", "AssertNotNull", []tenon.Value{tenon.Tuple(tenon.Null(str), n(1))}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/unknown", "AssertNotNull", []tenon.Value{tenon.Unknown(str)}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/unknown with a prefix", "AssertNotNull", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("abc"))}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/pending", "AssertNotNull", []tenon.Value{tenon.Pending(tenon.ListOf(tenon.Any()))}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/refused/null", "AssertNotNull", []tenon.Value{tenon.Null(str)}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/refused/pending null", "AssertNotNull", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/refused/redacted null", "AssertNotNull", []tenon.Value{tenon.WithMarks(tenon.Null(str), secret{})}, tenon.Safe, tenon.Constraint{}},
	{"AssertNotNull/refused/arity", "AssertNotNull", []tenon.Value{n(1), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Add/exact", "Add", []tenon.Value{tenon.NumberFromText("0.1"), tenon.NumberFromText("0.2")}, tenon.Safe, tenon.Constraint{}},
	{"Add/large", "Add", []tenon.Value{tenon.NumberFromText("1e200"), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Add/a range", "Add", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(1), true)), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Add/a string under Unsafe", "Add", []tenon.Value{s("2"), n(3)}, tenon.Unsafe, tenon.Constraint{}},
	{"Add/marked", "Add", []tenon.Value{tenon.WithMarks(n(2), plain), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Add/refused/null", "Add", []tenon.Value{tenon.Null(num), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Add/refused/a string under Safe", "Add", []tenon.Value{s("2"), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Subtract/exact", "Subtract", []tenon.Value{tenon.NumberFromText("0.3"), tenon.NumberFromText("0.1")}, tenon.Safe, tenon.Constraint{}},
	{"Multiply/exact", "Multiply", []tenon.Value{tenon.NumberFromText("1.1"), tenon.NumberFromText("1.1")}, tenon.Safe, tenon.Constraint{}},
	{"Divide/terminating", "Divide", []tenon.Value{n(1), n(8)}, tenon.Safe, tenon.Constraint{}},
	{"Divide/rounded", "Divide", []tenon.Value{n(2), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Divide/refused/by zero", "Divide", []tenon.Value{n(1), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Divide/refused/an unknown by zero", "Divide", []tenon.Value{tenon.Unknown(num), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Modulo/the dividend's sign", "Modulo", []tenon.Value{n(-7), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Modulo/decimal", "Modulo", []tenon.Value{tenon.NumberFromText("0.9"), tenon.NumberFromText("0.3")}, tenon.Safe, tenon.Constraint{}},
	{"Modulo/large", "Modulo", []tenon.Value{tenon.NumberFromText("1e200"), n(7)}, tenon.Safe, tenon.Constraint{}},
	{"Modulo/refused/by zero", "Modulo", []tenon.Value{n(1), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Negate/a number", "Negate", []tenon.Value{tenon.NumberFromText("-2.5")}, tenon.Safe, tenon.Constraint{}},
	{"Negate/a range", "Negate", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(1), true))}, tenon.Safe, tenon.Constraint{}},
	{"LessThan/numbers", "LessThan", []tenon.Value{n(1), tenon.NumberFromText("1.5")}, tenon.Safe, tenon.Constraint{}},
	{"LessThanOrEqualTo/equal", "LessThanOrEqualTo", []tenon.Value{tenon.NumberFromText("1.50"), tenon.NumberFromText("1.5")}, tenon.Safe, tenon.Constraint{}},
	{"GreaterThan/strings as numbers", "GreaterThan", []tenon.Value{s("10"), s("9")}, tenon.Unsafe, tenon.Constraint{}},
	{"GreaterThan/settled by a range", "GreaterThan", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true)), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"GreaterThanOrEqualTo/numbers", "GreaterThanOrEqualTo", []tenon.Value{n(2), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Equal/an untyped null and a typed one", "Equal", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Null(str)}, tenon.Safe, tenon.Constraint{}},
	{"Equal/an untyped null and a value", "Equal", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), s("x")}, tenon.Safe, tenon.Constraint{}},
	{"Equal/two untyped nulls", "Equal", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe, tenon.Constraint{}},
	{"Equal/types differ", "Equal", []tenon.Value{n(1), s("1")}, tenon.Safe, tenon.Constraint{}},
	{"Equal/numbers spelled differently", "Equal", []tenon.Value{tenon.NumberFromText("1.50"), tenon.NumberFromText("1.5")}, tenon.Safe, tenon.Constraint{}},
	{"NotEqual/an untyped null and a value", "NotEqual", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), s("x")}, tenon.Safe, tenon.Constraint{}},
	{"Not/true", "Not", []tenon.Value{tenon.Bool(true)}, tenon.Safe, tenon.Constraint{}},
	{"And/false decides", "And", []tenon.Value{tenon.Bool(false), tenon.Unknown(boo)}, tenon.Safe, tenon.Constraint{}},
	{"Or/true decides", "Or", []tenon.Value{tenon.Unknown(boo), tenon.Bool(true)}, tenon.Safe, tenon.Constraint{}},
	{"Or/refused/null", "Or", []tenon.Value{tenon.Bool(true), tenon.Null(boo)}, tenon.Safe, tenon.Constraint{}},
	{"Absolute/negative", "Absolute", []tenon.Value{tenon.NumberFromText("-2.5")}, tenon.Safe, tenon.Constraint{}},
	{"Absolute/a range across zero", "Absolute", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(-5), true), tenon.NumberMax(n(3), true))}, tenon.Safe, tenon.Constraint{}},
	{"Signum/a fraction", "Signum", []tenon.Value{tenon.NumberFromText("-0.5")}, tenon.Safe, tenon.Constraint{}},
	{"Signum/beyond 64 bits", "Signum", []tenon.Value{tenon.NumberFromText("9223372036854775808")}, tenon.Safe, tenon.Constraint{}},
	{"Signum/a positive range", "Signum", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(1), true))}, tenon.Safe, tenon.Constraint{}},
	{"Int/toward zero", "Int", []tenon.Value{tenon.NumberFromText("-1.5")}, tenon.Safe, tenon.Constraint{}},
	{"Int/large", "Int", []tenon.Value{tenon.NumberFromText("1e999999")}, tenon.Safe, tenon.Constraint{}},
	{"Ceil/a negative fraction", "Ceil", []tenon.Value{tenon.NumberFromText("-0.5")}, tenon.Safe, tenon.Constraint{}},
	{"Ceil/a tiny fraction", "Ceil", []tenon.Value{tenon.NumberFromText("1e-999999")}, tenon.Safe, tenon.Constraint{}},
	{"Ceil/an open range", "Ceil", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(2), false), tenon.NumberMax(n(3), false))}, tenon.Safe, tenon.Constraint{}},
	{"Floor/a negative fraction", "Floor", []tenon.Value{tenon.NumberFromText("-1.5")}, tenon.Safe, tenon.Constraint{}},
	{"Floor/96 nines", "Floor", []tenon.Value{tenon.Mul(tenon.Div(n(1), n(3)), n(3))}, tenon.Safe, tenon.Constraint{}},
	{"Min/numbers", "Min", []tenon.Value{n(3), tenon.NumberFromText("1.5"), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Min/one below the rest", "Min", []tenon.Value{n(2), tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true))}, tenon.Safe, tenon.Constraint{}},
	{"Min/overlapping ranges", "Min", []tenon.Value{tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(1), true), tenon.NumberMax(n(6), true)), tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(3), false), tenon.NumberMax(n(5), true))}, tenon.Safe, tenon.Constraint{}},
	{"Min/refused/no number", "Min", []tenon.Value{}, tenon.Safe, tenon.Constraint{}},
	{"Max/numbers", "Max", []tenon.Value{n(3), tenon.NumberFromText("1.5"), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"ParseInt/hexadecimal", "ParseInt", []tenon.Value{s("+ff"), n(16)}, tenon.Safe, tenon.Constraint{}},
	{"ParseInt/base 62", "ParseInt", []tenon.Value{s("Zz"), n(62)}, tenon.Safe, tenon.Constraint{}},
	{"ParseInt/a base not known yet", "ParseInt", []tenon.Value{s("ff"), tenon.Unknown(num)}, tenon.Safe, tenon.Constraint{}},
	{"ParseInt/refused/a prefix", "ParseInt", []tenon.Value{s("0xff"), n(16)}, tenon.Safe, tenon.Constraint{}},
	{"ParseInt/refused/a base out of range", "ParseInt", []tenon.Value{s("10"), n(63)}, tenon.Safe, tenon.Constraint{}},
	{"ParseInt/refused/a number", "ParseInt", []tenon.Value{n(10), n(16)}, tenon.Safe, tenon.Constraint{}},
	{"ParseInt/refused/redacted text", "ParseInt", []tenon.Value{tenon.WithMarks(s("hunter2"), secret{}), n(10)}, tenon.Safe, tenon.Constraint{}},
	{"Log/exact", "Log", []tenon.Value{n(1000), n(10)}, tenon.Safe, tenon.Constraint{}},
	{"Log/correctly rounded", "Log", []tenon.Value{n(2), n(10)}, tenon.Safe, tenon.Constraint{}},
	{"Log/a third", "Log", []tenon.Value{n(2), n(8)}, tenon.Safe, tenon.Constraint{}},
	{"Log/refused/not positive", "Log", []tenon.Value{n(0), n(10)}, tenon.Safe, tenon.Constraint{}},
	{"Log/refused/base one, whatever the number", "Log", []tenon.Value{tenon.Unknown(num), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Pow/exact", "Pow", []tenon.Value{n(10), n(23)}, tenon.Safe, tenon.Constraint{}},
	{"Pow/a root", "Pow", []tenon.Value{n(2), tenon.NumberFromText("0.5")}, tenon.Safe, tenon.Constraint{}},
	{"Pow/a midpoint", "Pow", []tenon.Value{n(5), n(138)}, tenon.Safe, tenon.Constraint{}},
	{"Pow/a negative base, an odd power", "Pow", []tenon.Value{n(-2), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Pow/zero to zero", "Pow", []tenon.Value{n(0), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Pow/refused/zero to a negative power", "Pow", []tenon.Value{n(0), n(-1)}, tenon.Safe, tenon.Constraint{}},
	{"Pow/refused/no real power", "Pow", []tenon.Value{n(-1), tenon.NumberFromText("0.5")}, tenon.Safe, tenon.Constraint{}},
	{"Pow/refused/out of range", "Pow", []tenon.Value{n(2), tenon.NumberFromText("1e400")}, tenon.Safe, tenon.Constraint{}},
	{"Length/a list", "Length", []tenon.Value{tenon.List(str, s("a"), tenon.Unknown(str))}, tenon.Safe, tenon.Constraint{}},
	{"Length/a string", "Length", []tenon.Value{s("e\U00000301\U0001F1FA\U0001F1F8")}, tenon.Safe, tenon.Constraint{}},
	{"Length/an object", "Length", []tenon.Value{tenon.Object(map[string]tenon.Value{"x": n(1), "y": n(2)})}, tenon.Safe, tenon.Constraint{}},
	{"Length/an unknown tuple", "Length", []tenon.Value{tenon.Unknown(tenon.TupleType(str, num))}, tenon.Safe, tenon.Constraint{}},
	{"Length/a set holding an unknown", "Length", []tenon.Value{tenon.Set(num, n(1), tenon.Unknown(num))}, tenon.Safe, tenon.Constraint{}},
	{"Length/refused/a number", "Length", []tenon.Value{n(1)}, tenon.Safe, tenon.Constraint{}},
	{"HasIndex/in range", "HasIndex", []tenon.Value{tenon.List(str, s("a"), s("b")), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"HasIndex/a string key into a list", "HasIndex", []tenon.Value{tenon.List(str, s("a")), s("0")}, tenon.Safe, tenon.Constraint{}},
	{"HasIndex/an object's attribute", "HasIndex", []tenon.Value{tenon.Object(map[string]tenon.Value{"x": n(1)}), s("x")}, tenon.Safe, tenon.Constraint{}},
	{"HasIndex/settled by a length", "HasIndex", []tenon.Value{tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMin(1)), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Index/a list", "Index", []tenon.Value{tenon.List(str, s("a"), s("b")), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Index/a map", "Index", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"k": n(1)}), s("k")}, tenon.Safe, tenon.Constraint{}},
	{"Index/refused/out of range", "Index", []tenon.Value{tenon.List(str, s("a")), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Index/refused/the wrong kind of key", "Index", []tenon.Value{tenon.List(str, s("a")), s("0")}, tenon.Safe, tenon.Constraint{}},
	{"Element/wrapped", "Element", []tenon.Value{tenon.List(str, s("a"), s("b"), s("c")), n(4)}, tenon.Safe, tenon.Constraint{}},
	{"Element/from the end", "Element", []tenon.Value{tenon.List(str, s("a"), s("b"), s("c")), n(-1)}, tenon.Safe, tenon.Constraint{}},
	{"Element/a large index", "Element", []tenon.Value{tenon.List(str, s("a"), s("b"), s("c")), tenon.NumberFromText("1e30")}, tenon.Safe, tenon.Constraint{}},
	{"Element/refused/empty", "Element", []tenon.Value{tenon.List(str), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Element/refused/not a whole number", "Element", []tenon.Value{tenon.List(str, s("a")), tenon.NumberFromText("1.5")}, tenon.Safe, tenon.Constraint{}},
	{"Slice/a list", "Slice", []tenon.Value{tenon.List(str, s("a"), s("b"), s("c")), n(1), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Slice/a tuple", "Slice", []tenon.Value{tenon.Tuple(s("a"), n(1), tenon.Bool(true)), n(1), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Slice/an unknown list, known indexes", "Slice", []tenon.Value{tenon.Unknown(tenon.ListType(str)), n(0), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Slice/refused/start past end", "Slice", []tenon.Value{tenon.List(str, s("a"), s("b")), n(2), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Slice/refused/a set", "Slice", []tenon.Value{tenon.Set(str, s("a")), n(0), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"ReverseList/a list", "ReverseList", []tenon.Value{tenon.List(str, s("a"), s("b"))}, tenon.Safe, tenon.Constraint{}},
	{"ReverseList/a set", "ReverseList", []tenon.Value{tenon.Set(num, n(3), n(1), n(2))}, tenon.Safe, tenon.Constraint{}},
	{"ReverseList/a set holding an unknown", "ReverseList", []tenon.Value{tenon.Set(num, n(1), tenon.Unknown(num))}, tenon.Safe, tenon.Constraint{}},
	{"Concat/lists", "Concat", []tenon.Value{tenon.List(num, n(1)), tenon.List(num, n(2))}, tenon.Safe, tenon.Constraint{}},
	{"Concat/unified under Unsafe", "Concat", []tenon.Value{tenon.List(num, n(1)), tenon.List(str, s("a"))}, tenon.Unsafe, tenon.Constraint{}},
	{"Concat/a tuple under Safe", "Concat", []tenon.Value{tenon.List(num, n(1)), tenon.List(str, s("a"))}, tenon.Safe, tenon.Constraint{}},
	{"Concat/lengths summed", "Concat", []tenon.Value{tenon.Narrow(tenon.Unknown(tenon.ListType(num)), tenon.LengthMin(2), tenon.LengthMax(4)), tenon.List(num, n(1))}, tenon.Safe, tenon.Constraint{}},
	{"Chunklist/pairs", "Chunklist", []tenon.Value{tenon.List(num, n(1), n(2), n(3)), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Chunklist/size zero", "Chunklist", []tenon.Value{tenon.List(num, n(1), n(2)), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Chunklist/the empty tuple", "Chunklist", []tenon.Value{tenon.Tuple(), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Chunklist/refused/a negative size", "Chunklist", []tenon.Value{tenon.List(num, n(1)), n(-1)}, tenon.Safe, tenon.Constraint{}},
	{"Flatten/nested", "Flatten", []tenon.Value{tenon.Tuple(s("a"), tenon.Tuple(s("b"), tenon.List(str, s("c"))))}, tenon.Safe, tenon.Constraint{}},
	{"Flatten/a null sequence a leaf", "Flatten", []tenon.Value{tenon.Tuple(tenon.Null(tenon.ListType(num)), tenon.List(num, n(1)))}, tenon.Safe, tenon.Constraint{}},
	{"Flatten/a nested unknown", "Flatten", []tenon.Value{tenon.Tuple(tenon.List(num, n(1)), tenon.Unknown(tenon.ListType(num)))}, tenon.Safe, tenon.Constraint{}},
	{"Compact/strings", "Compact", []tenon.Value{tenon.List(str, s("a"), s(""), tenon.Null(str), s("b"))}, tenon.Safe, tenon.Constraint{}},
	{"Compact/an unknown member", "Compact", []tenon.Value{tenon.List(str, s("a"), tenon.Unknown(str))}, tenon.Safe, tenon.Constraint{}},
	{"Distinct/numbers", "Distinct", []tenon.Value{tenon.List(num, n(1), n(2), n(1), n(3), n(2))}, tenon.Safe, tenon.Constraint{}},
	{"Distinct/the empty tuple", "Distinct", []tenon.Value{tenon.Tuple()}, tenon.Safe, tenon.Constraint{}},
	{"Distinct/an unknown member", "Distinct", []tenon.Value{tenon.List(num, n(1), tenon.Unknown(num))}, tenon.Safe, tenon.Constraint{}},
	{"CoalesceList/the first not empty", "CoalesceList", []tenon.Value{tenon.List(num), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.List(num, n(1))}, tenon.Safe, tenon.Constraint{}},
	{"CoalesceList/refused/every one empty", "CoalesceList", []tenon.Value{tenon.List(num), tenon.List(num)}, tenon.Safe, tenon.Constraint{}},
	{"Keys/a map", "Keys", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"b": n(2), "a": n(1), "B": n(3)})}, tenon.Safe, tenon.Constraint{}},
	{"Keys/an unknown object", "Keys", []tenon.Value{tenon.Unknown(tenon.ObjectType(map[string]tenon.Type{"b": str, "a": str}))}, tenon.Safe, tenon.Constraint{}},
	{"Values/a map", "Values", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"b": n(2), "a": n(1)})}, tenon.Safe, tenon.Constraint{}},
	{"Values/an object", "Values", []tenon.Value{tenon.Object(map[string]tenon.Value{"b": n(2), "a": s("x")})}, tenon.Safe, tenon.Constraint{}},
	{"Zipmap/a map", "Zipmap", []tenon.Value{tenon.List(str, s("a"), s("b")), tenon.List(num, n(1), n(2))}, tenon.Safe, tenon.Constraint{}},
	{"Zipmap/an object", "Zipmap", []tenon.Value{tenon.List(str, s("a"), s("b")), tenon.Tuple(n(1), s("x"))}, tenon.Safe, tenon.Constraint{}},
	{"Zipmap/the later key wins", "Zipmap", []tenon.Value{tenon.List(str, s("a"), s("a")), tenon.List(num, n(1), n(2))}, tenon.Safe, tenon.Constraint{}},
	{"Zipmap/refused/a null key", "Zipmap", []tenon.Value{tenon.List(str, tenon.Null(str)), tenon.List(num, n(1))}, tenon.Safe, tenon.Constraint{}},
	{"Zipmap/refused/lengths that differ", "Zipmap", []tenon.Value{tenon.List(str, s("a"), tenon.Unknown(str)), tenon.List(num, n(1))}, tenon.Safe, tenon.Constraint{}},
	{"Merge/no arguments", "Merge", nil, tenon.Safe, tenon.Constraint{}},
	{"Merge/maps of one type", "Merge", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"a": n(1)}), tenon.Map(num, map[string]tenon.Value{"a": n(2), "b": n(3)})}, tenon.Safe, tenon.Constraint{}},
	{"Merge/maps of two types", "Merge", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"a": n(1)}), tenon.Map(str, map[string]tenon.Value{"a": s("x")})}, tenon.Safe, tenon.Constraint{}},
	{"Merge/objects", "Merge", []tenon.Value{tenon.Object(map[string]tenon.Value{"a": n(1)}), tenon.Object(map[string]tenon.Value{"a": s("x"), "b": n(2)})}, tenon.Safe, tenon.Constraint{}},
	{"Merge/a null object", "Merge", []tenon.Value{tenon.Null(tenon.ObjectType(map[string]tenon.Type{"a": num})), tenon.Map(num, map[string]tenon.Value{"a": n(1)})}, tenon.Safe, tenon.Constraint{}},
	{"Merge/an untyped null", "Merge", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Map(num, map[string]tenon.Value{"a": n(1)})}, tenon.Safe, tenon.Constraint{}},
	{"Merge/an object not null", "Merge", []tenon.Value{tenon.Object(map[string]tenon.Value{"a": n(1)}), tenon.Narrow(tenon.Unknown(tenon.ObjectType(map[string]tenon.Type{"b": num})), tenon.NotNull())}, tenon.Safe, tenon.Constraint{}},
	{"Merge/an object that may be null", "Merge", []tenon.Value{tenon.Object(map[string]tenon.Value{"a": n(1)}), tenon.Unknown(tenon.ObjectType(map[string]tenon.Type{"b": num}))}, tenon.Safe, tenon.Constraint{}},
	{"Merge/a map not known yet", "Merge", []tenon.Value{tenon.Unknown(tenon.MapType(num)), tenon.Map(num, map[string]tenon.Value{"a": n(1)})}, tenon.Safe, tenon.Constraint{}},
	{"Merge/a map not known yet and an object", "Merge", []tenon.Value{tenon.Object(map[string]tenon.Value{"a": s("x")}), tenon.Unknown(tenon.MapType(num))}, tenon.Safe, tenon.Constraint{}},
	{"Merge/refused/a list", "Merge", []tenon.Value{tenon.List(str, s("a"))}, tenon.Safe, tenon.Constraint{}},
	{"Upper/ASCII", "Upper", []tenon.Value{s("hello")}, tenon.Safe, tenon.Constraint{}},
	{"Upper/full mappings", "Upper", []tenon.Value{s("stra\U000000DFe \U0000FB01sh \U000001F0")}, tenon.Safe, tenon.Constraint{}},
	{"Upper/composed after mapping", "Upper", []tenon.Value{s("i\U00000307")}, tenon.Safe, tenon.Constraint{}},
	{"Upper/a prefix", "Upper", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("v1-ab"))}, tenon.Safe, tenon.Constraint{}},
	{"Lower/final sigma", "Lower", []tenon.Value{s("\U0000039F\U00000394\U0000039F\U000003A3 \U000003A3")}, tenon.Safe, tenon.Constraint{}},
	{"Lower/capital I with dot above", "Lower", []tenon.Value{s("\U00000130")}, tenon.Safe, tenon.Constraint{}},
	{"Lower/a prefix ending in a sigma", "Lower", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ABC\U000003A3"))}, tenon.Safe, tenon.Constraint{}},
	{"Lower/a prefix settling a sigma", "Lower", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ABC\U000003A3 X"))}, tenon.Safe, tenon.Constraint{}},
	{"Title/separators", "Title", []tenon.Value{s("foo.example.com o'neil hello_world hELLO")}, tenon.Safe, tenon.Constraint{}},
	{"Title/full titlecase", "Title", []tenon.Value{s("\U000000DFtra\U000000DFe \U000001C6emal")}, tenon.Safe, tenon.Constraint{}},
	{"Title/punctuation outside ASCII", "Title", []tenon.Value{s("\U000000ABbonjour\U000000BB \U0000201Chi\U0000201D")}, tenon.Safe, tenon.Constraint{}},
	{"Strlen/clusters", "Strlen", []tenon.Value{s("q\U00000301 \r\n \U0001F1FA\U0001F1F8\U0001F1EC")}, tenon.Safe, tenon.Constraint{}},
	{"Strlen/a ZWJ after a regional indicator", "Strlen", []tenon.Value{s("\U0001F1FA\U0000200D\U0001F468")}, tenon.Safe, tenon.Constraint{}},
	{"Strlen/not known yet", "Strlen", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab-"), tenon.LengthMax(5))}, tenon.Safe, tenon.Constraint{}},
	{"Reverse/clusters", "Reverse", []tenon.Value{s("e\U00000301x a\r\nb \U0001F1FA\U0001F1F8")}, tenon.Safe, tenon.Constraint{}},
	{"Reverse/composing anew", "Reverse", []tenon.Value{s("\U00000301e")}, tenon.Safe, tenon.Constraint{}},
	{"Substr/from the start", "Substr", []tenon.Value{s("hello"), n(1), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Substr/from the end", "Substr", []tenon.Value{s("hello"), n(-3), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Substr/a length of zero", "Substr", []tenon.Value{s("hello"), n(-3), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Substr/the rest", "Substr", []tenon.Value{s("hello"), n(1), n(-1)}, tenon.Safe, tenon.Constraint{}},
	{"Substr/any magnitude", "Substr", []tenon.Value{s("hello"), tenon.NumberFromText("-1e30"), tenon.NumberFromText("1e30")}, tenon.Safe, tenon.Constraint{}},
	{"Substr/a prefix", "Substr", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("hello world")), n(0), n(5)}, tenon.Safe, tenon.Constraint{}},
	{"Substr/refused/a fraction", "Substr", []tenon.Value{s("hello"), tenon.NumberFromText("1.5"), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Split/leftmost, without overlap", "Split", []tenon.Value{s("aa"), s("aaaaa")}, tenon.Safe, tenon.Constraint{}},
	{"Split/inside CR LF", "Split", []tenon.Value{s("\n"), s("a\r\nb\r\n")}, tenon.Safe, tenon.Constraint{}},
	{"Split/no cluster split", "Split", []tenon.Value{s("\U0001F1F8\U0001F1EC"), s("\U0001F1FA\U0001F1F8\U0001F1EC\U0001F1E7")}, tenon.Safe, tenon.Constraint{}},
	{"Split/an empty separator", "Split", []tenon.Value{s(""), s("a\r\nb\U00000915\U0000094D\U00000937")}, tenon.Safe, tenon.Constraint{}},
	{"Split/the empty string", "Split", []tenon.Value{s(","), s("")}, tenon.Safe, tenon.Constraint{}},
	{"Split/a prefix", "Split", []tenon.Value{s(","), tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("a,b,c,d"))}, tenon.Safe, tenon.Constraint{}},
	{"Replace/leftmost", "Replace", []tenon.Value{s("aaa"), s("aa"), s("b")}, tenon.Safe, tenon.Constraint{}},
	{"Replace/an empty search", "Replace", []tenon.Value{s("ab\U00000915\U0000094D"), s(""), s("-")}, tenon.Safe, tenon.Constraint{}},
	{"Replace/composing", "Replace", []tenon.Value{s("e-y"), s("-"), s("\U00000301")}, tenon.Safe, tenon.Constraint{}},
	{"Replace/no cluster split", "Replace", []tenon.Value{s("q\U00000301"), s("q"), s("e")}, tenon.Safe, tenon.Constraint{}},
	{"Replace/a prefix", "Replace", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("a-b-c-d-e")), s("-"), s("+")}, tenon.Safe, tenon.Constraint{}},
	{"Replace/refused/past the bound", "Replace", []tenon.Value{s(strings.Repeat("a", 1000)), s("a"), s(strings.Repeat("x", 1000))}, tenon.Safe, tenon.Constraint{}},
	{"TrimPrefix/once", "TrimPrefix", []tenon.Value{s("aaa"), s("a")}, tenon.Safe, tenon.Constraint{}},
	{"TrimPrefix/no cluster split", "TrimPrefix", []tenon.Value{s("\U0001F1FA\U0001F1F8\U0001F1EC\U0001F1E7"), s("\U0001F1FA")}, tenon.Safe, tenon.Constraint{}},
	{"TrimPrefix/a prefix", "TrimPrefix", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("v1-alpha")), s("v1-")}, tenon.Safe, tenon.Constraint{}},
	{"TrimSuffix/inside CR LF", "TrimSuffix", []tenon.Value{s("a\r\n"), s("\n")}, tenon.Safe, tenon.Constraint{}},
	{"TrimSuffix/no cluster split", "TrimSuffix", []tenon.Value{s("xq\U00000301"), s("\U00000301")}, tenon.Safe, tenon.Constraint{}},
	{"TrimSuffix/a prefix", "TrimSuffix", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("hello world")), s(".txt")}, tenon.Safe, tenon.Constraint{}},
	{"Trim/a set of code points", "Trim", []tenon.Value{s("xyhixy"), s("yx")}, tenon.Safe, tenon.Constraint{}},
	{"Trim/no cluster split", "Trim", []tenon.Value{s("q\U00000301xq"), s("q")}, tenon.Safe, tenon.Constraint{}},
	{"Trim/a prefix", "Trim", []tenon.Value{tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("xxabc def")), s("x ")}, tenon.Safe, tenon.Constraint{}},
	{"TrimSpace/White_Space", "TrimSpace", []tenon.Value{s("\t\r\n x \U000000A0\U00003000")}, tenon.Safe, tenon.Constraint{}},
	{"TrimSpace/a space carrying a mark", "TrimSpace", []tenon.Value{s(" \U00000301x")}, tenon.Safe, tenon.Constraint{}},
	{"Chomp/CRs and LFs", "Chomp", []tenon.Value{s("a\r\n\n\r")}, tenon.Safe, tenon.Constraint{}},
	{"Chomp/other terminators stay", "Chomp", []tenon.Value{s("a\U00002028")}, tenon.Safe, tenon.Constraint{}},
	{"Indent/the last line too", "Indent", []tenon.Value{n(2), s("a\nb\n")}, tenon.Safe, tenon.Constraint{}},
	{"Indent/no LF", "Indent", []tenon.Value{tenon.NumberFromText("1e30"), s("ab")}, tenon.Safe, tenon.Constraint{}},
	{"Indent/a prefix", "Indent", []tenon.Value{tenon.Unknown(num), tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("a\nb\nc"))}, tenon.Safe, tenon.Constraint{}},
	{"Indent/refused/negative", "Indent", []tenon.Value{n(-1), s("ab")}, tenon.Safe, tenon.Constraint{}},
	{"Indent/refused/past the bound", "Indent", []tenon.Value{tenon.NumberFromText("4611686018427387904"), s("a\nb")}, tenon.Safe, tenon.Constraint{}},
	{"Range/a limit", "Range", []tenon.Value{n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Range/a negative limit", "Range", []tenon.Value{n(-3)}, tenon.Safe, tenon.Constraint{}},
	{"Range/down by three", "Range", []tenon.Value{n(10), n(1), n(-3)}, tenon.Safe, tenon.Constraint{}},
	{"Range/tenths exactly", "Range", []tenon.Value{n(0), n(1), tenon.NumberFromText("0.1")}, tenon.Safe, tenon.Constraint{}},
	{"Range/hundredths exactly", "Range", []tenon.Value{n(0), tenon.NumberFromText("0.05"), tenon.NumberFromText("0.01")}, tenon.Safe, tenon.Constraint{}},
	{"Range/the bound", "Range", []tenon.Value{n(-1024)}, tenon.Safe, tenon.Constraint{}},
	{"Range/not known yet", "Range", []tenon.Value{tenon.Unknown(num), n(3)}, tenon.Safe, tenon.Constraint{}},
	{"Range/refused/a step of zero", "Range", []tenon.Value{n(5), n(5), tenon.NumberFromText("0.0")}, tenon.Safe, tenon.Constraint{}},
	{"Range/refused/a step away from the limit", "Range", []tenon.Value{n(0), n(3), n(-1)}, tenon.Safe, tenon.Constraint{}},
	{"Range/refused/past the bound", "Range", []tenon.Value{n(0), n(1025)}, tenon.Safe, tenon.Constraint{}},
	{"Range/refused/a step of zero not known yet", "Range", []tenon.Value{tenon.Unknown(num), n(3), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"SetProduct/lists", "SetProduct", []tenon.Value{tenon.List(str, s("a"), s("b")), tenon.List(num, n(1), n(2))}, tenon.Safe, tenon.Constraint{}},
	{"SetProduct/a set", "SetProduct", []tenon.Value{tenon.Set(str, s("a"), s("b")), tenon.List(num, n(1))}, tenon.Safe, tenon.Constraint{}},
	{"SetProduct/the empty tuple", "SetProduct", []tenon.Value{tenon.Tuple(), tenon.List(num, n(1))}, tenon.Safe, tenon.Constraint{}},
	{"SetProduct/a tuple under Unsafe", "SetProduct", []tenon.Value{tenon.Tuple(n(1), s("a")), tenon.List(tenon.BoolType(), tenon.Bool(true))}, tenon.Unsafe, tenon.Constraint{}},
	{"SetProduct/not known yet", "SetProduct", []tenon.Value{tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.NotNull(), tenon.LengthMax(3)), tenon.List(num, n(1), n(2))}, tenon.Safe, tenon.Constraint{}},
	{"SetProduct/refused/one argument", "SetProduct", []tenon.Value{tenon.List(str, s("a"))}, tenon.Safe, tenon.Constraint{}},
	{"SetProduct/refused/past the bound", "SetProduct", []tenon.Value{tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.NotNull(), tenon.LengthMin(2000)), tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.NotNull(), tenon.LengthMin(1000))}, tenon.Safe, tenon.Constraint{}},
	{"Contains/found", "Contains", []tenon.Value{tenon.List(num, n(1), n(2)), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Contains/not found", "Contains", []tenon.Value{tenon.List(num, n(1), n(2)), s("1")}, tenon.Safe, tenon.Constraint{}},
	{"Contains/a member not known yet", "Contains", []tenon.Value{tenon.List(num, n(1), tenon.Unknown(num)), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Contains/the empty tuple", "Contains", []tenon.Value{tenon.Tuple(), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe, tenon.Constraint{}},
	{"Contains/an untyped null among nulls", "Contains", []tenon.Value{tenon.List(str, s("a"), tenon.Null(str)), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe, tenon.Constraint{}},
	{"Contains/a tuple among lists", "Contains", []tenon.Value{tenon.List(tenon.ListType(num), tenon.List(num, n(1))), tenon.Tuple(n(1))}, tenon.Safe, tenon.Constraint{}},
	{"Contains/refused/a map", "Contains", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"a": n(1)}), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"SetHasElement/found", "SetHasElement", []tenon.Value{tenon.Set(num, n(1), n(2)), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"SetHasElement/a null", "SetHasElement", []tenon.Value{tenon.Set(num, tenon.Null(num)), tenon.Null(num)}, tenon.Safe, tenon.Constraint{}},
	{"SetHasElement/a list under Unsafe", "SetHasElement", []tenon.Value{tenon.List(num, n(1)), n(1)}, tenon.Unsafe, tenon.Constraint{}},
	{"SetHasElement/refused/a list under Safe", "SetHasElement", []tenon.Value{tenon.List(num, n(1)), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"SetUnion/two sets", "SetUnion", []tenon.Value{tenon.Set(num, n(1), n(2)), tenon.Set(num, n(2), n(3))}, tenon.Safe, tenon.Constraint{}},
	{"SetUnion/unified under Unsafe", "SetUnion", []tenon.Value{tenon.Set(num, n(1)), tenon.Set(str, s("a"))}, tenon.Unsafe, tenon.Constraint{}},
	{"SetUnion/the empty tuple", "SetUnion", []tenon.Value{tenon.Tuple(), tenon.Set(num, n(1))}, tenon.Safe, tenon.Constraint{}},
	{"SetUnion/a set not known yet", "SetUnion", []tenon.Value{tenon.Narrow(tenon.Unknown(tenon.SetType(num)), tenon.LengthMax(2)), tenon.Set(num, n(1))}, tenon.Safe, tenon.Constraint{}},
	{"SetUnion/refused/not unified under Safe", "SetUnion", []tenon.Value{tenon.Set(num, n(1)), tenon.Set(str, s("a"))}, tenon.Safe, tenon.Constraint{}},
	{"SetUnion/refused/only empty tuples", "SetUnion", []tenon.Value{tenon.Tuple(), tenon.Tuple()}, tenon.Safe, tenon.Constraint{}},
	{"SetIntersection/two sets", "SetIntersection", []tenon.Value{tenon.Set(num, n(1), n(2)), tenon.Set(num, n(2), n(3))}, tenon.Safe, tenon.Constraint{}},
	{"SetIntersection/a member not known yet", "SetIntersection", []tenon.Value{tenon.Set(num, n(1), n(2)), tenon.Set(num, n(1), tenon.Unknown(num))}, tenon.Safe, tenon.Constraint{}},
	{"SetSubtract/two sets", "SetSubtract", []tenon.Value{tenon.Set(num, n(1), n(2)), tenon.Set(num, n(2), n(3))}, tenon.Safe, tenon.Constraint{}},
	{"SetSubtract/the empty set", "SetSubtract", []tenon.Value{tenon.Set(num, n(1), tenon.Unknown(num)), tenon.Set(num)}, tenon.Safe, tenon.Constraint{}},
	{"SetSymmetricDifference/three sets", "SetSymmetricDifference", []tenon.Value{tenon.Set(num, n(1), n(2)), tenon.Set(num, n(2), n(3)), tenon.Set(num, n(3), n(4))}, tenon.Safe, tenon.Constraint{}},
	{"SetSymmetricDifference/a member not known yet", "SetSymmetricDifference", []tenon.Value{tenon.Set(num, n(1), tenon.Unknown(num)), tenon.Set(num, n(2))}, tenon.Safe, tenon.Constraint{}},
	{"Lookup/found", "Lookup", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"a": n(1)}), s("a"), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Lookup/the default", "Lookup", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"a": n(1)}), s("c"), n(0)}, tenon.Safe, tenon.Constraint{}},
	{"Lookup/no default needed", "Lookup", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"a": n(1)}), s("a")}, tenon.Safe, tenon.Constraint{}},
	{"Lookup/a null default", "Lookup", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"a": n(1)}), s("c"), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe, tenon.Constraint{}},
	{"Lookup/refused/missing, no default", "Lookup", []tenon.Value{tenon.Map(num, map[string]tenon.Value{"a": n(1)}), s("c")}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/the first not null", "Coalesce", []tenon.Value{tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), tenon.Null(num), n(1), n(2)}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/one not known yet", "Coalesce", []tenon.Value{tenon.Unknown(num), n(1)}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/unified under Unsafe", "Coalesce", []tenon.Value{n(1), s("a")}, tenon.Unsafe, tenon.Constraint{}},
	{"Coalesce/marks of what is read", "Coalesce", []tenon.Value{tenon.WithMarks(tenon.Null(num), plain), n(1), tenon.WithMarks(n(2), secret{})}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/refused/every argument null", "Coalesce", []tenon.Value{tenon.Null(str), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())}, tenon.Safe, tenon.Constraint{}},
	{"Coalesce/refused/not unified under Safe", "Coalesce", []tenon.Value{n(1), s("a")}, tenon.Safe, tenon.Constraint{}},
	{"MakeTo/a string to a number", "MakeTo", []tenon.Value{s("5")}, tenon.Safe, tenon.Exactly(num)},
	{"MakeTo/a tuple to a list", "MakeTo", []tenon.Value{tenon.Tuple(n(1), s("a"))}, tenon.Safe, tenon.ListOf(tenon.Any())},
	{"MakeTo/null", "MakeTo", []tenon.Value{tenon.Null(str)}, tenon.Safe, tenon.Exactly(num)},
	{"MakeTo/refused/not a number", "MakeTo", []tenon.Value{s("inf")}, tenon.Safe, tenon.Exactly(num)},
	{"MakeTo/refused/a member", "MakeTo", []tenon.Value{tenon.Tuple(s("1"), s("x"))}, tenon.Safe, tenon.ListOf(tenon.Exactly(num))},
}

// callFile is the layout of functions.json.
type callFile struct {
	Format int       `json:"format"`
	About  string    `json:"about"`
	Calls  []callOut `json:"calls"`
}

type callOut struct {
	Name          string        `json:"name"`
	Function      string        `json:"function"`
	Constraint    string        `json:"constraint,omitempty"`
	ConstraintHex string        `json:"constraintHex,omitempty"`
	Args          []callArg     `json:"args"`
	Policy        string        `json:"policy"`
	Value         string        `json:"value,omitempty"`
	ValueHex      string        `json:"valueHex,omitempty"`
	Failures      []jsonFailure `json:"failures,omitempty"`
}

type callArg struct {
	Value string `json:"value"`
	Hex   string `json:"hex"`
}

// TestConformance_LB001_FunctionVectors writes functions.json: each call of a
// library function, its arguments' display forms and encodings, and what it
// answers, its display form and its encoding, or the failures it gives, each
// code with its path.
func TestConformance_LB001_FunctionVectors(t *testing.T) {
	conformance.Covers(t, "LB-001", "LN-001", "LN-002", "LN-010", "LN-011", "LN-020", "LN-030", "LN-031", "LN-032", "LN-033", "LN-050", "LN-060", "LN-061", "LN-080", "LN-083", "LN-085", "LC-001", "LC-002", "LC-003", "LC-004", "LC-010", "LC-011", "LC-012", "LC-013", "LC-020", "LC-021", "LC-022", "LC-023", "LC-030", "LC-031", "LC-032", "LC-033")
	f := callFile{
		Format: 1,
		About: "Each entry is a call of a function of the standard library (§13 to §20), named as the specification " +
			"names it, with arguments, each given as its display form and its encoding (args), under a policy. A call " +
			"that answers gives value, its display form, and valueHex, its encoding; one that fails gives the " +
			"failures, each a code and the display form of its path, in order. The marks the arguments carry are " +
			"those vectors.json lists. A call of a function MakeTo makes gives the constraint it was made for, as " +
			"its display form and as the encoding of the pending value of it (constraint, constraintHex).",
	}
	names := map[string]bool{}
	called := map[string]bool{}
	for _, vec := range callVectors {
		if names[vec.name] {
			t.Fatalf("two vectors are named %q", vec.name)
		}
		names[vec.name] = true
		fn, ok := library[vec.function]
		if !ok || !strings.HasPrefix(vec.name, vec.function+"/") {
			t.Fatalf("%s: no library function %q, or the name does not begin with it", vec.name, vec.function)
		}
		called[vec.function] = true
		out := callOut{Name: vec.name, Function: vec.function, Policy: vec.policy.String()}
		if vec.function == "MakeTo" {
			fn = stdlib.MakeToFunc(vec.to)
			pending, err := tenon.Serialize(tenon.Pending(vec.to))
			if err != nil {
				t.Fatalf("%s: the constraint %v does not encode: %v", vec.name, vec.to, err)
			}
			out.Constraint, out.ConstraintHex = vec.to.String(), hex.EncodeToString(pending)
		}
		for _, a := range vec.args {
			b, err := tenon.Serialize(a)
			if err != nil {
				t.Fatalf("%s: the argument %v does not encode: %v", vec.name, a, err)
			}
			out.Args = append(out.Args, callArg{Value: a.String(), Hex: hex.EncodeToString(b)})
		}
		r := tenon.Call(fn, vec.args, vec.policy)
		if r.IsError() {
			for _, d := range r.Diagnostics() {
				out.Failures = append(out.Failures, jsonFailure{Code: string(d.Code), Path: d.Path.String()})
			}
		} else {
			b, err := tenon.Serialize(r)
			if err != nil {
				t.Fatalf("%s: %v does not encode: %v", vec.name, r, err)
			}
			out.Value, out.ValueHex = r.String(), hex.EncodeToString(b)
		}
		if strings.Contains(vec.name, "/refused/") != (out.Failures != nil) {
			t.Errorf("%s: %v; the name says whether it is refused", vec.name, r)
		}
		f.Calls = append(f.Calls, out)
	}
	for name := range library {
		if !called[name] {
			t.Errorf("no vector calls %s", name)
		}
	}

	var b bytes.Buffer
	b.WriteString("{\n")
	b.WriteString(`  "format": ` + jsonLine(t, f.Format) + ",\n")
	b.WriteString(`  "about": ` + jsonLine(t, f.About) + ",\n")
	b.WriteString(`  "calls": [` + "\n")
	for i, out := range f.Calls {
		b.WriteString("    " + jsonLine(t, out))
		if i < len(f.Calls)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n}\n")
	want := b.Bytes()
	conformance.Emit(t, "functions.json", want)
	if os.Getenv("TENON_UPDATE_VECTORS") == "1" {
		if err := os.WriteFile("functions.json", want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile("functions.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v; set TENON_UPDATE_VECTORS=1 to write it", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("functions.json is not what the vectors give; if the change is deliberate, set TENON_UPDATE_VECTORS=1 and review the diff")
	}
}
