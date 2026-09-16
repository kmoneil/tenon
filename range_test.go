package tenon_test

import (
	"strconv"
	"strings"
	"testing"

	"tenon"
	"tenon/conformance"
)

func TestConformance_VA002_RangeOfAResolvedValue(t *testing.T) {
	conformance.Covers(t, "VA-002")
	str, num := tenon.StringType(), tenon.NumberType()
	for _, v := range []tenon.Value{
		tenon.String("text"),
		tenon.NumberFromInt(3),
		tenon.NullVal(str),
		tenon.Unknown(str),
		tenon.Unknown(tenon.List(num)),
	} {
		if !v.IsResolved() {
			t.Fatalf("%v is not a resolved value", v)
		}
		if got := v.Range().Type(); got != v.Type() {
			t.Errorf("%v: range of type %v, want %v", v, got, v.Type())
		}
	}
	// The range of a resolved value is a subset of its domain and null, so
	// null is in it exactly when the value may be null.
	for _, tt := range []struct {
		v    tenon.Value
		null bool
	}{
		{tenon.NullVal(str), true},
		{tenon.Unknown(str), true},
		{tenon.Narrow(tenon.Unknown(str), tenon.NotNull()), false},
		{tenon.String("text"), false},
	} {
		if got := tt.v.Range().AllowsNull(); got != tt.null {
			t.Errorf("%v: AllowsNull is %t, want %t", tt.v, got, tt.null)
		}
	}
	// A value with no type has no range.
	mustPanicUsage(t, "Range called on an error value", func() { tenon.String("\xff").Range() })
	mustPanicUsage(t, "Range called on a pending value", func() { tenon.Pending(tenon.Any()).Range() })
}

func TestConformance_UN001_RangesAreNeverEmpty(t *testing.T) {
	conformance.Covers(t, "UN-001")
	num := tenon.NumberType()
	// A fresh unknown describes the whole domain of its type, and null.
	if got := tenon.Unknown(num).String(); got != "unknown(number)" {
		t.Errorf("a fresh unknown is %s, want unknown(number)", got)
	}
	// Narrowing to nothing produces an error value: no value has an empty
	// range, because a value whose range is empty could not exist.
	empty := tenon.Narrow(tenon.Unknown(num),
		tenon.NumberMin(tenon.NumberFromInt(5), true),
		tenon.NumberMax(tenon.NumberFromInt(3), true))
	if empty.IsResolved() {
		t.Errorf("narrowing to nothing produced the resolved value %v", empty)
	}
	if !empty.IsError() {
		t.Errorf("narrowing to nothing produced %v, want an error value", empty)
	}
}

