// Package values builds the values that conformance property tests run over:
// one of every shape the value system can hold. A property asserted over these
// is asserted over error values, pending values, unknown values and the
// containers that hold them, rather than only over the values that are easy to
// write down.
//
// It is a package beside conformance rather than part of it because it imports
// tenon, and tenon's own internal tests import conformance.
package values

import "tenon"

type point struct{ x, y int }

// Opaque is a capsule type that declares nothing, so its values are equal only
// when they encapsulate one pointer.
var Opaque = tenon.Capsule("opaque", tenon.CapsuleOps[point]{})

// Compared is a capsule type that declares equality, so its values are equal
// when they encapsulate points that are equal.
var Compared = tenon.Capsule("compared", tenon.CapsuleOps[point]{
	Equals: func(a, b *point) bool { return *a == *b },
	Hash:   func(v *point) uint64 { return uint64(v.x)<<32 | uint64(v.y) },
})

var shared = &point{1, 2}

// All returns one value of every shape, in a fixed order. Some of them are the
// same value reached two ways, such as a number written with and without a
// trailing zero, or a set given its members in either order: a comparison that
// walked representations rather than values would tell those apart, and a
// property test over this list would catch it.
func All() []tenon.Value {
	bl, num, str := tenon.BoolType(), tenon.NumberType(), tenon.StringType()
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	s := func(text string) tenon.Value { return tenon.String(text) }
	unknownNum := tenon.Unknown(num)
	unknownStr := tenon.Unknown(str)
	return []tenon.Value{
		// Error values, which are their diagnostics and nothing else.
		tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}),
		tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed differently"}),
		tenon.ErrorVal(tenon.Diagnostic{Code: "app.other", Message: "it failed"}),
		tenon.ErrorVal(
			tenon.Diagnostic{Code: "app.failed", Message: "it failed"},
			tenon.Diagnostic{Code: "app.other", Message: "and again"},
		),
		tenon.ErrorVal(tenon.Diagnostic{
			Code: "app.failed", Message: "it failed", Path: tenon.Path{}.Attribute("a"),
		}),
		s("\xff"),

		// Pending values, which are a constraint and what is known about null.
		tenon.Pending(tenon.Any()),
		tenon.Pending(tenon.Exactly(str)),
		tenon.Pending(tenon.ListOf(tenon.Exactly(num))),
		tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null()),
		tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull()),

		// Null values, one per kind of type they belong to.
		tenon.NullVal(bl),
		tenon.NullVal(num),
		tenon.NullVal(str),
		tenon.NullVal(tenon.List(str)),
		tenon.NullVal(tenon.Tuple()),

		// Unknown values, from the widest to the narrowly bounded.
		tenon.Unknown(bl),
		unknownNum,
		unknownStr,
		tenon.Unknown(tenon.List(str)),
		tenon.Unknown(tenon.Map(num)),
		tenon.Narrow(unknownNum, tenon.NotNull()),
		tenon.Narrow(unknownNum, tenon.NumberMin(n(1), true)),
		tenon.Narrow(unknownNum, tenon.NumberMin(n(1), false)),
		tenon.Narrow(unknownNum, tenon.NumberMax(n(10), true)),
		tenon.Narrow(unknownNum, tenon.NumberMin(n(1), true), tenon.NumberMax(n(10), true)),
		tenon.Narrow(unknownStr, tenon.StringPrefix("ab-")),
		tenon.Narrow(unknownStr, tenon.LengthMin(2)),
		tenon.Narrow(unknownStr, tenon.LengthMax(5)),
		tenon.Narrow(tenon.Unknown(tenon.List(str)), tenon.LengthMin(1)),

		// Known scalars, including two ways of writing one value.
		tenon.Bool(true),
		tenon.Bool(false),
		n(0),
		n(1),
		tenon.NumberFromText("1.000"),
		n(-1),
		tenon.NumberFromText("1e100"),
		s(""),
		s("a"),
		s("ab"),
		s("e\U00000301"),
		s("\U000000e9"),
		tenon.CapsuleVal(Opaque, shared),
		tenon.CapsuleVal(Opaque, &point{1, 2}),
		tenon.CapsuleVal(Compared, shared),
		tenon.CapsuleVal(Compared, &point{1, 2}),

		// Collections, including ones holding a member that is not known.
		tenon.ListVal(str),
		tenon.ListVal(str, s("a")),
		tenon.ListVal(str, s("a"), s("b")),
		tenon.ListVal(str, tenon.NullVal(str)),
		tenon.ListVal(str, unknownStr),
		tenon.SetVal(str),
		tenon.SetVal(str, s("a")),
		tenon.SetVal(str, s("a"), s("b")),
		tenon.SetVal(str, s("b"), s("a")),
		tenon.SetVal(str, s("a"), s("a")),
		tenon.MapVal(num, nil),
		tenon.MapVal(num, map[string]tenon.Value{"k": n(1)}),
		tenon.MapVal(num, map[string]tenon.Value{"k": n(1), "j": n(2)}),
		tenon.MapVal(num, map[string]tenon.Value{"k": unknownNum}),

		// Structural values, and a nesting of both sorts.
		tenon.TupleVal(),
		tenon.TupleVal(tenon.Bool(true)),
		tenon.TupleVal(tenon.Bool(true), s("x")),
		tenon.TupleVal(unknownStr),
		tenon.ObjectVal(nil),
		tenon.ObjectVal(map[string]tenon.Value{"a": n(1)}),
		tenon.ObjectVal(map[string]tenon.Value{"a": n(1), "b": s("x")}),
		tenon.ObjectVal(map[string]tenon.Value{"a": unknownNum}),
		tenon.ListVal(tenon.List(str), tenon.ListVal(str, s("a"))),
		tenon.ObjectVal(map[string]tenon.Value{"a": tenon.ListVal(str, s("a"))}),
	}
}

// Known returns the values of All that are known: the ones whose range holds
// one value, which is what hashing and canonical order are defined over.
func Known() []tenon.Value {
	var known []tenon.Value
	for _, v := range All() {
		if v.IsKnown() {
			known = append(known, v)
		}
	}
	return known
}
