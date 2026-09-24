package tenon_test

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/conformance/values"
)

// stamp is a Mark for tests: comparable, with a policy, a redaction flag, and
// whether it is deep.
type stamp struct {
	id     string
	policy tenon.Propagation
	redact bool
	deep   bool
}

func (m stamp) MarkID() string                 { return m.id }
func (m stamp) Propagation() tenon.Propagation { return m.policy }
func (m stamp) Redacting() bool                { return m.redact }
func (m stamp) Deep() bool                     { return m.deep }

// bare is a Mark that does not implement DeepMark at all.
type bare string

func (m bare) MarkID() string               { return string(m) }
func (bare) Propagation() tenon.Propagation { return tenon.Propagate }
func (bare) Redacting() bool                { return false }

// slippery is a Mark whose type is not comparable, which WithMarks refuses.
type slippery struct {
	stamp
	payload []byte
}

func TestConformance_MK001_MarksAreTypedMetadata(t *testing.T) {
	conformance.Covers(t, "MK-001")
	num := tenon.NumberType()
	secret := stamp{id: "secret", redact: true}
	origin := stamp{id: "origin"}
	one := tenon.NumberFromInt(1)

	// A mark declares what the interface asks of it.
	if secret.MarkID() != "secret" || secret.Propagation() != tenon.Propagate || !secret.Redacting() {
		t.Errorf("the mark %v does not declare what it was built with", secret)
	}

	// Attaching a mark makes a marked value and leaves the original alone.
	m1 := tenon.WithMarks(one, secret)
	if !tenon.HasMark(m1, secret) || tenon.HasMark(m1, origin) {
		t.Errorf("%v carries the wrong marks", m1)
	}
	if tenon.HasMark(one, secret) {
		t.Error("marking a value changed the original")
	}

	// The marked value is still the value: its content reads as before.
	if got, ok := m1.AsInt64(); !ok || got != 1 {
		t.Errorf("the marked value reads as %d, %t, want 1, true", got, ok)
	}

	// A mark attaches once however often it is given, and Unmark returns the
	// marks sorted by identifier.
	m2 := tenon.WithMarks(tenon.WithMarks(m1, secret), origin, secret)
	u, ms := tenon.Unmark(m2)
	if len(ms) != 2 || ms[0].MarkID() != "origin" || ms[1].MarkID() != "secret" {
		t.Errorf("Unmark returned %v, want origin then secret", ms)
	}
	if tenon.HasMark(u, secret) || tenon.HasMark(u, origin) {
		t.Errorf("%v still carries marks after Unmark", u)
	}

	// Unmarking an unmarked value and attaching nothing are both the value.
	if u2, ms2 := tenon.Unmark(one); u2 != one || ms2 != nil {
		t.Errorf("Unmark of an unmarked value returned %v, %v, want the value and no marks", u2, ms2)
	}
	if got := tenon.WithMarks(m1); got != m1 {
		t.Errorf("attaching no marks returned %v, want the value itself", got)
	}

	// Every state can carry a mark: marks are metadata, not content.
	for _, v := range []tenon.Value{
		tenon.NullVal(num),
		tenon.Unknown(num),
		tenon.Pending(tenon.Any()),
		tenon.ErrorVal(tenon.Diagnostic{Code: "app.x", Message: "m"}),
	} {
		if !tenon.HasMark(tenon.WithMarks(v, secret), secret) {
			t.Errorf("%v did not take a mark", v)
		}
	}

	// A mark must be comparable, and must not be nil.
	mustPanicUsage(t, "not comparable", func() {
		tenon.WithMarks(one, slippery{stamp: stamp{id: "s"}})
	})
	mustPanicUsage(t, "nil Mark", func() {
		tenon.WithMarks(one, nil)
	})
}

func TestConformance_MK002_PropagationPolicies(t *testing.T) {
	conformance.Covers(t, "MK-002")
	num := tenon.NumberType()
	prop := stamp{id: "prop"}
	iso := stamp{id: "iso", policy: tenon.Isolate}
	five := tenon.NumberFromInt(5)
	marked := tenon.WithMarks(tenon.NumberFromInt(1), prop, iso)

	// A Propagate mark appears on the result of every operation consuming
	// the marked value; an Isolate mark stays behind.
	for _, tt := range []struct {
		name string
		r    tenon.Value
	}{
		{"Add", tenon.Add(marked, five)},
		{"And", tenon.And(tenon.WithMarks(tenon.Bool(true), prop, iso), tenon.Bool(false))},
		{"Equals", tenon.Equals(marked, five)},
	} {
		if !tenon.HasMark(tt.r, prop) {
			t.Errorf("%s: the Propagate mark did not reach %v", tt.name, tt.r)
		}
		if tenon.HasMark(tt.r, iso) {
			t.Errorf("%s: the Isolate mark transferred to %v", tt.name, tt.r)
		}
	}
	if !tenon.HasMark(marked, iso) || !tenon.HasMark(marked, prop) {
		t.Error("the operand lost its own marks")
	}

	// Narrowing refines the value rather than deriving a new one, so every
	// mark stays, Isolate included, whether the result is still a range or
	// has come down to one value.
	u := tenon.WithMarks(tenon.Unknown(num), prop, iso)
	nu := tenon.Narrow(u, tenon.NotNull())
	if !tenon.HasMark(nu, prop) || !tenon.HasMark(nu, iso) {
		t.Errorf("narrowing dropped marks: %v", nu)
	}
	k := tenon.Narrow(u, tenon.NotNull(), tenon.NumberMin(five, true), tenon.NumberMax(five, true))
	if !k.IsKnown() {
		t.Fatalf("bounds that meet produced %v, want the known value", k)
	}
	if !tenon.HasMark(k, prop) || !tenon.HasMark(k, iso) {
		t.Errorf("collapsing to one value dropped marks: %v", k)
	}
	s := tenon.Narrow(tenon.WithMarks(tenon.SetVal(num, five, tenon.Unknown(num)), prop, iso), tenon.LengthMax(1))
	if !s.IsKnown() || !tenon.HasMark(s, prop) || !tenon.HasMark(s, iso) {
		t.Errorf("a set left its known member produced %v, want that set, known, with both marks", s)
	}

	// Resolving a pending value refines it the same way.
	p := tenon.WithMarks(tenon.Pending(tenon.Any()), prop, iso)
	rv := tenon.Resolve(p, num)
	if !tenon.HasMark(rv, prop) || !tenon.HasMark(rv, iso) {
		t.Errorf("resolving dropped marks: %v", rv)
	}
}