func TestConformance_UN002_NarrowingTable(t *testing.T) {
	conformance.Covers(t, "UN-002")
	five := tenon.NumberFromInt(5)
	str, num := tenon.StringType(), tenon.NumberType()
	lst, set, mp := tenon.List(str), tenon.Set(str), tenon.Map(str)
	for _, tt := range []struct {
		name string
		v    tenon.Value
		ns   []tenon.Narrowing
		want string
	}{
		{"NotNull", tenon.Unknown(num), []tenon.Narrowing{tenon.NotNull()}, "unknown(number, not null)"},
		{"NumberMin inclusive", tenon.Unknown(num), []tenon.Narrowing{tenon.NumberMin(five, true)}, "unknown(number, >= 5)"},
		{"NumberMin exclusive", tenon.Unknown(num), []tenon.Narrowing{tenon.NumberMin(five, false)}, "unknown(number, > 5)"},
		{"NumberMax inclusive", tenon.Unknown(num), []tenon.Narrowing{tenon.NumberMax(five, true)}, "unknown(number, <= 5)"},
		{"NumberMax exclusive", tenon.Unknown(num), []tenon.Narrowing{tenon.NumberMax(five, false)}, "unknown(number, < 5)"},
		{"StringPrefix", tenon.Unknown(str), []tenon.Narrowing{tenon.StringPrefix("ab-")}, `unknown(string, prefix "ab-", length >= 3)`},
		{"LengthMin on a list", tenon.Unknown(lst), []tenon.Narrowing{tenon.LengthMin(2)}, "unknown(list(string), length >= 2)"},
		{"LengthMax on a set", tenon.Unknown(set), []tenon.Narrowing{tenon.LengthMax(3)}, "unknown(set(string), length <= 3)"},
		{"LengthMin on a map", tenon.Unknown(mp), []tenon.Narrowing{tenon.LengthMin(1)}, "unknown(map(string), length >= 1)"},
		{"LengthMax on a string", tenon.Unknown(str), []tenon.Narrowing{tenon.LengthMax(4)}, "unknown(string, length <= 4)"},
	} {
		if got := tenon.Narrow(tt.v, tt.ns...).String(); got != tt.want {
			t.Errorf("%s: narrowed to %s, want %s", tt.name, got, tt.want)
		}
	}
	// The narrowing to null leaves null as the only possibility.
	null := tenon.Narrow(tenon.Unknown(num), tenon.Null())
	if !null.Range().AllowsNull() {
		t.Errorf("narrowing to null produced %v, whose range excludes null", null)
	}
	if got := tenon.Narrow(null, tenon.NotNull()); !got.IsError() {
		t.Errorf("narrowing a null range to not null produced %v, want an error value", got)
	}
	// Each narrowing applies only to the types of its row.
	mustPanicUsage(t, "does not apply to a value of type string", func() {
		tenon.Narrow(tenon.Unknown(str), tenon.NumberMin(five, true))
	})
	mustPanicUsage(t, "does not apply to a value of type number", func() {
		tenon.Narrow(tenon.Unknown(num), tenon.StringPrefix("a"))
	})
	mustPanicUsage(t, "does not apply to a value of type number", func() {
		tenon.Narrow(tenon.Unknown(num), tenon.LengthMax(1))
	})
	mustPanicUsage(t, "does not apply to a value of type bool", func() {
		tenon.Narrow(tenon.Unknown(tenon.BoolType()), tenon.LengthMin(0))
	})
	// A tuple and an object have the length their type fixes, so a length
	// bound on one is a mistake, not a narrowing.
	mustPanicUsage(t, "does not apply to a value of type tuple([string])", func() {
		tenon.Narrow(tenon.Unknown(tenon.Tuple(str)), tenon.LengthMax(1))
	})
	mustPanicUsage(t, "does not apply to a value of type object({})", func() {
		tenon.Narrow(tenon.Unknown(tenon.Object(nil)), tenon.LengthMin(1))
	})
}

func TestConformance_UN003_NarrowingIsMonotone(t *testing.T) {
	conformance.Covers(t, "UN-003")
	one, five := tenon.NumberFromInt(1), tenon.NumberFromInt(5)
	str, num := tenon.StringType(), tenon.NumberType()
	lst := tenon.List(str)
	// Applying a weaker narrowing to a range that already says more leaves it
	// as it was, whichever order the two arrive in: a narrowing can only ever
	// remove possibilities.
	for _, tt := range []struct {
		name string
		v    tenon.Value
		a, b tenon.Narrowing
		want string
	}{
		{"lower bound", tenon.Unknown(num), tenon.NumberMin(five, true), tenon.NumberMin(one, true), "unknown(number, >= 5)"},
		{"bound inclusivity", tenon.Unknown(num), tenon.NumberMin(five, false), tenon.NumberMin(five, true), "unknown(number, > 5)"},
		{"upper bound", tenon.Unknown(num), tenon.NumberMax(one, true), tenon.NumberMax(five, true), "unknown(number, <= 1)"},
		{"length", tenon.Unknown(lst), tenon.LengthMax(3), tenon.LengthMax(9), "unknown(list(string), length <= 3)"},
		{"prefix", tenon.Unknown(str), tenon.StringPrefix("abc-"), tenon.StringPrefix("ab"), `unknown(string, prefix "abc-", length >= 4)`},
		{"not null", tenon.Unknown(num), tenon.NotNull(), tenon.NotNull(), "unknown(number, not null)"},
	} {
		for _, order := range [][]tenon.Narrowing{{tt.a, tt.b}, {tt.b, tt.a}} {
			if got := tenon.Narrow(tt.v, order...).String(); got != tt.want {
				t.Errorf("%s: narrowed to %s, want %s", tt.name, got, tt.want)
			}
		}
	}
	// A known value is a range of one, so narrowing it either keeps it whole
	// or contradicts it.
	text := tenon.String("abc")
	if got := tenon.Narrow(text, tenon.StringPrefix("ab"), tenon.NotNull(), tenon.LengthMax(3)); got != text {
		t.Errorf("narrowing a known value it satisfies produced %v, want the value itself", got)
	}
}

