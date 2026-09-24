package tenon_test

import (
	"bytes"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/conformance/values"
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

// TestConformance_EQ041_NoMemberThatIsNotKnownIsSettledEqual holds Equals to
// what lets a set keep a member that is not known without comparing it with
// the members it already holds: Equals never settles such a value equal to
// anything, since what it could still turn out to be might differ, so no member
// kept could be its double. Every pair from the corpus is tried, each value
// with itself among them, which is where an answer by identity would show.
func TestConformance_EQ041_NoMemberThatIsNotKnownIsSettledEqual(t *testing.T) {
	conformance.Covers(t, "EQ-041", "EQ-003")
	all := values.All()
	tried := 0
	for _, b := range all {
		if !b.IsResolved() || b.IsKnown() {
			continue
		}
		for _, a := range all {
			if eq := tenon.Equals(a, b); eq.IsKnown() && eq.AsBool() {
				t.Errorf("Equals(%v, %v) is known true, where %v is not known", a, b, b)
			}
			tried++
		}
	}
	if tried < 1000 {
		t.Errorf("only %d pairs held a value that is not known", tried)
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

	// A member is kept because it could still be a value of its own. Where
	// every value it could be is a member already, it could not, and the set
	// holds those alone.
	b, empty := tenon.BoolType(), tenon.Tuple()
	for _, tt := range []struct {
		name string
		set  tenon.Value
		want tenon.Value
	}{
		{
			"every bool, and an unknown bool",
			tenon.SetVal(b, tenon.Bool(false), tenon.Bool(true), tenon.NullVal(b), tenon.Unknown(b)),
			tenon.SetVal(b, tenon.Bool(false), tenon.Bool(true), tenon.NullVal(b)),
		},
		{
			"every bool, and two unknown bools, one of which cannot be null",
			tenon.SetVal(b, tenon.NullVal(b), tenon.Bool(true), tenon.Bool(false),
				tenon.Unknown(b), tenon.Narrow(tenon.Unknown(b), tenon.NotNull())),
			tenon.SetVal(b, tenon.Bool(false), tenon.Bool(true), tenon.NullVal(b)),
		},
		{
			"every empty tuple, and an unknown one",
			tenon.SetVal(empty, tenon.TupleVal(), tenon.NullVal(empty), tenon.Unknown(empty)),
			tenon.SetVal(empty, tenon.TupleVal(), tenon.NullVal(empty)),
		},
		{
			// Not every bool, but every bool this member could be.
			"false and true, and a bool that cannot be null",
			tenon.SetVal(b, tenon.Bool(false), tenon.Bool(true), tenon.Narrow(tenon.Unknown(b), tenon.NotNull())),
			tenon.SetVal(b, tenon.Bool(false), tenon.Bool(true)),
		},
		{
			"the empty tuple, and one that cannot be null",
			tenon.SetVal(empty, tenon.TupleVal(), tenon.Narrow(tenon.Unknown(empty), tenon.NotNull())),
			tenon.SetVal(empty, tenon.TupleVal()),
		},
		{
			// The member could be the set of the empty tuple or the set of
			// null, and the set holds both.
			"sets of empty tuples, and one holding an unknown",
			tenon.SetVal(tenon.Set(empty),
				tenon.SetVal(empty, tenon.TupleVal()), tenon.SetVal(empty, tenon.NullVal(empty)),
				tenon.SetVal(empty, tenon.Unknown(empty))),
			tenon.SetVal(tenon.Set(empty), tenon.SetVal(empty, tenon.TupleVal()), tenon.SetVal(empty, tenon.NullVal(empty))),
		},
	} {
		if !tt.set.IsKnown() || !tenon.Identical(tt.set, tt.want) {
			t.Errorf("%s: the set is %v, want %v, known", tt.name, tt.set, tt.want)
		}
		if got := tenon.Equals(tt.set, tt.want).String(); got != "true" {
			t.Errorf("%s: it equals %v as %s, want true", tt.name, tt.want, got)
		}
		if tenon.Hash(tt.set) != tenon.Hash(tt.want) {
			t.Errorf("%s: it hashes apart from %v", tt.name, tt.want)
		}
		if got := tenon.Narrow(tt.set); !tenon.Identical(got, tt.set) {
			t.Errorf("%s: narrowing it by nothing gave %v", tt.name, got)
		}
	}
	// One value short of every one of them, the member has something to be.
	for _, tt := range []struct {
		set     tenon.Value
		members int
	}{
		{tenon.SetVal(b, tenon.Bool(false), tenon.Bool(true), tenon.Unknown(b)), 3},
		{tenon.SetVal(b, tenon.NullVal(b), tenon.Bool(true), tenon.Unknown(b)), 3},
		{tenon.SetVal(b, tenon.Bool(false), tenon.Narrow(tenon.Unknown(b), tenon.NotNull())), 2},
		{tenon.SetVal(tenon.Set(empty), tenon.SetVal(empty, tenon.TupleVal()), tenon.SetVal(empty, tenon.Unknown(empty))), 2},
		{tenon.SetVal(empty, tenon.TupleVal(), tenon.Unknown(empty)), 2},
		{tenon.SetVal(num, n(1), unknown), 2},
		{
			// Each member is asked about for itself: the set of the empty
			// tuple and an unknown one could only be a member here, and is
			// dropped, while the set of an unknown one could be the set of
			// null, which is no member, and stays. The one dropped comes
			// first.
			tenon.SetVal(tenon.Set(empty),
				tenon.SetVal(empty, tenon.TupleVal()),
				tenon.SetVal(empty, tenon.TupleVal(), tenon.NullVal(empty)),
				tenon.SetVal(empty, tenon.TupleVal(), tenon.Unknown(empty)),
				tenon.SetVal(empty, tenon.Unknown(empty))),
			3,
		},
	} {
		if tt.set.IsKnown() || tt.set.Len() != tt.members {
			t.Errorf("%v is known, or does not hold %d members", tt.set, tt.members)
		}
	}
	// What a member could be is decided where the element type holds at most
	// 256 values, so a set over a type holding one more keeps its member,
	// however many of those values it holds already.
	for _, tt := range []struct {
		empties int  // how many empty tuples the element type holds
		known   bool // whether a set of every value of it comes out known
	}{
		{7, true},  // 128 values, and null: 129 members
		{8, false}, // 256 values, and null: 257 members
	} {
		elem := tenon.Tuple(slices.Repeat([]tenon.Type{empty}, tt.empties)...)
		values := []tenon.Value{tenon.NullVal(elem), tenon.Unknown(elem)}
		for mask := range 1 << tt.empties {
			row := make([]tenon.Value, tt.empties)
			for i := range row {
				if row[i] = tenon.TupleVal(); mask&(1<<i) != 0 {
					row[i] = tenon.NullVal(empty)
				}
			}
			values = append(values, tenon.TupleVal(row...))
		}
		if set := tenon.SetVal(elem, values...); set.IsKnown() != tt.known {
			t.Errorf("a set of every value of %v and an unknown one is known %t, want %t", elem, set.IsKnown(), tt.known)
		}
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

// TestConformance_EQ030_MembersAreToldApartByTheirHashes holds the work of
// telling known members apart to the hashes they have rather than to the pairs
// they make. A capsule type counts what its values are compared with, which
// only two known values of it ever are.
func TestConformance_EQ030_MembersAreToldApartByTheirHashes(t *testing.T) {
	conformance.Covers(t, "EQ-030", "EQ-042", "UN-002")
	compared := 0
	counted := tenon.Capsule("counted", tenon.CapsuleOps[int]{
		Equals:  func(a, b *int) bool { compared++; return *a == *b },
		Hash:    func(v *int) uint64 { return uint64(*v) },
		Compare: func(a, b *int) int { return *a - *b },
	})
	const size = 400
	members := make([]tenon.Value, size)
	for i := range members {
		v := i
		members[i] = tenon.CapsuleVal(counted, &v)
	}
	held := tenon.SetVal(counted, append(slices.Clone(members), tenon.Unknown(counted))...)
	for _, tt := range []struct {
		name string
		call func() tenon.Value
	}{
		{"a range listing them all", func() tenon.Value {
			return tenon.Narrow(tenon.Unknown(tenon.Set(counted)), tenon.Members(members...))
		}},
		{"the length of a set of them beside an unknown", func() tenon.Value { return tenon.Length(held) }},
	} {
		compared = 0
		if got := tt.call(); got.IsError() {
			t.Fatalf("%s: %v", tt.name, got)
		}
		// Comparing every pair would be eighty thousand of them.
		if compared > 4*size {
			t.Errorf("%s: %d comparisons over %d members, want at most %d", tt.name, compared, size, 4*size)
		}
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

// TestConformance_EQ044_MembersToldApartOnlyByACapsule holds members that are
// not known, and that differ only by a capsule value whose type declares no
// encoding, to an order that follows from those capsule values. The two
// encode alike, since what stands in for each capsule value does, so their
// encodings cannot place them; the capsule values' canonical order does, which
// is the order the type declares.
func TestConformance_EQ044_MembersToldApartOnlyByACapsule(t *testing.T) {
	conformance.Covers(t, "EQ-044", "EQ-045")
	num := tenon.NumberType()
	ranked := tenon.Capsule("ranked", tenon.CapsuleOps[int]{
		Equals:  func(a, b *int) bool { return *a == *b },
		Hash:    func(v *int) uint64 { return uint64(*v) },
		Compare: func(a, b *int) int { return *a - *b },
	})
	member := func(i int) tenon.Value {
		return tenon.TupleVal(tenon.CapsuleVal(ranked, &i), tenon.Unknown(num))
	}
	given := []tenon.Value{member(3), member(1), member(2)}
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}, {1, 2, 0}} {
		members := make([]tenon.Value, len(order))
		for i, at := range order {
			members[i] = given[at]
		}
		elems := tenon.SetVal(tenon.Tuple(ranked, num), members...).Elements()
		for i, want := range []int{1, 2, 3} {
			if !tenon.Identical(elems[i], member(want)) {
				t.Errorf("built in order %v, member %d of the set is %v, want the one ranked %d", order, i, elems[i], want)
			}
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

// TestConformance_EQ045_KnownMembersAreComparedInTheirOrder holds comparing
// the known members of two sets to the order a set holds them in, which puts
// them first and ties exactly the ones that are equal, rather than to the
// pairs they make. A capsule type counts what its values are compared with,
// which only two known values of it ever are.
// TestConformance_EQ041_UnknownMembersAreComparedInTheirOrder holds Identical
// over sets holding members that are not known to comparing each member with
// the one beside it. A set holds those members in the order of their
// encodings, which puts identical ones together and ties only identical ones,
// so two sets holding the same members hold them alike, repeats and all;
// counting each member's repeats in both compared every pair.
func TestConformance_EQ041_UnknownMembersAreComparedInTheirOrder(t *testing.T) {
	conformance.Covers(t, "EQ-041", "EQ-044", "EQ-010")
	const size = 400
	num := tenon.NumberType()
	elem := tenon.Tuple(counting, num)
	member := func(i int64) tenon.Value {
		return tenon.TupleVal(tenon.CapsuleVal(counting, &i), tenon.Unknown(num))
	}
	members := func(from int64) []tenon.Value {
		out := make([]tenon.Value, size)
		for i := range out {
			// Every member twice: a set keeps members that are not known
			// apart, however alike.
			out[i] = member(from + int64(i/2))
		}
		return out
	}
	forwards := members(0)
	backwards := slices.Clone(forwards)
	slices.Reverse(backwards)
	a, b := tenon.SetVal(elem, forwards...), tenon.SetVal(elem, backwards...)
	c := tenon.SetVal(elem, append(members(0)[:size-1], member(size))...)
	for _, tt := range []struct {
		name string
		x, y tenon.Value
		want bool
	}{
		{"the same members given in two orders", a, b, true},
		{"members differing in one", a, c, false},
	} {
		countingCompared = 0
		got := tenon.Identical(tt.x, tt.y)
		compared := countingCompared
		if got != tt.want {
			t.Errorf("%s: Identical is %t, want %t", tt.name, got, tt.want)
		}
		if compared > 2*size {
			t.Errorf("%s: %d comparisons over %d members, want at most %d", tt.name, compared, size, 2*size)
		}
	}
}

func TestConformance_EQ045_KnownMembersAreComparedInTheirOrder(t *testing.T) {
	conformance.Covers(t, "EQ-044", "EQ-045", "EQ-003", "EQ-010", "SE-005")
	const size = 400
	set := tenon.Set(counting)
	value := func(i int64) tenon.Value { return tenon.CapsuleVal(counting, &i) }
	members := func(from, count int64) []tenon.Value {
		out := make([]tenon.Value, count)
		for i := range out {
			out[i] = value(from + int64(i))
		}
		return out
	}
	// The same members, given in two orders, and a set that differs in one.
	forwards := members(0, size)
	backwards := slices.Clone(members(0, size))
	slices.Reverse(backwards)
	a, b := tenon.SetVal(counting, forwards...), tenon.SetVal(counting, backwards...)
	c := tenon.SetVal(counting, append(members(0, size-1), value(size))...)

	// Two sets whose members are partly known, listed in a range: the
	// document is canonical, and deciding what it says compares the members
	// of the two sets with each other.
	partly := func(extra int64) tenon.Value {
		return tenon.SetVal(counting, append(members(0, size-1), value(extra), tenon.Unknown(counting))...)
	}
	listing := tenon.Narrow(tenon.Unknown(tenon.Set(set)), tenon.Members(partly(size), partly(size+1)))
	listed, failure, ok := tenon.Serialize(listing)
	if !ok {
		t.Fatalf("Serialize(a listing of two partly known sets) failed: %v", failure)
	}
	// A document holding two sets that hold the same members: the outer value
	// is a set, so the two are one member and the document is not its own
	// encoding, which the decoder finds out by comparing them.
	pair, failure, ok := tenon.Serialize(tenon.ListVal(set, a, b))
	if !ok {
		t.Fatalf("Serialize(a list of two sets) failed: %v", failure)
	}
	pair = bytes.Clone(pair)
	at := bytes.Index(pair, []byte{0x82, 0x04, 0x82, 0x05})
	if at < 0 {
		t.Fatalf("no list-of-set type in %x", pair[:16])
	}
	pair[at+1] = 0x05 // the outer list type becomes a set type

	read := tenon.Decoders{Capsules: []tenon.Type{counting}}
	var decoded tenon.Value
	for _, tt := range []struct {
		name string
		want string
		// call does the work being counted and says what it got; every
		// comparison it makes is one of the capsule type's.
		call func() string
		// budget is how many comparisons it may make over size members. One
		// operation compares each member a few times at most; a document
		// decoded is several operations, since the two sets it holds are
		// built, told apart, recorded and encoded again.
		budget int
	}{
		{"Equals of two sets of the same members", "true", func() string { return tenon.Equals(a, b).String() }, 4 * size},
		{"Equals of two sets differing in one member", "false", func() string { return tenon.Equals(a, c).String() }, 4 * size},
		{"Identical of two sets of the same members", "true", func() string { return fmt.Sprint(tenon.Identical(a, b)) }, 4 * size},
		{"Identical of two sets differing in one member", "false", func() string { return fmt.Sprint(tenon.Identical(a, c)) }, 4 * size},
		{"Contains", "true", func() string { return tenon.Contains(a, value(size/2)).String() }, 4 * size},
		{"decoding a document of two sets holding the same members", string(tenon.CodeSerializeNotCanonical), func() string {
			_, failure, ok := tenon.Deserialize(pair, read)
			if ok {
				return "decoded"
			}
			return string(failure.Diagnostics()[0].Code)
		}, 8 * size},
		{"decoding a listing of two partly known sets", "true", func() string {
			var failure tenon.Value
			var ok bool
			decoded, failure, ok = tenon.Deserialize(listed, read)
			if !ok {
				return "refused: " + failure.Diagnostics()[0].Message
			}
			return "true"
		}, 20 * size},
	} {
		countingCompared = 0
		got := tt.call()
		compared := countingCompared
		if got != tt.want {
			t.Errorf("%s gave %s, want %s", tt.name, got, tt.want)
		}
		// Comparing every pair would be one hundred and sixty thousand.
		if compared > tt.budget {
			t.Errorf("%s: %d comparisons over %d members, want at most %d", tt.name, compared, size, tt.budget)
		}
	}
	// What came back is the value that was written, whatever the counting.
	if !tenon.Identical(decoded, listing) {
		t.Errorf("the listing came back as %v", decoded)
	}
}

// BenchmarkSetComparisons measures comparing two sets of known members, built
// apart, at a size and four times it: the growth from one to the other is the
// reading, not the wall clock.
func BenchmarkSetComparisons(b *testing.B) {
	num := tenon.NumberType()
	for _, size := range []int{2000, 8000} {
		build := func() tenon.Value {
			members := make([]tenon.Value, size)
			for i := range members {
				members[i] = tenon.NumberFromInt(int64(i))
			}
			return tenon.SetVal(num, members...)
		}
		x, y := build(), build()
		b.Run(fmt.Sprintf("equals/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if v := tenon.Equals(x, y); v.IsError() {
					b.Fatal(v)
				}
			}
		})
		b.Run(fmt.Sprintf("identical/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if !tenon.Identical(x, y) {
					b.Fatal("the sets differ")
				}
			}
		})
	}
}

// TestConformance_UN005_ASetOverFewValuesDecidesThemOnce holds what a set
// over an element type holding few values costs: the values that type holds
// are built once for the type rather than for every such set, and what the
// set does not hold already is worked out once rather than looked for again
// for each member that is not known.
func TestConformance_UN005_ASetOverFewValuesDecidesThemOnce(t *testing.T) {
	conformance.Covers(t, "UN-005", "EQ-041", "SE-005")
	boo := tenon.BoolType()
	five := func(elem tenon.Type) tenon.Type { return tenon.Tuple(elem, elem, elem, elem, elem) }
	// Two documents of the same shape and nearly the same size: one over an
	// element type holding 243 values, one over a type holding more than can
	// be counted. Each set holds one member that is not known, which is what
	// sets the first one deciding what its members leave open.
	document := func(elem tenon.Type) []byte {
		const sets = 1000
		members := make([]tenon.Value, sets)
		for i := range members {
			members[i] = tenon.SetVal(elem, tenon.Unknown(elem))
		}
		doc, failure, ok := tenon.Serialize(tenon.ListVal(tenon.Set(elem), members...))
		if !ok {
			t.Fatalf("Serialize(%d sets of %v) failed: %v", sets, elem, failure)
		}
		return doc
	}
	allocated := func(doc []byte) uint64 {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		if _, failure, ok := tenon.Deserialize(doc, decoders); !ok {
			t.Fatalf("a document of sets did not decode: %v", failure)
		}
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	few, many := allocated(document(five(boo))), allocated(document(five(num)))
	if grew := float64(few) / float64(many); grew > 4 {
		t.Errorf("sets over a type of 243 values allocated %.1f times what sets over a type of uncountably many did (%d bytes against %d)",
			grew, few, many)
	}

	// A set holding every value of its element type but null, beside members
	// that are not known: each of those could still be null, so the set keeps
	// them, and deciding that asked every value about every member before.
	elem := five(boo)
	full := tuplesOfBools(boo)
	members := append(slices.Clone(full), make([]tenon.Value, 2000)...)
	for i := len(full); i < len(members); i++ {
		members[i] = tenon.Unknown(elem)
	}
	set := tenon.SetVal(elem, members...)
	if got, want := set.Len(), len(members); got != want {
		t.Errorf("a set of every value but null, beside %d unknowns, holds %d members, want %d", len(members)-len(full), got, want)
	}
	// And a set that holds null as well leaves them nothing to be at all.
	withNull := tenon.SetVal(elem, append(slices.Clone(members), tenon.NullVal(elem))...)
	if got, want := withNull.Len(), len(full)+1; got != want {
		t.Errorf("a set of every value, beside %d unknowns, holds %d members, want %d", len(members)-len(full), got, want)
	}
	if !withNull.IsKnown() {
		t.Errorf("a set holding every value its members can be is %v, want a known value", withNull)
	}
}

// tuplesOfBools returns every value a tuple of five Bool elements can be,
// null aside: each element is true, false or the null of Bool, which is
// three to the fifth, 243 of them.
func tuplesOfBools(boo tenon.Type) []tenon.Value {
	each := []tenon.Value{tenon.Bool(false), tenon.Bool(true), tenon.NullVal(boo)}
	rows := [][]tenon.Value{nil}
	for range 5 {
		var next [][]tenon.Value
		for _, row := range rows {
			for _, v := range each {
				next = append(next, append(append([]tenon.Value{}, row...), v))
			}
		}
		rows = next
	}
	out := make([]tenon.Value, len(rows))
	for i, row := range rows {
		out[i] = tenon.TupleVal(row...)
	}
	return out
}

// TestSetsOverFewValuesConcurrent holds the values a type keeps for the sets
// over it to being built from several goroutines at once: every set comes out
// the same, whichever goroutine built the values first, and the race detector
// sees the writing and the reading.
func TestSetsOverFewValuesConcurrent(t *testing.T) {
	const workers = 16
	// A type of this run's own, so that the values it keeps are built here.
	boo := tenon.BoolType()
	elem := tenon.Tuple(boo, boo, tenon.Tuple(boo, boo))
	held := tenon.TupleVal(tenon.Bool(true), tenon.NullVal(boo), tenon.TupleVal(tenon.Bool(false), tenon.Bool(true)))
	results := make([]tenon.Value, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for range 50 {
				results[w] = tenon.SetVal(elem, held, tenon.Unknown(elem), tenon.Narrow(tenon.Unknown(elem), tenon.NotNull()))
			}
		})
	}
	wg.Wait()
	for w, got := range results {
		if !tenon.Identical(got, results[0]) {
			t.Errorf("worker %d built %v, where worker 0 built %v", w, got, results[0])
		}
	}
	if results[0].Len() != 3 {
		t.Errorf("the set holds %d members, want 3", results[0].Len())
	}
}