func TestConformance_MK003_ResultMarksAreTheUnion(t *testing.T) {
	conformance.Covers(t, "MK-003")
	num := tenon.NumberType()
	a, b, shared := stamp{id: "a"}, stamp{id: "b"}, stamp{id: "shared"}
	iso := stamp{id: "iso", policy: tenon.Isolate}
	x := tenon.WithMarks(tenon.NumberFromInt(1), a, shared, iso)
	y := tenon.WithMarks(tenon.NumberFromInt(2), b, shared)

	// The result carries each operand's Propagate marks, a mark on both
	// operands once, and no Isolate mark.
	sum := tenon.Add(x, y)
	if got, ok := sum.AsInt64(); !ok || got != 3 {
		t.Errorf("the marked sum reads as %d, %t, want 3, true", got, ok)
	}
	if _, ms := tenon.Unmark(sum); len(ms) != 3 {
		t.Errorf("the sum carries %v, want the union a, b, shared", ms)
	}
	for _, m := range []stamp{a, b, shared} {
		if !tenon.HasMark(sum, m) {
			t.Errorf("the sum is missing %v", m)
		}
	}
	if tenon.HasMark(sum, iso) {
		t.Error("the sum carries the Isolate mark")
	}

	// A result decided by one operand still carries the union: consuming is
	// what propagates, not deciding.
	f := tenon.And(tenon.WithMarks(tenon.Bool(false), a), tenon.WithMarks(tenon.Bool(true), b))
	if unmarked(f).String() != "false" || !tenon.HasMark(f, a) || !tenon.HasMark(f, b) {
		t.Errorf("And decided by false is %v with the wrong marks", f)
	}

	// An unknown result carries the union too.
	un := tenon.Equals(tenon.WithMarks(tenon.Unknown(num), a), tenon.NumberFromInt(3))
	if un.IsKnown() || !tenon.HasMark(un, a) {
		t.Errorf("the unknown result %v does not carry the operand's mark", un)
	}

	// A narrowing built from a value, as a bound is, passes that value's
	// Propagate marks on to the result as an operand would, whether or not the
	// bound changes the range.
	bounded := tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(tenon.WithMarks(tenon.NumberFromInt(1), a, iso), true))
	if !tenon.HasMark(bounded, a) || tenon.HasMark(bounded, iso) {
		t.Errorf("narrowing by a marked bound gave %v, with the wrong marks", bounded)
	}
	looser := tenon.Narrow(bounded, tenon.NumberMin(tenon.WithMarks(tenon.NumberFromInt(0), b), true))
	if !tenon.HasMark(looser, a) || !tenon.HasMark(looser, b) {
		t.Errorf("narrowing by a marked bound that changes nothing gave %v, without its marks", looser)
	}

	// An operation that reads the values within an operand consumes them too,
	// so their Propagate marks reach the result: equality reads the members of
	// what it compares, and membership reads the value it looks for. One that
	// reads only an operand's shape does not: a length counts members without
	// reading them, and a list is not null whatever it holds.
	held := tenon.ListVal(num, tenon.WithMarks(tenon.NumberFromInt(1), a, iso))
	plain := tenon.ListVal(num, tenon.NumberFromInt(1))
	for _, tt := range []struct {
		name string
		r    tenon.Value
		want bool
	}{
		{"Equals", tenon.Equals(held, plain), true},
		{"Equals the other way about", tenon.Equals(plain, held), true},
		{"Contains", tenon.Contains(tenon.SetVal(tenon.List(num), plain), held), true},
		{"Length", tenon.Length(held), false},
		{"IsNull", tenon.IsNull(held), false},
	} {
		if got := tenon.HasMark(tt.r, a); got != tt.want {
			t.Errorf("%s over a list holding a marked member: the result carries its mark: %t, want %t", tt.name, got, tt.want)
		}
		if tenon.HasMark(tt.r, iso) {
			t.Errorf("%s: an Isolate mark held within an operand reached the result", tt.name)
		}
	}
}

func TestConformance_MK004_EqualsIgnoresMarksIdenticalDoesNot(t *testing.T) {
	conformance.Covers(t, "MK-004")
	num := tenon.NumberType()
	m1, m2 := stamp{id: "a"}, stamp{id: "b"}
	one := tenon.NumberFromInt(1)
	mOne := tenon.WithMarks(one, m1)

	// Equals compares values, not what is attached to them: a marked value,
	// its unmarked twin, and a differently marked one are all one value, at
	// the top level and inside a container.
	for name, pair := range map[string][2]tenon.Value{
		"marked and unmarked":  {mOne, one},
		"differently marked":   {mOne, tenon.WithMarks(one, m2)},
		"marked list members":  {tenon.ListVal(num, mOne), tenon.ListVal(num, one)},
		"marked null and null": {tenon.WithMarks(tenon.NullVal(num), m1), tenon.NullVal(num)},
		"marked error operand": {tenon.WithMarks(tenon.Bool(true), m1), tenon.Bool(true)},
	} {
		if got := unmarked(tenon.Equals(pair[0], pair[1])).String(); got != "true" {
			t.Errorf("%s: Equals is %s, want true", name, got)
		}
	}

	// Identical holds everything the value system holds, marks included.
	if tenon.Identical(mOne, one) {
		t.Error("a marked value is identical to its unmarked twin")
	}
	if tenon.Identical(mOne, tenon.WithMarks(one, m2)) {
		t.Error("values carrying different marks are identical")
	}
	if !tenon.Identical(mOne, tenon.WithMarks(one, m1)) {
		t.Error("values carrying one mark are not identical")
	}
	// The marks are a set: attachment order is not part of identity.
	if !tenon.Identical(tenon.WithMarks(one, m1, m2), tenon.WithMarks(tenon.WithMarks(one, m2), m1)) {
		t.Error("attachment order is part of identity, but a set has no order")
	}
	// A mark deep inside a container is part of the container's identity.
	if tenon.Identical(tenon.ListVal(num, mOne), tenon.ListVal(num, one)) {
		t.Error("a list holding a marked member is identical to one holding it unmarked")
	}
	// Error values carry marks too, and Identical sees them.
	e := tenon.ErrorVal(tenon.Diagnostic{Code: "app.x", Message: "m"})
	if tenon.Identical(tenon.WithMarks(e, m1), e) {
		t.Error("a marked error value is identical to its unmarked twin")
	}
}