func TestConformance_UN004_ContradictionIsAnErrorValue(t *testing.T) {
	conformance.Covers(t, "UN-004")
	three, five := tenon.NumberFromInt(3), tenon.NumberFromInt(5)
	str, num := tenon.StringType(), tenon.NumberType()
	lst := tenon.List(str)
	for _, tt := range []struct {
		name string
		v    tenon.Value
		ns   []tenon.Narrowing
		want string
	}{
		{
			"a lower bound above an upper bound", tenon.Unknown(num),
			[]tenon.Narrowing{tenon.NumberMin(five, true), tenon.NumberMax(three, true)},
			"no value of type number satisfies both >= 5 and <= 3",
		},
		{
			"equal bounds, one of them exclusive", tenon.Unknown(num),
			[]tenon.Narrowing{tenon.NumberMin(five, false), tenon.NumberMax(five, true)},
			"no value of type number satisfies both > 5 and <= 5",
		},
		{
			"null after not null", tenon.Unknown(num),
			[]tenon.Narrowing{tenon.NotNull(), tenon.Null()},
			"no value of type number satisfies both not null and null",
		},

		{
			"two prefixes that diverge", tenon.Unknown(str),
			[]tenon.Narrowing{tenon.StringPrefix("ab-"), tenon.StringPrefix("ax-")},
			`no value of type string satisfies both prefix "ab-" and prefix "ax-"`,
		},
		{
			"a prefix longer than the length allows", tenon.Unknown(str),
			[]tenon.Narrowing{tenon.StringPrefix("ab-"), tenon.LengthMax(1)},
			`no value of type string satisfies both prefix "ab-" and length <= 1`,
		},
		{
			"length bounds that cross", tenon.Unknown(lst),
			[]tenon.Narrowing{tenon.LengthMin(3), tenon.LengthMax(2)},
			"no value of type list(string) satisfies both length >= 3 and length <= 2",
		},
		{
			"a known value that does not satisfy the narrowing", tenon.String("xy"),
			[]tenon.Narrowing{tenon.StringPrefix("ab-")},
			`the value "xy" does not satisfy prefix "ab-"`,
		},
		{
			"a known null that cannot be not null", tenon.NullVal(str),
			[]tenon.Narrowing{tenon.NotNull()},
			"the value null does not satisfy not null",
		},
	} {
		got := tenon.Narrow(tt.v, tt.ns...)
		if !got.IsError() {
			t.Errorf("%s: produced %v, want an error value", tt.name, got)
			continue
		}
		diags := got.Diagnostics()
		if len(diags) != 1 {
			t.Errorf("%s: produced %d diagnostics, want 1", tt.name, len(diags))
			continue
		}
		if diags[0].Code != tenon.CodeRangeContradiction {
			t.Errorf("%s: code %s, want %s", tt.name, diags[0].Code, tenon.CodeRangeContradiction)
		}
		if diags[0].Message != tt.want {
			t.Errorf("%s: message %q, want %q", tt.name, diags[0].Message, tt.want)
		}
		if diags[0].Path.Len() != 0 {
			t.Errorf("%s: path %s, want the empty path", tt.name, diags[0].Path)
		}
	}
}

