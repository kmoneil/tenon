package tenon_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
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
	one, two, three, five := tenon.NumberFromInt(1), tenon.NumberFromInt(2), tenon.NumberFromInt(3), tenon.NumberFromInt(5)
	str, num := tenon.StringType(), tenon.NumberType()
	lst := tenon.List(str)
	// A set of one or two members: the unknown may turn out to be 1.
	partial := tenon.SetVal(num, one, tenon.Unknown(num))
	atLeastFive := tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(five, true))
	between := func(lo, hi string) tenon.Value {
		return tenon.Narrow(tenon.Unknown(num), tenon.NotNull(),
			tenon.NumberMin(tenon.NumberFromText(lo), true), tenon.NumberMax(tenon.NumberFromText(hi), true))
	}
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
			"the value null(string) does not satisfy not null",
		},

		// A set holding a member that is not known has a length that is a
		// range, and the narrowings it is given are decided together against
		// that range.
		{
			"a set that cannot be null", partial,
			[]tenon.Narrowing{tenon.Null()},
			"the value set(number)[1, unknown(number)] does not satisfy null",
		},
		{
			"a set shorter than it can be", partial,
			[]tenon.Narrowing{tenon.LengthMax(0)},
			"the value set(number)[1, unknown(number)] does not satisfy length <= 0",
		},
		{
			"a set longer than it can be", partial,
			[]tenon.Narrowing{tenon.LengthMin(3)},
			"the value set(number)[1, unknown(number)] does not satisfy length >= 3",
		},
		{
			"lengths the set can have, bounds that cross", partial,
			[]tenon.Narrowing{tenon.LengthMin(2), tenon.LengthMax(1)},
			"the value set(number)[1, unknown(number)] does not satisfy both length >= 2 and length <= 1",
		},
		{
			"the same bounds the other way round", partial,
			[]tenon.Narrowing{tenon.LengthMax(1), tenon.LengthMin(2)},
			"the value set(number)[1, unknown(number)] does not satisfy both length <= 1 and length >= 2",
		},
		{
			"more listed members than the set holds", partial,
			[]tenon.Narrowing{tenon.Members(three, two)},
			"the value set(number)[1, unknown(number)] does not satisfy members {2, 3}",
		},
		{
			"a listed member the set can hold, and a length that leaves no room for it", partial,
			[]tenon.Narrowing{tenon.Members(two), tenon.LengthMax(1)},
			"the value set(number)[1, unknown(number)] does not satisfy both members {2} and length <= 1",
		},
		{
			"the same listing after the length", partial,
			[]tenon.Narrowing{tenon.LengthMax(1), tenon.Members(two)},
			"the value set(number)[1, unknown(number)] does not satisfy both length <= 1 and members {2}",
		},
		// Where more than one thing sets the bound that is crossed, the
		// message names the members, which the value shows, ahead of a
		// listing, and a listing ahead of a bound given, as a range's message
		// names a listing ahead of a bound.
		{
			"a least length the members set as well as a bound", partial,
			[]tenon.Narrowing{tenon.LengthMin(1), tenon.LengthMax(0)},
			"the value set(number)[1, unknown(number)] does not satisfy length <= 0",
		},
		{
			"a greatest length the members set as well as a bound", partial,
			[]tenon.Narrowing{tenon.LengthMax(2), tenon.LengthMin(3)},
			"the value set(number)[1, unknown(number)] does not satisfy length >= 3",
		},
		{
			"a least length a listing sets as well as a bound", partial,
			[]tenon.Narrowing{tenon.LengthMin(2), tenon.Members(two), tenon.LengthMax(1)},
			"the value set(number)[1, unknown(number)] does not satisfy both members {2} and length <= 1",
		},
		{
			// The first listing needs 1 and two more members, since its values
			// are provably distinct from 1 and from each other. The second
			// lists a value either of them could be, which sorts ahead of both
			// and counts in their place, but they are needed all the same.
			"a listing that counts fewer beside an earlier one",
			tenon.SetVal(num, one, tenon.Unknown(num), tenon.Unknown(num)),
			[]tenon.Narrowing{
				tenon.Members(between("5", "6"), between("8", "9")),
				tenon.Members(between("4.5", "9")),
				tenon.LengthMax(2),
			},
			"the value set(number)[1, unknown(number), ... does not satisfy both members {unknown(number, not nul... and length <= 2",
		},
		{
			"listings that need more members together than the set holds", partial,
			[]tenon.Narrowing{tenon.Members(two), tenon.Members(three)},
			"the value set(number)[1, unknown(number)] does not satisfy members {2, 3}",
		},
		{
			// Two members could be 3 and one more, but neither can be 3.
			"a listed member the set provably lacks", tenon.SetVal(num, atLeastFive, atLeastFive),
			[]tenon.Narrowing{tenon.Members(three)},
			"the value set(number)[unknown(number, not ... does not satisfy members {3}",
		},
		{
			// Two listed values that are provably distinct need two members.
			// Counted beside the member, which could be either of them and
			// comes first, they would count as one.
			"listed values the set has too few members for", tenon.SetVal(num, tenon.Unknown(num)),
			[]tenon.Narrowing{tenon.Members(atLeastFive, tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMax(one, true)))},
			"the value set(number)[unknown(number)] does not satisfy members {unknown(number, not nul...",
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