func TestConformance_MK005_MarksDoNotAffectResults(t *testing.T) {
	conformance.Covers(t, "MK-005")
	num, bl, str := tenon.NumberType(), tenon.BoolType(), tenon.StringType()
	m := stamp{id: "m"}
	one, two, zero := tenon.NumberFromInt(1), tenon.NumberFromInt(2), tenon.NumberFromInt(0)
	tr, fa := tenon.Bool(true), tenon.Bool(false)
	unNum, unBool := tenon.Unknown(num), tenon.Unknown(bl)
	nullNum, nullBool := tenon.NullVal(num), tenon.NullVal(bl)
	errV := tenon.ErrorVal(tenon.Diagnostic{Code: "app.x", Message: "m"})
	pendNum := tenon.Pending(tenon.Exactly(num))
	list := tenon.ListVal(str, tenon.String("a"), tenon.String("b"))
	set1 := tenon.SetVal(num, one)
	setPartial := tenon.SetVal(num, unNum)
	unSet := tenon.Narrow(tenon.Unknown(tenon.Set(num)), tenon.NotNull(), tenon.Members(one))
	nullSet := tenon.NullVal(tenon.Set(num))

	un := func(f func(tenon.Value) tenon.Value) func([]tenon.Value) tenon.Value {
		return func(vs []tenon.Value) tenon.Value { return f(vs[0]) }
	}
	bin := func(f func(a, b tenon.Value) tenon.Value) func([]tenon.Value) tenon.Value {
		return func(vs []tenon.Value) tenon.Value { return f(vs[0], vs[1]) }
	}

	// Every operation, over operands in every state it accepts: the result
	// with marked operands, unmarked, is the result without them. The operand
	// matrix asserts this of every registered operation; this table keeps the
	// cases readable, and covers Narrow and Resolve, which refine rather than
	// operate and are outside the matrix.
	for _, row := range []struct {
		name string
		call func([]tenon.Value) tenon.Value
		args [][]tenon.Value
	}{
		{"And", bin(tenon.And), [][]tenon.Value{{tr, fa}, {fa, unBool}, {tr, nullBool}, {errV, tr}}},
		{"Or", bin(tenon.Or), [][]tenon.Value{{tr, fa}, {fa, unBool}}},
		{"Not", un(tenon.Not), [][]tenon.Value{{tr}, {unBool}, {nullBool}}},
		{"IsNull", un(tenon.IsNull), [][]tenon.Value{{one}, {nullNum}, {unNum}, {pendNum}}},
		{"Equals", bin(tenon.Equals), [][]tenon.Value{{one, one}, {one, two}, {one, unNum}, {nullNum, one}, {errV, one}, {pendNum, one}}},
		{"LessThan", bin(tenon.LessThan), [][]tenon.Value{{one, two}, {one, unNum}, {nullNum, one}}},
		{"Add", bin(tenon.Add), [][]tenon.Value{{one, two}, {one, unNum}, {one, nullNum}, {errV, two}}},
		{"Sub", bin(tenon.Sub), [][]tenon.Value{{one, two}, {one, unNum}}},
		{"Mul", bin(tenon.Mul), [][]tenon.Value{{one, two}}},
		{"Div", bin(tenon.Div), [][]tenon.Value{{one, two}, {one, zero}}},
		{"Mod", bin(tenon.Mod), [][]tenon.Value{{one, two}}},
		{"Length", un(tenon.Length), [][]tenon.Value{{list}, {setPartial}, {unSet}, {tenon.NullVal(tenon.List(str))}}},
		{"Contains", bin(tenon.Contains), [][]tenon.Value{{set1, one}, {set1, two}, {unSet, one}, {nullSet, one}}},
	} {
		for _, args := range row.args {
			want := row.call(args)
			// Each operand marked alone, then every operand marked.
			for which := -1; which < len(args); which++ {
				marked := slices.Clone(args)
				for i := range marked {
					if which < 0 || which == i {
						marked[i] = tenon.WithMarks(marked[i], m)
					}
				}
				got, _ := tenon.Unmark(row.call(marked))
				if !tenon.Identical(got, want) {
					t.Errorf("%s over %v with operand %d marked: %v, want %v",
						row.name, args, which, got, want)
				}
			}
		}
	}

	// Narrowing and resolving are as transparent: the marks carry, the
	// value does not change.
	nWant := tenon.Narrow(unNum, tenon.NotNull(), tenon.NumberMin(one, true))
	nGot, _ := tenon.Unmark(tenon.Narrow(tenon.WithMarks(unNum, m), tenon.NotNull(), tenon.NumberMin(one, true)))
	if !tenon.Identical(nGot, nWant) {
		t.Errorf("narrowing a marked value produced %v, want %v", nGot, nWant)
	}
	rWant := tenon.Resolve(pendNum, num)
	rGot, _ := tenon.Unmark(tenon.Resolve(tenon.WithMarks(pendNum, m), num))
	if !tenon.Identical(rGot, rWant) {
		t.Errorf("resolving a marked value produced %v, want %v", rGot, rWant)
	}
	bWant := tenon.Narrow(unNum, tenon.NumberMin(one, true))
	bGot, _ := tenon.Unmark(tenon.Narrow(unNum, tenon.NumberMin(tenon.WithMarks(one, m), true)))
	if !tenon.Identical(bGot, bWant) {
		t.Errorf("narrowing by a marked bound produced %v, want %v", bGot, bWant)
	}

	// A redacting mark changes one thing besides the marks: the message of a
	// diagnostic that would otherwise show what the mark withholds. The codes
	// and the paths stay as they are.
	secret := stamp{id: "secret", redact: true}
	shown := tenon.Narrow(tenon.String("hunter2"), tenon.StringPrefix("ab-")).Diagnostics()
	withheld := tenon.Narrow(tenon.WithMarks(tenon.String("hunter2"), secret), tenon.StringPrefix("ab-")).Diagnostics()
	if len(shown) != 1 || len(withheld) != 1 || shown[0].Code != withheld[0].Code ||
		!shown[0].Path.Equal(withheld[0].Path) || shown[0].Message == withheld[0].Message {
		t.Errorf("redaction changed more than the message, or not the message: %v and %v", shown, withheld)
	}

	// A mark that does not redact changes no message at all. A failure that
	// renders what it failed on renders it as it reads unmarked, whether the
	// mark is on that value or on a value within it, Propagate, Isolate or
	// deep, and beside a redacting mark, whose placeholder names the
	// redacting marks alone.
	origin, apart, deep := stamp{id: "origin"}, stamp{id: "apart", policy: tenon.Isolate}, stamp{id: "deep", deep: true}
	both := func(v tenon.Value) tenon.Value { return tenon.WithMarks(v, origin, apart) }
	a, b, yes, huge := tenon.String("a"), tenon.String("b"), tenon.String("yes"), tenon.String("1e1000000")
	toNum := func(v tenon.Value) tenon.Value { return tenon.Convert(v, tenon.Exactly(num), tenon.Unsafe) }
	toNums := func(v tenon.Value) tenon.Value {
		return tenon.Convert(v, tenon.ListOf(tenon.Exactly(num)), tenon.Unsafe)
	}
	for _, tt := range []struct {
		name          string
		marked, plain tenon.Value
	}{
		{"a string that is not a number", toNum(both(a)), toNum(a)},
		{"a string that is not a bool",
			tenon.Convert(both(yes), tenon.Exactly(bl), tenon.Unsafe), tenon.Convert(yes, tenon.Exactly(bl), tenon.Unsafe)},
		{"a string outside the range of numbers", toNum(both(huge)), toNum(huge)},
		{"a list member", toNums(tenon.ListVal(str, b, both(a))), toNums(tenon.ListVal(str, b, a))},
		{"a list under a deep mark", toNums(tenon.WithMarks(tenon.ListVal(str, a), deep)), toNums(tenon.ListVal(str, a))},
		{"an attribute",
			tenon.Convert(tenon.ObjectVal(map[string]tenon.Value{"x": both(a)}),
				tenon.ObjectWith(map[string]tenon.Field{"x": tenon.Required(tenon.Exactly(num))}, true), tenon.Unsafe),
			tenon.Convert(tenon.ObjectVal(map[string]tenon.Value{"x": a}),
				tenon.ObjectWith(map[string]tenon.Field{"x": tenon.Required(tenon.Exactly(num))}, true), tenon.Unsafe)},
		{"a map element",
			tenon.Convert(tenon.MapVal(str, map[string]tenon.Value{"k": both(a)}), tenon.MapOf(tenon.Exactly(num)), tenon.Unsafe),
			tenon.Convert(tenon.MapVal(str, map[string]tenon.Value{"k": a}), tenon.MapOf(tenon.Exactly(num)), tenon.Unsafe)},
		{"a member beside a redacting mark",
			toNum(tenon.WithMarks(a, secret, origin, apart)), toNum(tenon.WithMarks(a, secret))},
		{"a member of a redacted list",
			toNums(tenon.WithMarks(tenon.ListVal(str, both(a)), secret)), toNums(tenon.WithMarks(tenon.ListVal(str, a), secret))},
		{"a known value narrowed", tenon.Narrow(both(one), tenon.NumberMin(two, true)), tenon.Narrow(one, tenon.NumberMin(two, true))},
		{"a value within a known value narrowed",
			tenon.Narrow(tenon.ListVal(str, both(a), b), tenon.LengthMax(1)), tenon.Narrow(tenon.ListVal(str, a, b), tenon.LengthMax(1))},
		{"an element of a map narrowed",
			tenon.Narrow(tenon.MapVal(str, map[string]tenon.Value{"k": both(a)}), tenon.LengthMax(0)),
			tenon.Narrow(tenon.MapVal(str, map[string]tenon.Value{"k": a}), tenon.LengthMax(0))},
		{"a value under a deep mark narrowed",
			tenon.Narrow(tenon.WithMarks(tenon.ListVal(str, a, b), deep), tenon.LengthMax(1)), tenon.Narrow(tenon.ListVal(str, a, b), tenon.LengthMax(1))},
		// A long rendering is cut short, so the marked value comes first.
		{"beside a redacted value within a value narrowed",
			tenon.Narrow(tenon.ListVal(str, both(b), tenon.WithMarks(a, secret)), tenon.LengthMax(1)),
			tenon.Narrow(tenon.ListVal(str, b, tenon.WithMarks(a, secret)), tenon.LengthMax(1))},
	} {
		want, _ := tenon.UnmarkDeep(tt.plain)
		if !want.IsError() {
			t.Errorf("%s: %v is not an error value", tt.name, want)
			continue
		}
		if got, _ := tenon.UnmarkDeep(tt.marked); !tenon.Identical(got, want) {
			t.Errorf("%s: %v, want %v", tt.name, got, want)
		}
	}

	// An operand is marked when a value it holds carries a mark, and that is
	// as transparent as a mark on the operand itself.
	within := func(v tenon.Value) tenon.Value { return tenon.WithMarks(v, m) }
	for _, tt := range []struct {
		name      string
		got, want tenon.Value
	}{
		{
			"Length",
			tenon.Length(tenon.ListVal(str, within(tenon.String("a")))),
			tenon.Length(tenon.ListVal(str, tenon.String("a"))),
		},
		{
			"Equals",
			tenon.Equals(tenon.ListVal(num, within(one)), tenon.ListVal(num, one)),
			tenon.Equals(tenon.ListVal(num, one), tenon.ListVal(num, one)),
		},
		{
			"Contains",
			tenon.Contains(tenon.SetVal(tenon.List(num), tenon.ListVal(num, one)), tenon.ListVal(num, within(one))),
			tenon.Contains(tenon.SetVal(tenon.List(num), tenon.ListVal(num, one)), tenon.ListVal(num, one)),
		},
		{
			"IsNull",
			tenon.IsNull(tenon.TupleVal(within(unNum))),
			tenon.IsNull(tenon.TupleVal(unNum)),
		},
	} {
		if got, _ := tenon.Unmark(tt.got); !tenon.Identical(got, tt.want) {
			t.Errorf("%s over an operand holding a marked value: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestConformance_MK006_MarkedValuesHaveNoHashAndNoPlaceInASet(t *testing.T) {
	conformance.Covers(t, "MK-006")
	num := tenon.NumberType()
	lists := tenon.List(num)
	m, iso := stamp{id: "m"}, stamp{id: "iso", policy: tenon.Isolate}
	one, two := tenon.NumberFromInt(1), tenon.NumberFromInt(2)
	marked := tenon.WithMarks(one, m)
	// A value is marked when it carries a mark or holds, at any depth, a value
	// that does. This list carries none, and its element holds one.
	holding := tenon.ListVal(lists, tenon.ListVal(num, two, marked))

	// Hashing a marked value, ordering one canonically, placing one into a set
	// and listing one as a set's member are mistakes in the calling program.
	// Each message says where the mark is and what to do instead.
	for _, tt := range []struct {
		want string
		f    func()
	}{
		{"Hash called on a value of type number that carries marks", func() { tenon.Hash(marked) }},
		{"that holds a marked value at .[0][1]", func() { tenon.Hash(holding) }},
		{"that holds a marked value at .[1], and", func() { tenon.Hash(tenon.ListVal(num, two, marked, marked)) }},
		{"hash the value UnmarkDeep returns", func() { tenon.Hash(holding) }},
		{"CanonicalCompare called on a value of type number that carries marks", func() { tenon.CanonicalCompare(marked, one) }},
		{"that holds a marked value at .[0][1]", func() { tenon.CanonicalCompare(one, holding) }},
		{"compare the values UnmarkDeep returns", func() { tenon.CanonicalCompare(one, holding) }},
		{"SetVal: element 1 is a value of type number that carries marks", func() { tenon.SetVal(num, two, marked) }},
		{
			"SetVal: element 0 is a value of type list(number) that holds a marked value at .[1]",
			func() { tenon.SetVal(lists, holding.Index(0)) },
		},
		{"an unknown value of type number that carries marks", func() { tenon.SetVal(num, tenon.WithMarks(tenon.Unknown(num), m)) }},
		{"the null value of type number that carries marks", func() { tenon.SetVal(num, tenon.WithMarks(tenon.NullVal(num), iso)) }},
		{"unmark it with UnmarkDeep and reapply the marks to the set", func() { tenon.SetVal(num, marked) }},
		{"Members called with a value of type number that carries marks as member 1", func() { tenon.Members(two, marked) }},
		{"unmark it with UnmarkDeep and reapply the marks to the set", func() { tenon.Members(marked) }},
	} {
		mustPanicUsage(t, tt.want, tt.f)
	}

	// An error member is never placed into a set, because the set is an error
	// value in its place, so its marks are no reason to refuse it. A marked
	// member beside it is refused all the same: an error does not mask a
	// mistake in the calling program.
	failed := tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"}), m)
	if got := tenon.SetVal(num, one, failed); !got.IsError() {
		t.Errorf("a set given a marked error member is %v, want an error value", got)
	}
	mustPanicUsage(t, "SetVal: element 0 is a value of type number that carries marks", func() {
		tenon.SetVal(num, marked, failed)
	})

	// Nothing else is refused. A list holds a marked member as it is, and
	// asking whether a set holds a marked value places nothing in the set: the
	// answer carries the mark, as the answer of any operation would.
	if l := tenon.ListVal(num, marked); !tenon.HasMark(l.Index(0), m) {
		t.Errorf("the list %v lost its member's mark", l)
	}
	if got := tenon.Contains(tenon.SetVal(num, one), marked); unmarked(got).String() != "true" || !tenon.HasMark(got, m) {
		t.Errorf("asking whether a set holds a marked member gave %v", got)
	}

	// UnmarkDeep takes every mark, the value's own and those of everything it
	// holds, each once, sorted by identifier, and changes nothing else.
	deep := tenon.WithMarks(tenon.ObjectVal(map[string]tenon.Value{
		"a": tenon.ListVal(num, marked, tenon.WithMarks(two, iso)),
		"b": tenon.MapVal(num, map[string]tenon.Value{"k": marked}),
	}), m)
	plain := tenon.ObjectVal(map[string]tenon.Value{
		"a": tenon.ListVal(num, one, two),
		"b": tenon.MapVal(num, map[string]tenon.Value{"k": one}),
	})
	u, ms := tenon.UnmarkDeep(deep)
	if len(ms) != 2 || ms[0] != iso || ms[1] != m {
		t.Errorf("UnmarkDeep took %v, want iso then m", ms)
	}
	if !tenon.Identical(u, plain) || tenon.Hash(u) != tenon.Hash(plain) {
		t.Errorf("UnmarkDeep left %v, want %v", u, plain)
	}
	if !tenon.HasMark(deep, m) || !tenon.HasMark(deep.Attribute("a").Index(1), iso) {
		t.Error("UnmarkDeep changed the value it was given")
	}
	if got, ms := tenon.UnmarkDeep(plain); got != plain || ms != nil {
		t.Errorf("UnmarkDeep of a value marked nowhere returned %v, %v, want the value and no marks", got, ms)
	}
	// Unmark takes only the value's own marks, so what it leaves can still be
	// marked.
	top, _ := tenon.Unmark(tenon.WithMarks(holding, iso))
	mustPanicUsage(t, "that holds a marked value at .[0][1]", func() { tenon.Hash(top) })

	// The pattern: unmark each member, build the set, and reapply the marks to
	// the set. The marks of two equal members all survive, so the set is the
	// same whichever of them was given first.
	build := func(given ...tenon.Value) tenon.Value {
		var marks []tenon.Mark
		members := make([]tenon.Value, len(given))
		for i, v := range given {
			var ms []tenon.Mark
			members[i], ms = tenon.UnmarkDeep(v)
			marks = append(marks, ms...)
		}
		return tenon.WithMarks(tenon.SetVal(lists, members...), marks...)
	}
	listOne, listTwo := tenon.ListVal(num, one), tenon.ListVal(num, two)
	set := build(tenon.ListVal(num, marked), tenon.WithMarks(listTwo, iso), listOne)
	want := tenon.WithMarks(tenon.SetVal(lists, listOne, listTwo), m, iso)
	if !tenon.Identical(set, want) {
		t.Errorf("the set built by the pattern is %v, want %v", set, want)
	}
	if again := build(listOne, tenon.WithMarks(listTwo, iso), tenon.ListVal(num, marked)); !tenon.Identical(set, again) {
		t.Errorf("the pattern built %v one way about and %v the other", set, again)
	}
	// A Members narrowing takes the same pattern, with the marks going on the
	// set that is narrowed.
	listed, listedMarks := tenon.UnmarkDeep(marked)
	r := tenon.Narrow(tenon.WithMarks(tenon.Unknown(tenon.Set(num)), listedMarks...), tenon.NotNull(), tenon.Members(listed))
	if got := tenon.Contains(r, one); unmarked(got).String() != "true" || !tenon.HasMark(got, m) {
		t.Errorf("membership of the listed value gave %v", got)
	}

	// Over every shape the generator holds: a marked value is refused wherever
	// marked values are, and what UnmarkDeep makes of it is accepted wherever
	// its state allows.
	refused := 0
	for _, v := range values.All() {
		u, ms := tenon.UnmarkDeep(v)
		if _, again := tenon.UnmarkDeep(u); again != nil {
			t.Errorf("UnmarkDeep of %v left %v, which is still marked with %v", v, u, again)
		}
		if ms == nil && u != v {
			t.Errorf("UnmarkDeep of %v, which is marked nowhere, returned %v", v, u)
		}
		if v.IsResolved() {
			if ms != nil {
				refused++
				mustPanicUsage(t, "reapply the marks to the set", func() { tenon.SetVal(v.Type(), v) })
			}
			tenon.SetVal(u.Type(), u)
		}
		if v.IsKnown() && ms != nil {
			mustPanicUsage(t, "UnmarkDeep", func() { tenon.CanonicalCompare(u, v) })
			if tenon.IsNull(u).String() == "false" {
				mustPanicUsage(t, "UnmarkDeep", func() { tenon.Hash(v) })
			}
		}
	}
	if refused < 10 {
		t.Errorf("only %d marked values were refused, which is too few to say much", refused)
	}
}

// readable returns a known value and every value a caller can read out of it,
// at any depth, through the accessors a caller would use.
func readable(v tenon.Value) []tenon.Value {
	out := []tenon.Value{v}
	if tenon.IsNull(v).AsBool() {
		return out
	}
	switch v.Type().Kind() {
	case tenon.KindList, tenon.KindSet, tenon.KindTuple:
		for _, e := range v.Elements() {
			out = append(out, readable(e)...)
		}
	case tenon.KindMap:
		for _, k := range v.MapKeys() {
			e, _ := v.MapElement(k)
			out = append(out, readable(e)...)
		}
	case tenon.KindObject:
		for _, name := range v.Type().AttributeNames() {
			out = append(out, readable(v.Attribute(name))...)
		}
	}
	return out
}

func TestConformance_MK008_DeepMarks(t *testing.T) {
	conformance.Covers(t, "MK-008")
	num, str := tenon.NumberType(), tenon.StringType()
	deep := stamp{id: "deep", deep: true}
	one, two := tenon.NumberFromInt(1), tenon.NumberFromInt(2)
	set := tenon.SetVal(str, tenon.String("a"), tenon.String("b"))
	// A value with every kind of container in it, a set among them, a null and
	// a member that is not known.
	tree := tenon.ObjectVal(map[string]tenon.Value{
		"list":  tenon.ListVal(num, one, tenon.Unknown(num)),
		"map":   tenon.MapVal(num, map[string]tenon.Value{"k": two}),
		"tuple": tenon.TupleVal(set, tenon.NullVal(str)),
	})

	// A deep mark attached to a value is attached to every value within it,
	// whatever state that value is in and however it is read.
	marked := tenon.WithMarks(tree, deep)
	list, tuple := marked.Attribute("list"), marked.Attribute("tuple")
	k, _ := marked.Attribute("map").MapElement("k")
	for _, r := range []struct {
		where string
		v     tenon.Value
	}{
		{"the value", marked},
		{".list", list},
		{".list[0]", list.Index(0)},
		{".list[1], which is not known", list.Index(1)},
		{".map", marked.Attribute("map")},
		{`.map["k"]`, k},
		{".tuple", tuple},
		{".tuple[0], a set", tuple.Index(0)},
		{".tuple[0], a member of the set", tuple.Index(0).Elements()[1]},
		{".tuple[1], null", tuple.Index(1)},
	} {
		if !tenon.HasMark(r.v, deep) {
			t.Errorf("%s does not carry the deep mark: %v", r.where, r.v)
		}
	}

	// A mark that is not deep marks the value it is attached to and nothing
	// within it, whether it says so or does not implement DeepMark at all. A
	// deep mark's policy governs how it moves through operations, not how far
	// down it is attached, so a deep Isolate mark marks everything within too.
	shallow, plain := stamp{id: "shallow"}, bare("plain")
	isolated := stamp{id: "isolated", policy: tenon.Isolate, deep: true}
	several := tenon.WithMarks(tree, shallow, plain, isolated)
	inner := several.Attribute("list").Index(0)
	if !tenon.HasMark(several, shallow) || !tenon.HasMark(several, plain) {
		t.Errorf("%v lost the marks attached to it", several)
	}
	if tenon.HasMark(inner, shallow) || tenon.HasMark(inner, plain) {
		t.Errorf("a mark that is not deep reached %v", inner)
	}
	if !tenon.HasMark(inner, isolated) {
		t.Errorf("a deep Isolate mark did not reach %v", inner)
	}

	// Deep marking is applied when the mark is attached, not when a value is
	// inspected: taking the mark off the value afterwards leaves it on the
	// values within, which carry it in their own right.
	stripped, _ := tenon.Unmark(marked)
	if tenon.HasMark(stripped, deep) || !tenon.HasMark(stripped.Attribute("list").Index(0), deep) {
		t.Errorf("unmarking the value changed what is within it: %v", stripped)
	}
	if tenon.Identical(stripped, tree) {
		t.Error("a value whose deep mark was taken off again is identical to one never marked")
	}

	// Except a set's members, which carry no marks. The deep mark is recorded
	// on the set, its members stay unmarked in storage, and each member
	// carries the mark from the moment it is retrieved.
	sealed := tenon.WithMarks(set, deep)
	if stored, _ := tenon.Unmark(sealed); !tenon.Identical(stored, set) {
		t.Errorf("the members of a deep-marked set were marked in storage: %v", stored)
	}
	for i, got := range sealed.Elements() {
		if want := tenon.WithMarks(set.Elements()[i], deep); !tenon.Identical(got, want) {
			t.Errorf("member %d was retrieved as %v, want %v", i, got, want)
		}
	}
	if stored, _ := tenon.Unmark(tuple.Index(0)); !tenon.Identical(stored, set) {
		t.Errorf("the set within the deep-marked value holds marked members: %v", stored)
	}
	// Retrieval applies the mark deeply, to what a member holds.
	lists := tenon.WithMarks(tenon.SetVal(tenon.List(num), tenon.ListVal(num, one)), deep)
	if got := lists.Elements()[0].Index(0); !tenon.HasMark(got, deep) {
		t.Errorf("the element of a retrieved member does not carry the deep mark: %v", got)
	}
	// A mark on a set that is not deep stays on the set.
	if got := tenon.WithMarks(set, shallow).Elements()[0]; tenon.HasMark(got, shallow) {
		t.Errorf("a member retrieved from a set carries the set's shallow mark: %v", got)
	}

	// A deep mark is added to the marks a value within carries, and replaces
	// none of them.
	owned := tenon.WithMarks(tenon.ListVal(num, tenon.WithMarks(one, shallow), tenon.WithMarks(two, plain)), deep)
	for i, want := range []tenon.Mark{shallow, plain} {
		if e := owned.Index(i); !tenon.HasMark(e, want) || !tenon.HasMark(e, deep) {
			t.Errorf("element %d of the deep-marked list is %v, without its own mark or the deep one", i, e)
		}
	}

	// Marking a value whose members carry the mark already gives what marking
	// it unmarked gives: one value, one representation. Attaching the mark
	// again gives the value itself.
	got := tenon.WithMarks(tenon.ListVal(num, tenon.WithMarks(one, deep)), deep)
	if want := tenon.WithMarks(tenon.ListVal(num, one), deep); !tenon.Identical(got, want) {
		t.Errorf("marking a list whose element carries the mark gave %v, want %v", got, want)
	}
	if tenon.WithMarks(marked, deep) != marked {
		t.Error("attaching a deep mark again did not return the value itself")
	}

	// A mark carried onto a value that narrowing comes down to stays on it:
	// on the empty tuple, and on the set a listing describes, where Elements
	// applies it to the members.
	tup := tenon.Narrow(tenon.WithMarks(tenon.Unknown(tenon.Tuple()), deep), tenon.NotNull())
	if !tup.IsKnown() || !tenon.HasMark(tup, deep) {
		t.Errorf("narrowing came down to %v, which does not carry the deep mark", tup)
	}
	listed := tenon.Narrow(tenon.WithMarks(tenon.Unknown(tenon.Set(num)), deep),
		tenon.NotNull(), tenon.Members(one), tenon.LengthMax(1))
	if stored, _ := tenon.Unmark(listed); !tenon.Identical(stored, tenon.SetVal(num, one)) || !tenon.HasMark(listed.Elements()[0], deep) {
		t.Errorf("narrowing came down to the set %v, marked in the wrong place", listed)
	}

	// A deep mark survives the trip into a set and back out: unmark the member,
	// reapply its marks to the set, and retrieval puts the mark back where it
	// was.
	member := tenon.WithMarks(tenon.ListVal(num, one, two), deep)
	u, ms := tenon.UnmarkDeep(member)
	outer := tenon.WithMarks(tenon.SetVal(tenon.List(num), u), ms...)
	if back := outer.Elements()[0]; !tenon.Identical(back, member) {
		t.Errorf("the member came back out of the set as %v, want %v", back, member)
	}

	// Over every value the generator holds: a deep mark changes nothing but
	// marks, attaches once, and reaches everything a caller can read out of a
	// known value.
	for _, v := range values.All() {
		dv := tenon.WithMarks(v, deep)
		if !tenon.HasMark(dv, deep) || tenon.WithMarks(dv, deep) != dv {
			t.Errorf("%v took the deep mark as %v, and not once", v, dv)
		}
		got, _ := tenon.UnmarkDeep(dv)
		if want, _ := tenon.UnmarkDeep(v); !tenon.Identical(got, want) {
			t.Errorf("deep marking %v changed more than its marks: %v", v, got)
		}
		if !v.IsKnown() {
			continue
		}
		for _, r := range readable(dv) {
			if !tenon.HasMark(r, deep) {
				t.Errorf("%v, read out of %v, does not carry the deep mark", r, dv)
			}
		}
	}
}

func TestConformance_MK010_ErrorValuesCarryMarks(t *testing.T) {
	conformance.Covers(t, "MK-010")
	num, str, bl := tenon.NumberType(), tenon.StringType(), tenon.BoolType()
	p, q := stamp{id: "p"}, stamp{id: "q"}
	iso := stamp{id: "iso", policy: tenon.Isolate}
	one, five := tenon.NumberFromInt(1), tenon.NumberFromInt(5)
	failed := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "it failed"})
	other := tenon.ErrorVal(tenon.Diagnostic{Code: "app.other", Message: "and again"})
	for _, tt := range []struct {
		name  string
		got   tenon.Value
		diags string   // each diagnostic's code and path
		marks []string // the identifiers of the marks carried, in order
	}{
		// A failed operation carries the Propagate marks of every operand, and
		// an Isolate mark stays behind.
		{
			"a division by zero",
			tenon.Div(tenon.WithMarks(one, p, iso), tenon.WithMarks(tenon.NumberFromInt(0), q)),
			"number.divide_by_zero", []string{"p", "q"},
		},
		{
			"a null operand",
			tenon.And(tenon.WithMarks(tenon.NullVal(bl), p), tenon.Bool(true)),
			"operation.null_operand", []string{"p"},
		},
		{
			"a pending operand whose type cannot fit",
			tenon.Add(tenon.WithMarks(tenon.Pending(tenon.Exactly(str)), p), one),
			"operation.wrong_type", []string{"p"},
		},
		// The Propagate marks of an error operand survive the hoisting of its
		// diagnostics into the error value of the operation.
		{
			"an error operand",
			tenon.Add(tenon.WithMarks(failed, p, iso), tenon.WithMarks(one, q)),
			"app.failed", []string{"p", "q"},
		},
		{
			"two error operands",
			tenon.Add(tenon.WithMarks(failed, p), tenon.WithMarks(other, q)),
			"app.failed; app.other", []string{"p", "q"},
		},
		// And the hoisting of its diagnostics into the error value of a
		// container, at any depth. Building a container is not an operation
		// over its elements: an element that is not an error keeps its marks
		// to itself.
		{
			"an error element",
			tenon.ListVal(num, tenon.WithMarks(one, q), tenon.WithMarks(failed, p, iso)),
			"app.failed at .[1]", []string{"p"},
		},
		{
			"an error hoisted twice",
			tenon.TupleVal(tenon.ObjectVal(map[string]tenon.Value{"a": tenon.WithMarks(failed, p)})),
			"app.failed at .[0].a", []string{"p"},
		},
		{
			"an error entry of a map",
			tenon.MapVal(num, map[string]tenon.Value{"k": tenon.WithMarks(failed, p)}),
			`app.failed at .["k"]`, []string{"p"},
		},
		{
			"an error member of a set",
			tenon.SetVal(num, tenon.WithMarks(failed, p)),
			"app.failed at .[0]", []string{"p"},
		},
		// Narrowing and resolving refine the value they are given, so an error
		// keeps every mark it carries, and a contradiction carries every mark
		// of the value it contradicts beside the Propagate marks of a bound.
		{
			"narrowing an error",
			tenon.Narrow(tenon.WithMarks(failed, p, iso), tenon.NotNull()),
			"app.failed", []string{"iso", "p"},
		},
		{
			"resolving an error",
			tenon.Resolve(tenon.WithMarks(failed, iso), num),
			"app.failed", []string{"iso"},
		},
		{
			"a contradiction",
			tenon.Narrow(tenon.WithMarks(one, iso), tenon.NumberMin(tenon.WithMarks(five, q), true)),
			"range.contradiction", []string{"iso", "q"},
		},
	} {
		if !tt.got.IsError() {
			t.Errorf("%s: %v is not an error value", tt.name, tt.got)
			continue
		}
		var diags []string
		for _, d := range tt.got.Diagnostics() {
			text := string(d.Code)
			if d.Path.Len() > 0 {
				text += " at " + d.Path.String()
			}
			diags = append(diags, text)
		}
		if got := strings.Join(diags, "; "); got != tt.diags {
			t.Errorf("%s: the diagnostics are %s, want %s", tt.name, got, tt.diags)
		}
		var ids []string
		_, ms := tenon.Unmark(tt.got)
		for _, m := range ms {
			ids = append(ids, m.MarkID())
		}
		if !slices.Equal(ids, tt.marks) {
			t.Errorf("%s: the error value carries %v, want %v", tt.name, ids, tt.marks)
		}
	}
}

// manyStamps returns m Propagate marks, their identifiers in the order the
// marks are listed, deep ones where deep says.
func manyStamps(m int, deep bool) []tenon.Mark {
	marks := make([]tenon.Mark, m)
	for i := range marks {
		marks[i] = stamp{id: fmt.Sprintf("m%05d", i), deep: deep}
	}
	return marks
}

// TestConformance_MK003_ManyMarksAreGatheredEachOnce holds what gathers the
// Propagate marks of what it consumes to taking each once and no Isolate mark,
// on either side of the count past which marks are looked up through a set: an
// operation over its operands, a container built from marked error members,
// and a narrowing over its bounds. Diff sets a container's deep marks aside
// from each value within it the same way.
func TestConformance_MK003_ManyMarksAreGatheredEachOnce(t *testing.T) {
	conformance.Covers(t, "MK-003", "ER-008", "DI-034")
	num := tenon.NumberType()
	kept := stamp{id: "kept", policy: tenon.Isolate}
	carries := func(what string, v tenon.Value, want []tenon.Mark) {
		t.Helper()
		if _, got := tenon.Unmark(v); !slices.Equal(got, want) {
			t.Errorf("%s carries %d marks, want the %d gathered: %v", what, len(got), len(want), got)
		}
	}
	for _, m := range []int{4, 16, 17, 100} {
		marks := manyStamps(m, false)
		// The operands share half of their marks.
		a := tenon.WithMarks(n(1), append(marks[:m*3/4:m*3/4], kept)...)
		b := tenon.WithMarks(n(2), marks[m/4:]...)
		carries(fmt.Sprintf("the sum of operands carrying %d marks", m), tenon.Add(a, b), marks)
		// Each error member carries its own mark and its neighbour's.
		members := make([]tenon.Value, m)
		for i := range members {
			failed := tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: fmt.Sprintf("member %d", i)})
			members[i] = tenon.WithMarks(failed, marks[i], marks[(i+1)%m], kept)
		}
		carries(fmt.Sprintf("a list of %d marked error members", m), tenon.ListVal(num, members...), marks)
		// The bounds share half of their marks.
		lo, hi := tenon.WithMarks(n(1), marks[:m*3/4]...), tenon.WithMarks(n(9), marks[m/4:]...)
		carries(fmt.Sprintf("a narrowing by bounds carrying %d marks", m),
			tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(lo, true), tenon.NumberMax(hi, true)), marks)
		// A list's deep marks are set aside from its members, so a member
		// that gains a mark of its own has changed by that mark alone.
		deep, extra := manyStamps(m, true), stamp{id: "extra"}
		before := tenon.WithMarks(tenon.ListVal(num, n(1)), deep...)
		after := tenon.WithMarks(tenon.ListVal(num, tenon.WithMarks(n(1), extra)), deep...)
		changes := tenon.Diff(before, after)
		if len(changes) != 1 || changes[0].Kind != tenon.ChangeMarks || len(changes[0].OldMarks) != 0 ||
			!slices.Equal(changes[0].NewMarks, []tenon.Mark{extra}) {
			t.Errorf("a member gaining a mark under %d deep marks: %v", m, changes)
		}
	}
}