func TestNarrowingSaysNothingAboutNull(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	five := tenon.NumberFromInt(5)
	// A bound, a prefix or a length says what a value is when it is not null,
	// so null satisfies it: an optional attribute with a constraint on it
	// stays optional.
	null := tenon.NullVal(str)
	if got := tenon.Narrow(null, tenon.StringPrefix("ab"), tenon.LengthMin(2)); got != null {
		t.Errorf("narrowing the null value produced %v, want the value itself", got)
	}
	// The same holds of a range that has been narrowed to null, and the
	// narrowing leaves no trace, so the range of null has one spelling.
	onlyNull := tenon.Narrow(tenon.Unknown(num), tenon.Null())
	if got := tenon.Narrow(onlyNull, tenon.NumberMin(five, true)); got != onlyNull {
		t.Errorf("narrowing a null range by a bound produced %v, want the range itself", got)
	}
	// A range that holds null keeps it until NotNull takes it away.
	bounded := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(five, true))
	if !bounded.Range().AllowsNull() {
		t.Errorf("%v excludes null, but only NotNull excludes null", bounded)
	}
	if tenon.Narrow(bounded, tenon.NotNull()).Range().AllowsNull() {
		t.Error("NotNull left null in the range")
	}
}

func TestUnknownAndNullValues(t *testing.T) {
	str := tenon.StringType()
	unknown, null := tenon.Unknown(str), tenon.NullVal(str)
	for _, tt := range []struct {
		v       tenon.Value
		known   bool
		text    string
		content string
	}{
		{unknown, false, "unknown(string)", "an unknown value of type string"},
		{null, true, "null", "the null value of type string"},
	} {
		if !tt.v.IsResolved() || tt.v.Type() != str {
			t.Errorf("%v is not a resolved string value", tt.v)
		}
		if tt.v.IsError() || tt.v.IsPending() {
			t.Errorf("%v is in more than one state", tt.v)
		}
		if got := tt.v.IsKnown(); got != tt.known {
			t.Errorf("%v: IsKnown is %t, want %t", tt.v, got, tt.known)
		}
		if got := tt.v.String(); got != tt.text {
			t.Errorf("String is %s, want %s", got, tt.text)
		}
		// Neither has content to read.
		mustPanicUsage(t, "AsString called on "+tt.content+", which has no content", func() { tt.v.AsString() })
	}
	// Containers of them are read through the same accessors, which refuse.
	lst := tenon.List(str)
	mustPanicUsage(t, "Len called on the null value of type list(string), which has no content", func() {
		tenon.NullVal(lst).Len()
	})
	mustPanicUsage(t, "Index called on an unknown value of type list(string), which has no content", func() {
		tenon.Unknown(lst).Index(0)
	})
	mustPanicUsage(t, "Elements called on an unknown value of type list(string), which has no content", func() {
		tenon.Unknown(lst).Elements()
	})
	mustPanicUsage(t, "MapKeys called on an unknown value of type map(string), which has no content", func() {
		tenon.Unknown(tenon.Map(str)).MapKeys()
	})
	// A member of a container must still be a known value; null and unknown
	// members arrive with the rest of the range work.
	mustPanicUsage(t, "element 0 is an unknown value of type string, not a known value", func() {
		tenon.ListVal(str, tenon.Unknown(str))
	})
	mustPanicUsage(t, "element 0 is the null value of type string, not a known value", func() {
		tenon.TupleVal(tenon.NullVal(str))
	})
	// Neither is a type.
	mustPanicUsage(t, "use of the zero Type", func() { tenon.Unknown(tenon.Type{}) })
	mustPanicUsage(t, "use of the zero Type", func() { tenon.NullVal(tenon.Type{}) })
}

func TestNarrowingLengthIsGraphemeClusters(t *testing.T) {
	// "e" followed by a combining acute accent is one grapheme cluster, and a
	// string's length is its count of clusters, not of scalars or bytes.
	text := tenon.String("e\U00000301")
	if got := tenon.Narrow(text, tenon.LengthMax(1)); got != text {
		t.Errorf("narrowing a one-cluster string to length <= 1 produced %v, want the value itself", got)
	}
	if got := tenon.Narrow(text, tenon.LengthMin(2)); !got.IsError() {
		t.Errorf("narrowing a one-cluster string to length >= 2 produced %v, want an error value", got)
	}
	// A prefix of one cluster forces a length of at least one, though this one
	// is two scalar values and eight bytes long.
	flag := "\U0001F1E9\U0001F1EA"
	want := "unknown(string, prefix " + strconv.Quote(flag) + ", length >= 1)"
	if got := tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix(flag)).String(); got != want {
		t.Errorf("narrowed to %s, want %s", got, want)
	}
}

