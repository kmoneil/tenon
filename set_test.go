package tenon_test

import (
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
)

func TestConformance_EQ040_SetMembersAreToldApartByEquality(t *testing.T) {
	conformance.Covers(t, "EQ-040")
	str, num := tenon.StringType(), tenon.NumberType()
	s := func(text string) tenon.Value { return tenon.String(text) }
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	null := tenon.NullVal(str)
	for _, tt := range []struct {
		name    string
		set     tenon.Value
		members int
	}{
		{"nothing at all", tenon.SetVal(str), 0},
		{"a member given twice", tenon.SetVal(str, s("a"), s("a")), 1},
		{"a member given three times among others", tenon.SetVal(str, s("a"), s("b"), s("a"), s("a")), 2},
		{"members that differ", tenon.SetVal(str, s("a"), s("b")), 2},
		// Equality is what tells members apart, so a value written another
		// way is not another member.
		{"one number written two ways", tenon.SetVal(num, n(1), tenon.NumberFromText("1.000")), 1},
		{"one string in two normal forms", tenon.SetVal(str, s("e\U00000301"), s("\U000000e9")), 1},
		{"two nulls", tenon.SetVal(str, null, null), 1},
		{"null beside a value", tenon.SetVal(str, null, s("a")), 2},
		{
			"one list written two ways",
			tenon.SetVal(tenon.List(num), tenon.ListVal(num, n(1)), tenon.ListVal(num, tenon.NumberFromText("1.0"))),
			1,
		},
		{
			"one set given its members in either order",
			tenon.SetVal(tenon.Set(str), tenon.SetVal(str, s("a"), s("b")), tenon.SetVal(str, s("b"), s("a"))),
			1,
		},
	} {
		if got := tt.set.Len(); got != tt.members {
			t.Errorf("%s: the set has %d members, want %d: %v", tt.name, got, tt.members, tt.set)
		}
	}
	// The member kept is the one given first, and it is the value, not the
	// spelling it arrived in.
	set := tenon.SetVal(num, tenon.NumberFromText("1.000"), n(1))
	if got := set.Elements()[0].String(); got != "1" {
		t.Errorf("the set kept %s, want the first member given", got)
	}
	// A set built from repetitions is the set, by every comparison there is.
	once, thrice := tenon.SetVal(str, s("a")), tenon.SetVal(str, s("a"), s("a"), s("a"))
	if !tenon.Identical(once, thrice) || tenon.Hash(once) != tenon.Hash(thrice) {
		t.Errorf("%v and %v are not one set", once, thrice)
	}
	if got := tenon.Equals(once, thrice).String(); got != "true" {
		t.Errorf("one set built two ways is %s", got)
	}
	// An error member is still reported where it was given: dedup never moves
	// one, because an error member never reaches it.
	bad := tenon.SetVal(str, s("a"), s("a"), tenon.String("\xff"))
	if !bad.IsError() {
		t.Fatalf("a set with an error member is %v, want an error value", bad)
	}
	if got := bad.Diagnostics()[0].Path.String(); got != ".[2]" {
		t.Errorf("the error member is located at %s, want .[2], where it was given", got)
	}
}

func TestConformance_EQ041_MembersThatAreNotKnownAreKept(t *testing.T) {
	conformance.Covers(t, "EQ-041")
	num := tenon.NumberType()
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	unknown := tenon.Unknown(num)
	atLeastFive := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true))
	for _, tt := range []struct {
		name    string
		set     tenon.Value
		members int
	}{
		// Two unknowns could turn out to be one value or two. Dropping one
		// would answer a question nothing has answered.
		{"two unknowns", tenon.SetVal(num, unknown, unknown), 2},
		{"an unknown beside a value it could be", tenon.SetVal(num, n(1), unknown), 2},
		// Even where they provably differ, they are two members, which is
		// what they would be anyway.
		{"an unknown that cannot be the value beside it", tenon.SetVal(num, n(1), atLeastFive), 2},
		{"unknowns with different ranges", tenon.SetVal(num, unknown, atLeastFive), 2},
		{
			"lists holding unknowns, which are not known either",
			tenon.SetVal(tenon.List(num), tenon.ListVal(num, unknown), tenon.ListVal(num, unknown)),
			2,
		},
		// Dedup carries on around them.
		{"a value given twice beside an unknown", tenon.SetVal(num, n(1), unknown, n(1)), 2},
		{"an unknown between two of one value", tenon.SetVal(num, n(1), unknown, tenon.NumberFromText("1.0")), 2},
	} {
		if got := tt.set.Len(); got != tt.members {
			t.Errorf("%s: the set has %d members, want %d: %v", tt.name, got, tt.members, tt.set)
		}
	}
	// A set holding one is not known itself, since what it holds is not.
	if tenon.SetVal(num, unknown).IsKnown() {
		t.Error("a set holding an unknown reports itself known")
	}
}

