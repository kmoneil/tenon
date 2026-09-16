package tenon_test

import (
	"slices"
	"testing"

	"tenon"
	"tenon/conformance"
	"tenon/conformance/values"
)

// stamp is a Mark for tests: comparable, with a policy and a redaction flag.
type stamp struct {
	id     string
	policy tenon.Propagation
	redact bool
}

func (m stamp) MarkID() string                 { return m.id }
func (m stamp) Propagation() tenon.Propagation { return m.policy }
func (m stamp) Redacting() bool                { return m.redact }

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
	if f.String() != "false" || !tenon.HasMark(f, a) || !tenon.HasMark(f, b) {
		t.Errorf("And decided by false is %v with the wrong marks", f)
	}

	// An unknown result carries the union too.
	un := tenon.Equals(tenon.WithMarks(tenon.Unknown(num), a), tenon.NumberFromInt(3))
	if un.IsKnown() || !tenon.HasMark(un, a) {
		t.Errorf("the unknown result %v does not carry the operand's mark", un)
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
		if got := tenon.Equals(pair[0], pair[1]).String(); got != "true" {
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
	// with marked operands, unmarked, is the result without them. The
	// operand matrix will assert this over the operation registry; until it
	// exists this table is the registry, and a new operation belongs here.
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
		{"that holds a marked value at [0][1]", func() { tenon.Hash(holding) }},
		{"that holds a marked value at [1], and", func() { tenon.Hash(tenon.ListVal(num, two, marked, marked)) }},
		{"hash the value UnmarkDeep returns", func() { tenon.Hash(holding) }},
		{"CanonicalCompare called on a value of type number that carries marks", func() { tenon.CanonicalCompare(marked, one) }},
		{"that holds a marked value at [0][1]", func() { tenon.CanonicalCompare(one, holding) }},
		{"compare the values UnmarkDeep returns", func() { tenon.CanonicalCompare(one, holding) }},
		{"SetVal: element 1 is a value of type number that carries marks", func() { tenon.SetVal(num, two, marked) }},
		{
			"SetVal: element 0 is a value of type list(number) that holds a marked value at [1]",
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
	if got := tenon.Contains(tenon.SetVal(num, one), marked); got.String() != "true" || !tenon.HasMark(got, m) {
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
	mustPanicUsage(t, "that holds a marked value at [0][1]", func() { tenon.Hash(top) })

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
	if got := tenon.Contains(r, one); got.String() != "true" || !tenon.HasMark(got, m) {
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