func TestConformance_UN006_PrefixTruncation(t *testing.T) {
	conformance.Covers(t, "UN-006")
	str := tenon.StringType()
	flag := "\U0001F1E9\U0001F1EA"
	// What a narrowing records is the part of the supplied text that text
	// following it cannot change.
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"nothing composes with a hyphen", "ab-", "ab-"},
		{"nothing composes with a digit", "v1", "v1"},
		{"q is the one letter nothing composes with", "seq", "seq"},
		{"a trailing letter a mark would change", "cafe", "caf"},
		{"a trailing letter, with no accent in sight", "hello world", "hello worl"},
		{"nothing composes with a regional indicator", flag, flag},
		{"a combining sequence, which leaves nothing", "e\U00000301", ""},
	} {
		got := tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix(tt.in)).String()
		if tt.want == "" {
			if got != "unknown(string)" {
				t.Errorf("%s: %q recorded %s, want unknown(string)", tt.name, tt.in, got)
			}
			continue
		}
		if want := "prefix " + strconv.Quote(tt.want); !strings.Contains(got, want) {
			t.Errorf("%s: %q recorded %s, want it to hold %s", tt.name, tt.in, got, want)
		}
	}
	// The truncation is what makes the narrowing sound: however the supplied
	// text continues, the value it grows into still satisfies the narrowing.
	for _, suffix := range []string{
		"", "x", " ", "\U00000301", "\U00000307", "\U0000030C", "\U00000323",
		"\U00000327", "\U0000200D", "\U0001F1EB", "e\U00000301",
	} {
		for _, text := range []string{"cafe", "hello world", "ab-", flag} {
			v := tenon.String(text + suffix)
			if got := tenon.Narrow(v, tenon.StringPrefix(text)); got.IsError() {
				t.Errorf("%q continued by %q gives %v, which contradicts a prefix of %q: %v",
					text, suffix, v, text, got)
			}
		}
	}
}

func TestNarrowUsageErrors(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	mustPanicUsage(t, "Narrow called on a pending value, which has no range", func() {
		tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NotNull())
	})
	mustPanicUsage(t, "Narrow called with the zero Narrowing", func() {
		tenon.Narrow(tenon.Unknown(str), tenon.Narrowing{})
	})
	mustPanicUsage(t, "use of the zero Value", func() { tenon.Narrow(tenon.Value{}, tenon.NotNull()) })
	mustPanicUsage(t, "NumberMin called with an unknown value of type number as a bound, not a known Number value", func() {
		tenon.NumberMin(tenon.Unknown(num), true)
	})
	mustPanicUsage(t, "NumberMax called with a value of type string as a bound, not a known Number value", func() {
		tenon.NumberMax(tenon.String("5"), true)
	})
	mustPanicUsage(t, "StringPrefix called with text that is not well-formed UTF-8, at byte 1", func() {
		tenon.StringPrefix("a\xffb")
	})
	mustPanicUsage(t, "LengthMin called with a negative length, -1", func() { tenon.LengthMin(-1) })
	mustPanicUsage(t, "LengthMax called with a negative length, -2", func() { tenon.LengthMax(-2) })
}

func TestNarrowAnErrorValue(t *testing.T) {
	// Narrowing an error value carries its diagnostics forward, like any other
	// operation on one.
	bad := tenon.String("\xff")
	got := tenon.Narrow(bad, tenon.NotNull())
	if !got.IsError() {
		t.Fatalf("narrowing an error value produced %v, want an error value", got)
	}
	if diags := got.Diagnostics(); len(diags) != 1 || diags[0].Code != tenon.CodeStringInvalidUTF8 {
		t.Errorf("narrowing an error value produced %v, want its own diagnostic", got)
	}
}