func TestConformance_EQ042_TheLengthOfASetHoldingUnknowns(t *testing.T) {
	conformance.Covers(t, "EQ-042")
	num, str, boolType := tenon.NumberType(), tenon.StringType(), tenon.BoolType()
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	unknown, unknownBool := tenon.Unknown(num), tenon.Unknown(boolType)
	atLeastFive := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true))
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		// Two unknowns could be one member or two, so the length is both.
		{"two unknowns", tenon.Length(tenon.SetVal(num, unknown, unknown)), "unknown(number, not null, >= 1, <= 2)"},
		{"one unknown", tenon.Length(tenon.SetVal(num, unknown)), "1"},
		{
			"an unknown beside a value it could be",
			tenon.Length(tenon.SetVal(num, n(1), unknown)),
			"unknown(number, not null, >= 1, <= 2)",
		},
		// An unknown that cannot be the value beside it is another member,
		// which settles the length after all.
		{"an unknown that is provably another member", tenon.Length(tenon.SetVal(num, n(1), atLeastFive)), "2"},
		{
			"two of them, one provably distinct and one not",
			tenon.Length(tenon.SetVal(num, n(1), atLeastFive, unknown)),
			"unknown(number, not null, >= 2, <= 3)",
		},
		// Every other container has the length it has.
		{"a set of known members", tenon.Length(tenon.SetVal(num, n(1), n(2), n(1))), "2"},
		{"a list holding an unknown", tenon.Length(tenon.ListVal(num, unknown, unknown)), "2"},
		{"a map holding an unknown", tenon.Length(tenon.MapVal(num, map[string]tenon.Value{"k": unknown})), "1"},
		{"a string", tenon.Length(tenon.String("e\U00000301x")), "2"},
		{"nothing at all", tenon.Length(tenon.SetVal(num)), "0"},
		// A value that is not there to count says what its range says, and a
		// length is never negative whatever else is unknown.
		{"an unknown list", tenon.Length(tenon.Unknown(tenon.List(num))), "unknown(number, not null, >= 0)"},
		{
			"an unknown string with a length bound",
			tenon.Length(tenon.Narrow(tenon.Unknown(str), tenon.LengthMin(2), tenon.LengthMax(5))),
			"unknown(number, not null, >= 2, <= 5)",
		},
		{"a pending value", tenon.Length(tenon.Pending(tenon.Any())), "unknown(number, not null, >= 0)"},
		// A set holds distinct values of its element type, null among them, so
		// where that type holds few values they bound the length as well.
		{
			"four unknown bools",
			tenon.Length(tenon.SetVal(boolType, unknownBool, unknownBool, unknownBool, unknownBool)),
			"unknown(number, not null, >= 1, <= 3)",
		},
		{
			"an unknown set of bools",
			tenon.Length(tenon.Narrow(tenon.Unknown(tenon.Set(boolType)), tenon.NotNull())),
			"unknown(number, not null, >= 0, <= 3)",
		},
		{
			"an unknown set of numbers, which has no such bound",
			tenon.Length(tenon.Narrow(tenon.Unknown(tenon.Set(num)), tenon.NotNull())),
			"unknown(number, not null, >= 0)",
		},
		{
			// A list holds a value as often as it likes, so what its element
			// type holds bounds nothing.
			"an unknown list of bools",
			tenon.Length(tenon.Narrow(tenon.Unknown(tenon.List(boolType)), tenon.NotNull())),
			"unknown(number, not null, >= 0)",
		},
		{"a list holding four unknown bools", tenon.Length(tenon.ListVal(boolType, unknownBool, unknownBool, unknownBool, unknownBool)), "4"},
	} {
		if got := tt.got.String(); got != tt.want {
			t.Errorf("%s: the length is %s, want %s", tt.name, got, tt.want)
		}
	}
	// A narrowing allows such a set the lengths Length gives it: a bound that
	// leaves one of them does not contradict the set, and one that leaves none
	// does.
	for _, set := range []tenon.Value{
		tenon.SetVal(num, unknown, unknown),
		tenon.SetVal(num, n(1), unknown),
		tenon.SetVal(num, n(1), atLeastFive),
		tenon.SetVal(num, n(1), atLeastFive, unknown),
		tenon.SetVal(num, n(1), n(2), unknown, atLeastFive),
		tenon.SetVal(boolType, unknownBool, unknownBool, unknownBool, unknownBool),
		tenon.SetVal(boolType, tenon.Bool(true), unknownBool),
	} {
		const most = 6
		var can [most + 1]bool // whether Length allows each length
		for l := range can {
			eq := tenon.Equals(tenon.Length(set), n(int64(l)))
			can[l] = !eq.IsKnown() || eq.String() == "true"
		}
		for l := range int64(most + 1) {
			for _, tt := range []struct {
				ns    []tenon.Narrowing
				leave []bool // whether Length allows each length the bounds leave
			}{
				{[]tenon.Narrowing{tenon.LengthMax(l)}, can[:l+1]},
				{[]tenon.Narrowing{tenon.LengthMin(l)}, can[l:]},
				{[]tenon.Narrowing{tenon.LengthMin(l), tenon.LengthMax(l)}, can[l : l+1]},
			} {
				if got := tenon.Narrow(set, tt.ns...); got.IsError() == slices.Contains(tt.leave, true) {
					t.Errorf("%v narrowed by %v is %v, but its length is %v", set, tt.ns, got, tenon.Length(set))
				}
			}
		}
	}

	// Length is for the kinds that have one.
	for _, v := range []tenon.Value{tenon.Bool(true), n(1), tenon.TupleVal(), tenon.ObjectVal(nil)} {
		mustPanicUsage(t, "does not satisfy one_of", func() { tenon.Length(v) })
	}
	null := tenon.Length(tenon.NullVal(tenon.List(num)))
	if !null.IsError() || null.Diagnostics()[0].Code != tenon.CodeOperationNullOperand {
		t.Errorf("the length of null is %v, want a null-operand error value", null)
	}
}