func TestConformance_UN004_ASetHoldingUnknownsHasTheLengthsItCanHave(t *testing.T) {
	conformance.Covers(t, "UN-004", "UN-007")
	num := tenon.NumberType()
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	unknown := tenon.Unknown(num)
	atLeastFive := tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(n(5), true))

	// A set holding members that are not known has a length that is a range,
	// from the count of its members that are provably distinct to the count
	// of all of them. A narrowing to lengths in that range leaves the set as
	// it was, since it has nowhere to record what that says of its members.
	for _, tt := range []struct {
		name string
		v    tenon.Value
		ns   []tenon.Narrowing
	}{
		// 0.2.0 contradicted the first two, counting the members held.
		{"two unknowns that may be one member", tenon.SetVal(num, unknown, unknown), []tenon.Narrowing{tenon.LengthMax(1)}},
		{
			"an unknown that may be either of two others", tenon.SetVal(num, n(1), atLeastFive, unknown),
			[]tenon.Narrowing{tenon.LengthMax(2)},
		},
		{"two unknowns that may be two members", tenon.SetVal(num, unknown, unknown), []tenon.Narrowing{tenon.LengthMin(2)}},
		{"exactly two", tenon.SetVal(num, unknown, unknown), []tenon.Narrowing{tenon.LengthMin(2), tenon.LengthMax(2)}},
		{
			"every length it can have, and not null", tenon.SetVal(num, unknown, unknown),
			[]tenon.Narrowing{tenon.NotNull(), tenon.LengthMin(1), tenon.LengthMax(2)},
		},
		{"a listed value an unknown may be", tenon.SetVal(num, unknown, unknown), []tenon.Narrowing{tenon.Members(n(1))}},
		{
			"a listed value a known member is, and room for another", tenon.SetVal(num, n(1), unknown),
			[]tenon.Narrowing{tenon.Members(n(1)), tenon.LengthMin(2)},
		},
	} {
		if got := tenon.Narrow(tt.v, tt.ns...); !tenon.Identical(got, tt.v) {
			t.Errorf("%s: narrowed %v to %v, want it as it was", tt.name, tt.v, got)
		}
	}

	// Sets of up to three members, each a small integer or an unknown bounded
	// among them, some of which may be null, are narrowed every way and checked
	// against the sets each could turn out to be: a contradiction leaves no
	// such set that satisfies the narrowings, a known result is the only one
	// that does, and any other result is the set as it was. An unknown is
	// tried at every integer and half-integer in its range, and at null where
	// it may be null, which lets three members be all alike, all apart, or
	// anything between wherever their ranges allow.
	type member struct {
		lo, hi int64
		null   bool // whether the member may be null
	}
	kinds := []member{
		{lo: 0, hi: 0}, {lo: 1, hi: 1},
		{lo: 0, hi: 1}, {lo: 1, hi: 2}, {lo: 0, hi: 2},
		{lo: 0, hi: 1, null: true}, {lo: 2, hi: 3, null: true},
	}
	var specs [][]member
	for i, a := range kinds {
		specs = append(specs, []member{a})
		for j, b := range kinds[i:] {
			specs = append(specs, []member{a, b})
			for _, c := range kinds[i+j:] {
				specs = append(specs, []member{a, b, c})
			}
		}
	}
	value := func(m member) tenon.Value {
		switch {
		case m.lo == m.hi:
			return n(m.lo)
		case m.null:
			return tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(m.lo), true), tenon.NumberMax(n(m.hi), true))
		}
		return tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(n(m.lo), true), tenon.NumberMax(n(m.hi), true))
	}
	build := func(spec []member) tenon.Value {
		elems := make([]tenon.Value, len(spec))
		for i, m := range spec {
			elems[i] = value(m)
		}
		return tenon.SetVal(num, elems...)
	}
	// choices returns the sets spec is tried as.
	choices := func(spec []member) []tenon.Value {
		var out []tenon.Value
		var walk func(i int, picked []tenon.Value)
		walk = func(i int, picked []tenon.Value) {
			if i == len(spec) {
				out = append(out, tenon.SetVal(num, picked...))
				return
			}
			if spec[i].null {
				walk(i+1, append(slices.Clone(picked), tenon.NullVal(num)))
			}
			for half := 2 * spec[i].lo; half <= 2*spec[i].hi; half++ {
				v := n(half / 2)
				if half%2 != 0 {
					v = tenon.NumberFromText(strconv.FormatInt(half/2, 10) + ".5")
				}
				walk(i+1, append(slices.Clone(picked), v))
			}
		}
		walk(0, nil)
		return out
	}
	var narrowings [][]tenon.Narrowing
	listings := [][]tenon.Value{
		nil, {n(0)}, {n(3)}, {n(0), n(1)}, {n(1), n(2)}, {tenon.NullVal(num)},
		{value(member{lo: 2, hi: 3})}, {n(0), value(member{lo: 2, hi: 3})},
	}
	for lo := int64(-1); lo <= 3; lo++ {
		for hi := int64(-1); hi <= 3; hi++ {
			for _, listed := range listings {
				var ns []tenon.Narrowing
				if lo >= 0 {
					ns = append(ns, tenon.LengthMin(lo))
				}
				if hi >= 0 {
					ns = append(ns, tenon.LengthMax(hi))
				}
				if listed != nil {
					ns = append(ns, tenon.Members(listed...))
				}
				narrowings = append(narrowings, ns)
			}
		}
	}
	contradicted, pinned, kept := 0, 0, 0
	for _, spec := range specs {
		set := build(spec)
		if set.IsKnown() {
			continue // every member is known, so it has one length
		}
		sets := choices(spec)
		for _, ns := range narrowings {
			got := tenon.Narrow(set, ns...)
			var allowed []tenon.Value
			for _, s := range sets {
				if !tenon.Narrow(s, ns...).IsError() {
					allowed = append(allowed, s)
				}
			}
			switch {
			case got.IsError():
				contradicted++
				if code := got.Diagnostics()[0].Code; code != tenon.CodeRangeContradiction {
					t.Errorf("%v narrowed by %v: code %s, want %s", set, ns, code, tenon.CodeRangeContradiction)
				}
				if len(allowed) > 0 {
					t.Errorf("%v narrowed by %v is a contradiction, but %v satisfies them", set, ns, allowed[0])
				}
			case got.IsKnown():
				pinned++
				if len(allowed) == 0 {
					t.Errorf("%v narrowed by %v is %v, but no set it could be satisfies them", set, ns, got)
				}
				for _, s := range allowed {
					if !tenon.Identical(s, got) {
						t.Errorf("%v narrowed by %v is %v, but %v satisfies them too", set, ns, got, s)
					}
				}
			default:
				kept++
				if !tenon.Identical(got, set) {
					t.Errorf("%v narrowed by %v is %v, want the set as it was", set, ns, got)
				}
			}
			// The narrowings are decided together, so their order does not
			// change what they leave.
			back := slices.Clone(ns)
			slices.Reverse(back)
			if again := tenon.Narrow(set, back...); again.IsError() != got.IsError() ||
				!got.IsError() && !tenon.Identical(again, got) {
				t.Errorf("%v narrowed by %v is %v, but narrowed by %v it is %v", set, ns, got, back, again)
			}
		}
	}
	if contradicted < 100 || pinned < 100 || kept < 100 {
		t.Errorf("too few of each result to say much: %d contradictions, %d known, %d kept", contradicted, pinned, kept)
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
		{null, true, "null(string)", "the null value of type string"},
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
	mustPanicUsage(t, "does not apply to a pending value, whose type is not determined", func() {
		tenon.Narrow(tenon.Pending(tenon.Any()), tenon.LengthMax(3))
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
	bounded := func(ns ...tenon.Narrowing) tenon.Value {
		return tenon.Narrow(tenon.Unknown(num), append([]tenon.Narrowing{tenon.NotNull()}, ns...)...)
	}
	// Two sets that equality cannot tell apart, although the second can never
	// be the first: its unknowns would need their one member between 3 and 4
	// to be both of them.
	n := tenon.NumberFromInt
	fourKnown := tenon.SetVal(num, n(1), n(2), n(3), n(4))
	oneOrTwo := bounded(tenon.NumberMin(n(1), true), tenon.NumberMax(n(2), true))
	fourUnfit := tenon.SetVal(num, oneOrTwo, oneOrTwo, oneOrTwo, bounded(tenon.NumberMin(n(3), true), tenon.NumberMax(n(4), true)))
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
			"an object with no attributes", tenon.Unknown(tenon.Object(nil)),
			[]tenon.Narrowing{tenon.NotNull()},
			tenon.ObjectVal(nil),
		},

		// A set holding members that are not known, left no more members than
		// its known ones, holds those alone: every other member must turn out
		// to be one of them.
		{
			"a set left its known member", tenon.SetVal(num, one, tenon.Unknown(num)),
			[]tenon.Narrowing{tenon.LengthMax(1)},
			tenon.SetVal(num, one),
		},
		{
			"a set left its known members, which hold what is listed",
			tenon.SetVal(num, one, five, tenon.Unknown(num), bounded(tenon.NumberMax(five, false))),
			[]tenon.Narrowing{tenon.NotNull(), tenon.Members(five), tenon.LengthMax(2)},
			tenon.SetVal(num, one, five),
		},
		{
			"a set left its null member", tenon.SetVal(num, tenon.NullVal(num), tenon.Unknown(num)),
			[]tenon.Narrowing{tenon.LengthMax(1)},
			tenon.SetVal(num, tenon.NullVal(num)),
		},
		{
			"a set left its known list",
			tenon.SetVal(tenon.List(num), tenon.ListVal(num, one, five), tenon.ListVal(num, tenon.Unknown(num), five)),
			[]tenon.Narrowing{tenon.LengthMax(1)},
			tenon.SetVal(tenon.List(num), tenon.ListVal(num, one, five)),
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
			// Each member of the pair may be the empty tuple or null.
			"members whose types have one value besides null", tenon.Unknown(tenon.Tuple(empty, empty)),
			[]tenon.Narrowing{tenon.NotNull()},
		},
		{
			"an attribute whose type has one value besides null",
			tenon.Unknown(tenon.Object(map[string]tenon.Type{"a": tenon.Object(nil)})),
			[]tenon.Narrowing{tenon.NotNull()},
		},
		{
			// A list of exactly three empty tuples holds one value too, but
			// finding that out means building a value as large as the bounds
			// allow, which the rule permits an implementation to decline.
			"a length that pins a collection", tenon.Unknown(tenon.List(empty)),
			[]tenon.Narrowing{tenon.NotNull(), tenon.LengthMin(3), tenon.LengthMax(3)},
		},
		{
			// The second unknown is provably not 1, so the set holds two
			// members, and one of them is not known.
			"a set left room for a member that is not known",
			tenon.SetVal(num, one, bounded(tenon.NumberMin(five, true)), tenon.Unknown(num)),
			[]tenon.Narrowing{tenon.LengthMax(2)},
		},
		{
			"a set with no known member to be left", tenon.SetVal(num, tenon.Unknown(num), tenon.Unknown(num)),
			[]tenon.Narrowing{tenon.LengthMax(1)},
		},
		{
			// A length of one leaves no set at all, and Narrow, which cannot
			// tell that either, must not answer with the first member alone.
			"a set of sets", tenon.SetVal(tenon.Set(num), fourKnown, fourUnfit),
			[]tenon.Narrowing{tenon.LengthMax(1)},
		},
	} {
		if got := tenon.Narrow(tt.v, tt.ns...); got.IsKnown() {
			t.Errorf("%s: narrowed to the known value %v", tt.name, got)
		}
	}
	// Nor where those two sets lie deeper within the members.
	for _, wrap := range []func(tenon.Value) tenon.Value{
		func(s tenon.Value) tenon.Value { return tenon.ListVal(s.Type(), s) },
		func(s tenon.Value) tenon.Value { return tenon.MapVal(s.Type(), map[string]tenon.Value{"k": s}) },
		func(s tenon.Value) tenon.Value { return tenon.TupleVal(s) },
		func(s tenon.Value) tenon.Value {
			return tenon.ObjectVal(map[string]tenon.Value{"a": tenon.TupleVal(s)})
		},
	} {
		a, b := wrap(fourKnown), wrap(fourUnfit)
		if got := tenon.Narrow(tenon.SetVal(a.Type(), a, b), tenon.LengthMax(1)); got.IsKnown() {
			t.Errorf("a set of %v narrowed to one member is the known value %v", a.Type(), got)
		}
	}
	// Both values that range holds exist.
	pair := tenon.Tuple(empty, empty)
	for _, v := range []tenon.Value{tenon.TupleVal(tenon.TupleVal(), tenon.TupleVal()), tenon.TupleVal(tenon.NullVal(empty), tenon.TupleVal())} {
		if v.Type() != pair || !v.IsKnown() {
			t.Errorf("%v is not a known value of %v", v, pair)
		}
	}
}