// TestConformance_MK003_GatheringManyMarksGrowsWithThem holds gathering the
// Propagate marks of what is consumed to work in proportion to the marks, as
// the growth job reads the benchmark pairs: what an operation, a container of
// error members, a narrowing and a diff allocate at 4,000 marks is under five
// times what they allocate at 1,000. Scanning the gathered marks for each
// costs the square of them in time, which no count shows; a list grown a mark
// at a time costs more than five times the bytes here, which this does.
func TestConformance_MK003_GatheringManyMarksGrowsWithThem(t *testing.T) {
	conformance.Covers(t, "MK-003", "DI-034")
	num := tenon.NumberType()
	shapes := func(m int) map[string]func() {
		marks, deep := manyStamps(m, false), manyStamps(m, true)
		members := make([]tenon.Value, m)
		for i := range members {
			members[i] = tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: fmt.Sprintf("member %d", i)}), marks[i])
		}
		lo, hi := tenon.WithMarks(n(1), marks[:m/2]...), tenon.WithMarks(n(9), marks[m/2:]...)
		before := tenon.WithMarks(tenon.ListVal(num, n(1)), deep...)
		after := tenon.WithMarks(tenon.ListVal(num, tenon.WithMarks(n(1), stamp{id: "extra"})), deep...)
		return map[string]func(){
			"an operation": func() { tenon.Add(lo, hi) },
			"a container":  func() { tenon.ListVal(num, members...) },
			"a narrowing":  func() { tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(lo, true), tenon.NumberMax(hi, true)) },
			"a diff":       func() { tenon.Diff(before, after) },
		}
	}
	allocated := func(f func()) uint64 {
		const rounds = 5
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for range rounds {
			f()
		}
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	small, large := shapes(1000), shapes(4000)
	for _, name := range []string{"an operation", "a container", "a narrowing", "a diff"} {
		a, b := allocated(small[name]), allocated(large[name])
		if grew := float64(b) / float64(a); grew > 5 {
			t.Errorf("%s over four times the marks allocated %.2f times the bytes (%d, then %d)", name, grew, a, b)
		}
	}
}