func TestConformance_EQ043_MembershipOfASetHoldingUnknowns(t *testing.T) {
	conformance.Covers(t, "EQ-043")
	num, str := tenon.NumberType(), tenon.StringType()
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	unknown := tenon.Unknown(num)
	atLeastFive := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true))
	known := tenon.SetVal(num, n(1), n(2))
	open := tenon.SetVal(num, n(1), unknown)
	untypedNull := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.Null())
	for _, tt := range []struct {
		name string
		got  tenon.Value
		want string
	}{
		{"a member of a known set", tenon.Contains(known, n(1)), "true"},
		{"a value that is not one", tenon.Contains(known, n(3)), "false"},
		{"the same value written another way", tenon.Contains(known, tenon.NumberFromText("2.00")), "true"},
		{"a value of another type, which is not a member", tenon.Contains(known, tenon.String("1")), "false"},
		{"null, which is not a member here", tenon.Contains(known, tenon.NullVal(num)), "false"},
		{"null, which is one there", tenon.Contains(tenon.SetVal(num, tenon.NullVal(num)), tenon.NullVal(num)), "true"},
		{"nothing is a member of an empty set", tenon.Contains(tenon.SetVal(num), n(1)), "false"},
		// A member that is provably there settles it however open the rest is.
		{"a known member of a set holding an unknown", tenon.Contains(open, n(1)), "true"},
		// Otherwise the unknown member could be the value looked for.
		{"a value the unknown member could be", tenon.Contains(open, n(3)), "unknown(bool, not null)"},
		// Unless it provably is not, which settles it the other way.
		{
			"a value no member could be",
			tenon.Contains(tenon.SetVal(num, n(1), atLeastFive), n(3)),
			"false",
		},
		{
			"a value looked for that is not known itself",
			tenon.Contains(known, unknown),
			"unknown(bool, not null)",
		},
		{
			"one that is provably no member",
			tenon.Contains(known, atLeastFive),
			"false",
		},
		// A null whose type is not known yet is a null all the same: it is no
		// member of a set whose members cannot be null, and it could be one of a
		// set holding null.
		{"a null of no known type, in a set of values", tenon.Contains(known, untypedNull), "false"},
		{"a null of no known type, in the empty set", tenon.Contains(tenon.SetVal(num), untypedNull), "false"},
		{
			"a null of no known type, in a set holding null",
			tenon.Contains(tenon.SetVal(num, tenon.NullVal(num)), untypedNull),
			"unknown(bool, not null)",
		},
		{
			"a null of a type the set's members cannot have",
			tenon.Contains(tenon.SetVal(num, tenon.NullVal(num)), tenon.Narrow(tenon.Pending(tenon.Exactly(str)), tenon.Null())),
			"false",
		},
		// A set that is not there to look through leaves it open.
		{"an unknown set", tenon.Contains(tenon.Unknown(tenon.Set(num)), n(1)), "unknown(bool, not null)"},
		{"a pending set", tenon.Contains(tenon.Pending(tenon.SetOf(tenon.Any())), n(1)), "unknown(bool, not null)"},
	} {
		if got := tt.got.String(); got != tt.want {
			t.Errorf("%s: membership is %s, want %s", tt.name, got, tt.want)
		}
	}
	// The first operand is a set, and nothing else.
	for _, v := range []tenon.Value{tenon.ListVal(num), tenon.String("a"), n(1)} {
		mustPanicUsage(t, "does not satisfy set_of(any)", func() { tenon.Contains(v, n(1)) })
	}
	null := tenon.Contains(tenon.NullVal(tenon.Set(str)), tenon.String("a"))
	if !null.IsError() || null.Diagnostics()[0].Code != tenon.CodeOperationNullOperand {
		t.Errorf("membership of null is %v, want a null-operand error value", null)
	}
	bad := tenon.Contains(known, tenon.String("\xff"))
	if !bad.IsError() || bad.Diagnostics()[0].Code != tenon.CodeStringInvalidUTF8 {
		t.Errorf("membership of an error value is %v, want its diagnostics", bad)
	}
}