func TestConformance_UN002_MembersNarrowing(t *testing.T) {
	conformance.Covers(t, "UN-002")
	num := tenon.NumberType()
	set := tenon.Set(num)
	n := func(i int64) tenon.Value { return tenon.NumberFromInt(i) }
	one, two, three := n(1), n(2), n(3)
	// Listed members that cannot be null. Null is the one value every nullable
	// range of a type still holds, so two of them are never provably distinct
	// however far apart their numbers are; the case below lists two that are.
	atLeast := func(i int64) tenon.Value {
		return tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(n(i), true))
	}
	atMostZero := tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMax(n(0), true))

	// The recorded members are canonical: values that are one member appear
	// once, the order is the order a set iterates in, and the least length
	// they imply is recorded with them.
	a := tenon.Narrow(tenon.Unknown(set), tenon.Members(two, tenon.NumberFromText("1.000"), one))
	if got, want := a.String(), "unknown(set(number), length >= 2, members {1, 2})"; got != want {
		t.Errorf("recorded members render as %s, want %s", got, want)
	}
	b := tenon.Narrow(tenon.Unknown(set), tenon.Members(one), tenon.Members(two))
	if !tenon.Identical(a, b) {
		t.Errorf("%v and %v record one listing two ways, but are not identical", a, b)
	}
	if got := tenon.Narrow(a, tenon.Members(one, two)); got != a {
		t.Errorf("listing recorded members again produced %v, want the range unchanged", got)
	}
	if got, want := tenon.Narrow(tenon.Unknown(set), tenon.Members(tenon.NullVal(num), one)).String(),
		"unknown(set(number), length >= 2, members {null(number), 1})"; got != want {
		t.Errorf("a listed null member renders as %s, want %s", got, want)
	}
	// The narrowing says nothing about the set being null, so the range
	// keeps null until NotNull takes it away.
	if !a.Range().AllowsNull() {
		t.Errorf("%v excludes null, but only NotNull excludes null", a)
	}

	// Membership through the narrowing: known true for a recorded member once
	// null is excluded, unknown for anything else, and unknown for everything
	// while the set could still be null, which would make the answer an error.
	nn := tenon.Narrow(a, tenon.NotNull())
	if got := tenon.Contains(nn, one).String(); got != "true" {
		t.Errorf("Contains of a recorded member is %s, want true", got)
	}
	if got := tenon.Contains(nn, three); got.IsKnown() {
		t.Errorf("Contains of an unlisted value is %v, want an unknown Bool", got)
	}
	if got := tenon.Contains(a, one); got.IsKnown() {
		t.Errorf("Contains on a possibly-null set is %v, want an unknown Bool", got)
	}

	// The length of the set answers from the recorded members.
	if got, want := tenon.Length(nn).String(), "unknown(number, not null, >= 2)"; got != want {
		t.Errorf("Length is %s, want %s", got, want)
	}

	// Listed values raise the least length only where they are provably
	// distinct, and a pair with identical ranges is recorded once.
	distinct := tenon.Narrow(tenon.Unknown(set), tenon.Members(atLeast(5), atMostZero))
	if got, want := distinct.String(),
		"unknown(set(number), length >= 2, members {unknown(number, not null, <= 0), unknown(number, not null, >= 5)})"; got != want {
		t.Errorf("provably distinct members render as %s, want %s", got, want)
	}
	// The same two ranges while each still holds null are not provably
	// distinct: both could turn out to be null, which is one member.
	nullable := tenon.Narrow(tenon.Unknown(set), tenon.Members(
		tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true)),
		tenon.Narrow(tenon.Unknown(num), tenon.NumberMax(n(0), true))))
	if got, want := nullable.String(),
		"unknown(set(number), length >= 1, members {unknown(number, <= 0), unknown(number, >= 5)})"; got != want {
		t.Errorf("members that could each be null render as %s, want %s", got, want)
	}
	overlap := tenon.Narrow(tenon.Unknown(set), tenon.Members(atLeast(5), atLeast(6)))
	if got, want := overlap.String(),
		"unknown(set(number), length >= 1, members {unknown(number, not null, >= 5), unknown(number, not null, >= 6)})"; got != want {
		t.Errorf("possibly-equal members render as %s, want %s", got, want)
	}
	twice := tenon.Narrow(tenon.Unknown(set), tenon.Members(atLeast(5), atLeast(5)))
	if got, want := twice.String(),
		"unknown(set(number), length >= 1, members {unknown(number, not null, >= 5)})"; got != want {
		t.Errorf("identical listed values render as %s, want %s", got, want)
	}

	// A listed value whose range excludes nothing promises only that a member
	// exists, so it is recorded as the least length it implies.
	vacuous := tenon.Narrow(tenon.Unknown(set), tenon.Members(tenon.Unknown(num)))
	if got, want := vacuous.String(), "unknown(set(number), length >= 1)"; got != want {
		t.Errorf("a member that could be anything renders as %s, want %s", got, want)
	}
	if !tenon.Identical(vacuous, tenon.Narrow(tenon.Unknown(set), tenon.LengthMin(1))) {
		t.Error("a member that could be anything and LengthMin(1) spell one range two ways")
	}

	// More provably distinct members than the greatest length allows is a
	// contradiction, in whichever order the two narrowings arrive.
	for _, ns := range [][]tenon.Narrowing{
		{tenon.Members(one, two), tenon.LengthMax(1)},
		{tenon.LengthMax(1), tenon.Members(one, two)},
	} {
		got := tenon.Narrow(tenon.Unknown(set), ns...)
		if !got.IsError() {
			t.Fatalf("narrowing to nothing produced %v, want an error value", got)
		}
		if diags := got.Diagnostics(); diags[0].Code != tenon.CodeRangeContradiction {
			t.Errorf("code %s, want %s", diags[0].Code, tenon.CodeRangeContradiction)
		}
	}

	// As many provably distinct members as the greatest length allows leaves
	// exactly the set holding them: known members make a known set, members
	// that are not known make the set that holds them, still not known, and
	// members that could turn out to be one member leave the range standing.
	full := tenon.Narrow(tenon.Unknown(set), tenon.NotNull(), tenon.Members(one, two), tenon.LengthMax(2))
	if !full.IsKnown() || !tenon.Identical(full, tenon.SetVal(num, one, two)) {
		t.Errorf("a full listing of known members produced %v, want the set holding them", full)
	}
	held := tenon.Narrow(tenon.Unknown(set), tenon.NotNull(),
		tenon.Members(atLeast(5), atMostZero), tenon.LengthMax(2))
	if held.IsKnown() || !tenon.Identical(held, tenon.SetVal(num, atLeast(5), atMostZero)) {
		t.Errorf("a full listing of distinct unknowns produced %v, want the set holding them", held)
	}
	loose := tenon.Narrow(tenon.Unknown(set), tenon.NotNull(),
		tenon.Members(atLeast(5), atLeast(6)), tenon.LengthMax(2))
	if got, want := loose.String(),
		"unknown(set(number), not null, length >= 1, length <= 2, members {unknown(number, not null, >= 5), unknown(number, not null, >= 6)})"; got != want {
		t.Errorf("members that could be one render as %s, want %s", got, want)
	}

	// A known set is a range of one: a listing it could satisfy leaves it,
	// one it provably cannot contradicts it, and the null set satisfies any
	// listing vacuously, since a narrowing says nothing about null.
	s12 := tenon.SetVal(num, one, two)
	if got := tenon.Narrow(s12, tenon.Members(one), tenon.Members(tenon.Unknown(num))); got != s12 {
		t.Errorf("narrowing a known set it could satisfy produced %v, want the set itself", got)
	}
	if got := tenon.Narrow(s12, tenon.Members(three)); !got.IsError() {
		t.Errorf("narrowing a known set by a member it provably lacks produced %v, want an error value", got)
	}
	nullSet := tenon.NullVal(set)
	if got := tenon.Narrow(nullSet, tenon.Members(one)); got != nullSet {
		t.Errorf("narrowing the null set produced %v, want the value itself", got)
	}

	// Mistakes in the calling program panic: a Members narrowing on a type
	// that is not a set, a member of another type than the set's, and a
	// listed value that is not resolved.
	mustPanicUsage(t, "does not apply to a value of type list(number)", func() {
		tenon.Narrow(tenon.Unknown(tenon.List(num)), tenon.Members(one))
	})
	mustPanicUsage(t, "are of type number", func() {
		tenon.Narrow(tenon.Unknown(set), tenon.Members(tenon.String("x")))
	})
	mustPanicUsage(t, "not a resolved value", func() {
		tenon.Members(tenon.Pending(tenon.Any()))
	})
	mustPanicUsage(t, "not a resolved value", func() {
		tenon.Members(tenon.ErrorVal(tenon.Diagnostic{Code: "app.x", Message: "m"}))
	})
}