// BenchmarkMarkUnions measures gathering the Propagate marks of what is
// consumed, at a count of marks and four times it: an operation over two
// operands carrying them between them, a list of error members each carrying
// one, a narrowing by two bounds carrying them between them, and a diff that
// sets a list's deep marks aside from its member. The growth from one count to
// the other is the reading, not the wall clock.
func BenchmarkMarkUnions(b *testing.B) {
	num := tenon.NumberType()
	for _, m := range []int{1000, 4000} {
		marks, deep := manyStamps(m, false), manyStamps(m, true)
		members := make([]tenon.Value, m)
		for i := range members {
			members[i] = tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: fmt.Sprintf("member %d", i)}), marks[i])
		}
		lo, hi := tenon.WithMarks(n(1), marks[:m/2]...), tenon.WithMarks(n(9), marks[m/2:]...)
		before := tenon.WithMarks(tenon.ListVal(num, n(1)), deep...)
		after := tenon.WithMarks(tenon.ListVal(num, tenon.WithMarks(n(1), stamp{id: "extra"})), deep...)
		for _, shape := range []struct {
			name string
			run  func()
		}{
			{"operation", func() { tenon.Add(lo, hi) }},
			{"container", func() { tenon.ListVal(num, members...) }},
			{"narrowing", func() { tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(lo, true), tenon.NumberMax(hi, true)) }},
			{"diff", func() { tenon.Diff(before, after) }},
		} {
			b.Run(fmt.Sprintf("%s/%d", shape.name, m), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					shape.run()
				}
			})
		}
	}
}