func TestConformance_EQ044_SetIterationOrder(t *testing.T) {
	conformance.Covers(t, "EQ-044")
	num := tenon.NumberType()
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	// Known members come in canonical order, whatever order they were given.
	given := []tenon.Value{n(3), n(1), n(2), tenon.NullVal(num), n(1)}
	want := "set(number)[null(number), 1, 2, 3]"
	for _, order := range [][]int{{0, 1, 2, 3, 4}, {4, 3, 2, 1, 0}, {2, 0, 4, 1, 3}, {3, 1, 4, 0, 2}} {
		members := make([]tenon.Value, len(order))
		for i, at := range order {
			members[i] = given[at]
		}
		if got := tenon.SetVal(num, members...).String(); got != want {
			t.Errorf("built in order %v the set reads %s, want %s", order, got, want)
		}
	}
	// Members that are not known come after the known ones, and their place
	// follows from the member rather than from where it was given: two sets
	// with the same members are one value and iterate one way.
	unknown := tenon.Unknown(num)
	atLeastFive := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true))
	notNull := tenon.Narrow(tenon.Unknown(num), tenon.NotNull())
	first := tenon.SetVal(num, unknown, n(2), atLeastFive, notNull, n(1))
	for _, members := range [][]tenon.Value{
		{n(1), n(2), unknown, atLeastFive, notNull},
		{atLeastFive, notNull, unknown, n(2), n(1)},
		{notNull, n(1), atLeastFive, n(2), unknown},
	} {
		again := tenon.SetVal(num, members...)
		if !tenon.Identical(first, again) {
			t.Fatalf("%v and %v are not one set to begin with", first, again)
		}
		if got, want := again.String(), first.String(); got != want {
			t.Errorf("one set built two ways iterates %s and %s", want, got)
		}
	}
	// The known members really do come first, and in canonical order.
	elems := first.Elements()
	if got, want := len(elems), 5; got != want {
		t.Fatalf("the set has %d members, want %d", got, want)
	}
	for i, e := range elems {
		if known := e.IsKnown(); known != (i < 2) {
			t.Errorf("member %d of %v is known: %t", i, first, known)
		}
	}
	if got := tenon.CanonicalCompare(elems[0], elems[1]); got >= 0 {
		t.Errorf("the known members are not in canonical order: %d", got)
	}
	// Iterating again gives the same order, in this run and in the next.
	for range 50 {
		if !slicesEqualValues(first.Elements(), elems) {
			t.Fatal("iterating the same set twice gave two orders")
		}
	}
}

// slicesEqualValues reports whether two slices hold the same values in the
// same places.
func slicesEqualValues(a, b []tenon.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !tenon.Identical(a[i], b[i]) {
			return false
		}
	}
	return true
}
