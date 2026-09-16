package tenon_test

import (
	"testing"

	"tenon"
	"tenon/conformance"
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
	if got := bad.Diagnostics()[0].Path.String(); got != "[2]" {
		t.Errorf("the error member is located at %s, want [2], where it was given", got)
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
