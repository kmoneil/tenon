// Package values builds the values that conformance property tests run over:
// one of every shape the value system can hold. A property asserted over these
// is asserted over error values, pending values, unknown values, marked values
// and the containers that hold them, rather than only over the values that are
// easy to write down.
//
// It is a package beside conformance rather than part of it because it imports
// tenon, and tenon's own internal tests import conformance.
package values

import "github.com/kmoneil/tenon"

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

// Colliding is a capsule type that declares equality and a hash that is the
// same for every value, which a hash is allowed to be, and no order. Two of
// its values are equal exactly when they encapsulate equal points, and nothing
// it declares tells two unequal ones apart, so the canonical order falls back
// to numbering them.
var Colliding = tenon.Capsule("colliding", tenon.CapsuleOps[point]{
	Equals: func(a, b *point) bool { return *a == *b },
	Hash:   func(*point) uint64 { return 7 },
})

var shared = &point{1, 2}

// label is a mark the generator attaches. Marks are told apart by Go equality,
// so two labels with one name are one mark.
type label string

func (m label) MarkID() string               { return string(m) }
func (label) Propagation() tenon.Propagation { return tenon.Propagate }
func (label) Redacting() bool                { return false }

// Two marks, so that values can differ by which mark they carry as well as by
// whether they carry one.
var origin, audit tenon.Mark = label("origin"), label("audit")

// layer is a deep mark: attached to a value, it is attached to every value
// within it too.
type layer string

func (m layer) MarkID() string               { return string(m) }
func (layer) Propagation() tenon.Propagation { return tenon.Propagate }
func (layer) Redacting() bool                { return false }
func (layer) Deep() bool                     { return true }

var sealed tenon.Mark = layer("sealed")

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
		tenon.Unknown(tenon.Set(str)),
		tenon.Narrow(tenon.Unknown(tenon.Set(str)), tenon.Members(s("a"))),
		tenon.Narrow(tenon.Unknown(tenon.Set(str)), tenon.Members(s("a")), tenon.Members(s("b"))),
		tenon.Narrow(tenon.Unknown(tenon.Set(str)), tenon.Members(s("b"), s("a"))),
		tenon.Narrow(tenon.Unknown(tenon.Set(str)), tenon.Members(unknownStr)),
		tenon.Narrow(tenon.Unknown(tenon.Set(str)), tenon.LengthMin(1)),
		tenon.Narrow(tenon.Unknown(tenon.Set(str)),
			tenon.Members(tenon.Narrow(unknownStr, tenon.StringPrefix("ab-")))),

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
		tenon.CapsuleVal(Colliding, shared),
		tenon.CapsuleVal(Colliding, &point{1, 2}),
		tenon.CapsuleVal(Colliding, &point{3, 4}),

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
		tenon.SetVal(str, tenon.NullVal(str)),
		tenon.SetVal(str, unknownStr),
		tenon.SetVal(str, s("a"), unknownStr),
		tenon.SetVal(str, unknownStr, s("a")),
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

		// Marked values, in every state. Two are one value reached two ways: a
		// number written two ways under one mark, and two marks attached in
		// either order.
		tenon.WithMarks(n(1), origin),
		tenon.WithMarks(tenon.NumberFromText("1.000"), origin),
		tenon.WithMarks(n(1), audit),
		tenon.WithMarks(s("a"), origin, audit),
		tenon.WithMarks(tenon.WithMarks(s("a"), audit), origin),
		tenon.WithMarks(tenon.NullVal(num), origin),
		tenon.WithMarks(unknownNum, origin),
		tenon.WithMarks(tenon.Pending(tenon.Any()), origin),
		tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}), origin),
		tenon.WithMarks(tenon.ListVal(str, s("a")), origin),
		tenon.WithMarks(tenon.SetVal(str, s("a"), s("b")), origin),

		// Containers holding a marked member, which are marked although they
		// carry no mark themselves, at one depth and at two, and one whose
		// marked member is not known.
		tenon.ListVal(num, tenon.WithMarks(n(1), origin)),
		tenon.ListVal(num, tenon.WithMarks(tenon.NumberFromText("1.0"), origin)),
		tenon.ListVal(tenon.List(num), tenon.ListVal(num, n(2), tenon.WithMarks(n(1), audit))),
		tenon.MapVal(num, map[string]tenon.Value{"k": tenon.WithMarks(n(1), origin)}),
		tenon.TupleVal(tenon.WithMarks(unknownStr, audit)),
		tenon.ObjectVal(map[string]tenon.Value{"a": tenon.WithMarks(n(1), origin), "b": s("x")}),

		// Deep-marked values, whose mark is on everything within them but the
		// members of a set, which get it when they are read. Two are one value
		// reached two ways: a list marked whole, and the same list marked whole
		// after one of its elements was marked alone.
		tenon.WithMarks(tenon.ListVal(tenon.List(num), tenon.ListVal(num, n(1), n(2))), sealed),
		tenon.WithMarks(tenon.ListVal(tenon.List(num), tenon.ListVal(num, tenon.WithMarks(n(1), sealed), n(2))), sealed),
		tenon.WithMarks(tenon.SetVal(str, s("a"), s("b")), sealed),
		tenon.WithMarks(tenon.MapVal(num, map[string]tenon.Value{"k": n(1)}), sealed),
		tenon.WithMarks(tenon.ObjectVal(map[string]tenon.Value{
			"a": tenon.SetVal(str, s("a")),
			"b": unknownNum,
			"c": tenon.WithMarks(n(1), origin),
		}), sealed),
	}
}

// Orderable returns the values of All that hashing and the canonical order are
// defined over: the known ones that are not marked, carrying no mark and
// holding none.
func Orderable() []tenon.Value {
	var out []tenon.Value
	for _, v := range All() {
		if _, marks := tenon.UnmarkDeep(v); v.IsKnown() && len(marks) == 0 {
			out = append(out, v)
		}
	}
	return out
}