func TestConformance_UN005_NarrowingToOneValue(t *testing.T) {
	conformance.Covers(t, "UN-005")
	one, five := tenon.NumberFromInt(1), tenon.NumberFromInt(5)
	str, num := tenon.StringType(), tenon.NumberType()
	lst, set, mp := tenon.List(str), tenon.Set(str), tenon.Map(str)
	empty := tenon.Tuple()
	// A range that comes down to one value is that value, known.
	for _, tt := range []struct {
		name string
		v    tenon.Value
		ns   []tenon.Narrowing
		want tenon.Value
	}{
		{
			"bounds that meet", tenon.Unknown(num),
			[]tenon.Narrowing{tenon.NumberMin(five, true), tenon.NumberMax(five, true), tenon.NotNull()},
			five,
		},
		{
			"a string of no length", tenon.Unknown(str),
			[]tenon.Narrowing{tenon.NotNull(), tenon.LengthMax(0)},
			tenon.String(""),
		},
		{
			"a list of no length", tenon.Unknown(lst),
			[]tenon.Narrowing{tenon.NotNull(), tenon.LengthMax(0)},
			tenon.ListVal(str),
		},
		{
			"a set of no length", tenon.Unknown(set),
			[]tenon.Narrowing{tenon.NotNull(), tenon.LengthMax(0)},
			tenon.SetVal(str),
		},
		{
			"a map of no length", tenon.Unknown(mp),
			[]tenon.Narrowing{tenon.NotNull(), tenon.LengthMax(0)},
			tenon.MapVal(str, nil),
		},
		{"null on a number", tenon.Unknown(num), []tenon.Narrowing{tenon.Null()}, tenon.NullVal(num)},
		{"null on a list", tenon.Unknown(lst), []tenon.Narrowing{tenon.Null()}, tenon.NullVal(lst)},
		{
			"a type with one value", tenon.Unknown(empty),
			[]tenon.Narrowing{tenon.NotNull()},
			tenon.TupleVal(),
		},
		{
			"a type whose members have one value", tenon.Unknown(tenon.Tuple(empty, empty)),
			[]tenon.Narrowing{tenon.NotNull()},
			tenon.TupleVal(tenon.TupleVal(), tenon.TupleVal()),
		},
		{
			"an object with no attributes", tenon.Unknown(tenon.Object(nil)),
			[]tenon.Narrowing{tenon.NotNull()},
			tenon.ObjectVal(nil),
		},
	} {
		got := tenon.Narrow(tt.v, tt.ns...)
		if !got.IsKnown() {
			t.Errorf("%s: narrowed to %v, which is not a known value", tt.name, got)
			continue
		}
		if got.Type() != tt.want.Type() || got.String() != tt.want.String() {
			t.Errorf("%s: narrowed to %v, want %v", tt.name, got, tt.want)
		}
	}
	// A range that still holds more than one value stays unknown.
	for _, tt := range []struct {
		name string
		v    tenon.Value
		ns   []tenon.Narrowing
	}{
		{
			"bounds that do not meet", tenon.Unknown(num),
			[]tenon.Narrowing{tenon.NumberMin(one, true), tenon.NumberMax(five, true), tenon.NotNull()},
		},
		{
			"a length that leaves room", tenon.Unknown(lst),
			[]tenon.Narrowing{tenon.NotNull(), tenon.LengthMax(1)},
		},
		{
			"no length, but null is still possible", tenon.Unknown(str),
			[]tenon.Narrowing{tenon.LengthMax(0)},
		},
		{
			"a member with more than one value", tenon.Unknown(tenon.Tuple(tenon.BoolType())),
			[]tenon.Narrowing{tenon.NotNull()},
		},
		{
			// A list of exactly three empty tuples holds one value too, but
			// finding that out means building a value as large as the bounds
			// allow, which the rule permits an implementation to decline.
			"a length that pins a collection", tenon.Unknown(tenon.List(empty)),
			[]tenon.Narrowing{tenon.NotNull(), tenon.LengthMin(3), tenon.LengthMax(3)},
		},
	} {
		if got := tenon.Narrow(tt.v, tt.ns...); got.IsKnown() {
			t.Errorf("%s: narrowed to the known value %v", tt.name, got)
		}
	}
}
