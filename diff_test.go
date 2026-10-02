package tenon_test

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// wantDiff fails t unless the diff of a against b displays as the lines given.
func wantDiff(t *testing.T, what string, a, b tenon.Value, lines ...string) {
	t.Helper()
	want := ""
	for _, l := range lines {
		want += l + "\n"
	}
	if got := tenon.Diff(a, b).String(); got != want {
		t.Errorf("%s: the diff of\n  %v\nagainst\n  %v\nis\n%swant\n%s", what, a, b, got, want)
	}
}

// partAt returns the part of v that p locates, and whether there is one, a
// pending tuple's or object's members among them.
func partAt(v tenon.Value, p tenon.Path) (tenon.Value, bool) {
	for _, s := range p.Steps() {
		if !v.HasMembers() {
			return tenon.Value{}, false
		}
		k := tenon.KindTuple
		switch {
		case !v.IsPending():
			k = v.Type().Kind()
		case v.Constraint().Kind() == tenon.ConstraintObjectWith:
			k = tenon.KindObject
		}
		switch {
		case s.Kind() == tenon.StepAttribute && k == tenon.KindObject:
			a, ok := v.LookupAttribute(s.Name())
			if !ok {
				return tenon.Value{}, false
			}
			v = a
		case s.Kind() == tenon.StepIndex && (k == tenon.KindList || k == tenon.KindTuple):
			i, ok := s.Key().AsInt64()
			if !ok || i < 0 || int(i) >= v.Len() {
				return tenon.Value{}, false
			}
			v = v.Index(int(i))
		case s.Kind() == tenon.StepIndex && k == tenon.KindMap && s.Key().Type() == tenon.StringType():
			e, ok := v.LookupMapElement(s.Key().AsString())
			if !ok {
				return tenon.Value{}, false
			}
			v = e
		default:
			return tenon.Value{}, false
		}
	}
	return v, true
}

// mirrored returns c turned the other way, as Diff(b, a) holds it.
func mirrored(c tenon.Change) tenon.Change {
	m := tenon.Change{Kind: c.Kind, Path: c.Path, Old: c.New, New: c.Old, OldMarks: c.NewMarks, NewMarks: c.OldMarks, InCollection: c.InCollection}
	switch c.Kind {
	case tenon.ChangeAdded:
		m.Kind = tenon.ChangeRemoved
	case tenon.ChangeRemoved:
		m.Kind = tenon.ChangeAdded
	case tenon.ChangeMemberAdded:
		m.Kind = tenon.ChangeMemberRemoved
	case tenon.ChangeMemberRemoved:
		m.Kind = tenon.ChangeMemberAdded
	}
	return m
}

// TestConformance_DI035_MembersThatReadAlikeMirror holds the mirror promise
// where a member removal and a member addition read alike. Two sets each
// hold a tuple of a capsule value and an unknown; the display forms tie, so
// the display interleave says nothing, and the two are ordered as a set
// holding the members of both sets orders members that encode alike
// (EQ-044): by their encodings, and past a stand-in by the capsule values.
// The key follows from the member and not from the side it came from, so
// Diff(b, a) mirrors Diff(a, b), where each direction once put its own
// removal first.
func TestConformance_DI035_MembersThatReadAlikeMirror(t *testing.T) {
	conformance.Covers(t, "DI-035", "EQ-044")
	num := tenon.NumberType()
	mirrorOf := func(t *testing.T, what string, a, b tenon.Value) tenon.Changes {
		t.Helper()
		forward, back := tenon.Diff(a, b), tenon.Diff(b, a)
		turned := make(tenon.Changes, len(forward))
		for i, c := range forward {
			turned[i] = mirrored(c)
		}
		if turned.String() != back.String() {
			t.Errorf("%s: the diff is\n%sand the other way\n%s", what, forward, back)
		}
		return forward
	}

	// A capsule type with no operations: nothing orders its values but the
	// run's own bookkeeping, which EQ-045 leaves to the implementation, and
	// the mirror holds through it.
	blank := tenon.NewCapsule("blank", tenon.CapsuleOps[celsius]{})
	blankTuple := tenon.TupleType(blank.Type(), num)
	m1 := tenon.Tuple(blank.Value(&celsius{1}), tenon.Unknown(num))
	m2 := tenon.Tuple(blank.Value(&celsius{2}), tenon.Unknown(num))
	if m1.String() != m2.String() {
		t.Fatalf("the members read %s and %s, not alike", m1, m2)
	}
	mirrorOf(t, "no operations", tenon.Set(blankTuple, m1), tenon.Set(blankTuple, m2))

	// A capsule type that encodes but does not display: the members read
	// alike and their encodings differ, so the one with the lesser encoding
	// leads from either side, in any run.
	coded := tenon.NewCapsule("coded", tenon.CapsuleOps[celsius]{
		Equal: func(a, b *celsius) bool { return *a == *b },
		Hash:  func(v *celsius) uint64 { return uint64(v.degrees) },
		Encoding: &tenon.CapsuleEncoding[celsius]{
			ID:     "t/coded",
			Type:   num,
			Encode: func(v *celsius) tenon.Value { return tenon.NumberFromInt(v.degrees) },
			Decode: func(v tenon.Value) (*celsius, error) { i, _ := v.AsInt64(); return &celsius{i}, nil },
		},
	})
	codedTuple := tenon.TupleType(coded.Type(), num)
	lesser := tenon.Tuple(coded.Value(&celsius{1}), tenon.Unknown(num))
	greater := tenon.Tuple(coded.Value(&celsius{2}), tenon.Unknown(num))
	if lesser.String() != greater.String() {
		t.Fatalf("the members read %s and %s, not alike", lesser, greater)
	}
	forward := mirrorOf(t, "encodings differ", tenon.Set(codedTuple, lesser), tenon.Set(codedTuple, greater))
	if len(forward) != 2 || forward[0].Kind != tenon.ChangeMemberRemoved || !tenon.Identical(forward[0].Old, lesser) {
		t.Errorf("the member with the lesser encoding does not lead: %s", forward)
	}
}

func TestConformance_DI030_ChangesAndWhatTheyCarry(t *testing.T) {
	conformance.Covers(t, "DI-030")
	num, str := tenon.NumberType(), tenon.StringType()
	n, s := tenon.NumberFromInt, tenon.String
	m := stamp{id: "m"}
	a := tenon.Object(map[string]tenon.Value{
		"gone": s("x"), "list": tenon.List(num, n(1)), "set": tenon.Set(str, s("a")), "same": n(1),
		"swap": n(1), "marked": tenon.List(num),
	})
	b := tenon.Object(map[string]tenon.Value{
		"list": tenon.List(num, n(1), n(2)), "new": s("y"), "set": tenon.Set(str, s("b")), "same": n(1),
		"swap": s("1"), "marked": tenon.WithMarks(tenon.List(num), m),
	})
	got := tenon.Diff(a, b)
	root := tenon.Path{}
	// A change within a list, set or map says so: the element type fixes its
	// parts' type in both values.
	want := tenon.Changes{
		{Kind: tenon.ChangeRemoved, Path: root.Attribute("gone"), Old: s("x")},
		{Kind: tenon.ChangeAdded, Path: root.Attribute("list").Index(n(1)), New: n(2), InCollection: true},
		{Kind: tenon.ChangeMarks, Path: root.Attribute("marked"), NewMarks: []tenon.Mark{m}},
		{Kind: tenon.ChangeAdded, Path: root.Attribute("new"), New: s("y")},
		{Kind: tenon.ChangeMemberRemoved, Path: root.Attribute("set"), Old: s("a"), InCollection: true},
		{Kind: tenon.ChangeMemberAdded, Path: root.Attribute("set"), New: s("b"), InCollection: true},
		{Kind: tenon.ChangeReplaced, Path: root.Attribute("swap"), Old: n(1), New: s("1")},
	}
	if len(got) != len(want) {
		t.Fatalf("the diff has %d changes, want %d:\n%s", len(got), len(want), got)
	}
	for i, c := range got {
		w := want[i]
		if c.Kind != w.Kind || !c.Path.Equal(w.Path) || !c.Old.Equal(w.Old) || !c.New.Equal(w.New) ||
			!slices.Equal(c.OldMarks, w.OldMarks) || !slices.Equal(c.NewMarks, w.NewMarks) || c.InCollection != w.InCollection {
			t.Errorf("change %d is %s (%s, in a collection %t), want %s (%s, %t)", i, c, c.Kind, c.InCollection, w, w.Kind, w.InCollection)
		}
	}

	// A member change carries its member as its set gives it when read,
	// with the set's deep marks on it, as Elements gives it, each side's
	// its own: the removed members carry the old set's deep mark, the added
	// ones the new set's. The members are paired and ordered as the sets
	// hold them, so the marks move nothing but what the changes carry.
	d, e := stamp{id: "d", deep: true}, stamp{id: "e", deep: true}
	u := tenon.Unknown(num)
	old := tenon.WithMarks(tenon.Set(num, n(1), n(2), u), d)
	fresh := tenon.WithMarks(tenon.Set(num, n(2), n(3), tenon.Narrow(u, tenon.NotNull())), e)
	given := func(set tenon.Value, m tenon.Value) tenon.Value {
		for _, g := range set.Elements() {
			if stored, _ := tenon.UnmarkDeep(g); tenon.Identical(stored, m) {
				return g
			}
		}
		t.Fatalf("%v gives no member %v", set, m)
		return tenon.Value{}
	}
	wantDiff(t, "member changes carrying the sets' deep marks", old, fresh,
		`~ .: marks ["d"] -> ["e"]`,
		`- .: member marked(1, "d")`, `+ .: member marked(3, "e")`,
		`- .: member marked(unknown, "d")`, `+ .: member marked(unknown(not null), "e")`)
	for _, c := range tenon.Diff(old, fresh) {
		switch c.Kind {
		case tenon.ChangeMemberRemoved:
			if stored, _ := tenon.UnmarkDeep(c.Old); !tenon.Identical(c.Old, given(old, stored)) {
				t.Errorf("the removal carries %v, where Elements gives %v", c.Old, given(old, stored))
			}
		case tenon.ChangeMemberAdded:
			if stored, _ := tenon.UnmarkDeep(c.New); !tenon.Identical(c.New, given(fresh, stored)) {
				t.Errorf("the addition carries %v, where Elements gives %v", c.New, given(fresh, stored))
			}
		}
	}
}

func TestConformance_DI031_EmptyExactlyWhenIdentical(t *testing.T) {
	conformance.Covers(t, "DI-031")
	all := values.All()
	for _, a := range all {
		for _, b := range all {
			if empty, identical := len(tenon.Diff(a, b)) == 0, tenon.Identical(a, b); empty != identical {
				t.Errorf("%v against %v: empty diff %t, identical %t:\n%s", a, b, empty, identical, tenon.Diff(a, b))
			}
		}
	}
	// A deep mark set aside within is still a difference, counted on the part
	// that carries it.
	d := stamp{id: "d", deep: true}
	l := tenon.List(tenon.NumberType(), tenon.NumberFromInt(1))
	if len(tenon.Diff(l, tenon.WithMarks(l, d))) == 0 {
		t.Error("a list and its deeply marked twin have an empty diff")
	}
}

func TestConformance_DI032_WhereTheDiffLooksWithin(t *testing.T) {
	conformance.Covers(t, "DI-032")
	num, str := tenon.NumberType(), tenon.StringType()
	n, s := tenon.NumberFromInt, tenon.String
	one := tenon.List(num, n(1))
	// Parts that do not match are replaced whole.
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		{"kinds that differ", tenon.Tuple(n(1)), one, `~ .: [1] -> list(number)[1]`},
		{"a map and an object", tenon.Map(num, map[string]tenon.Value{"a": n(1)}), tenon.Object(map[string]tenon.Value{"a": n(1)}),
			`~ .: map(number){"a": 1} -> {"a": 1}`},
		{"element types that differ", tenon.List(num), tenon.List(str), `~ .: list(number)[] -> list(string)[]`},
		{"a null", tenon.Null(tenon.ListType(num)), one, `~ .: null(list(number)) -> list(number)[1]`},
		{"an unknown", tenon.Unknown(tenon.ListType(num)), one, `~ .: unknown(list(number)) -> list(number)[1]`},
		{"a pending value", tenon.Pending(tenon.Any()), one, `~ .: pending(any) -> list(number)[1]`},
		{"an error value", tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x"}), one, `~ .: error(app.failed: "x") -> list(number)[1]`},
		{"scalars", s("a"), s("b"), `~ .: "a" -> "b"`},
		{"a redacted container", tenon.WithMarks(one, stamp{id: "s", redact: true}), tenon.WithMarks(tenon.List(num, n(2)), stamp{id: "s", redact: true}),
			`~ .: redacted("s") -> redacted("s")`},
	} {
		wantDiff(t, tt.name, tt.a, tt.b, tt.want)
	}

	// Parts that differ in their marks alone have a mark change, unless one
	// carries a redacting mark.
	m := stamp{id: "m"}
	wantDiff(t, "a marked scalar", n(1), tenon.WithMarks(n(1), m), `~ .: marks [] -> ["m"]`)
	wantDiff(t, "a marked unknown", tenon.WithMarks(tenon.Unknown(str), m), tenon.Unknown(str), `~ .: marks ["m"] -> []`)
	wantDiff(t, "a marked error", tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x"}),
		tenon.WithMarks(tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x"}), m), `~ .: marks [] -> ["m"]`)
	wantDiff(t, "a value gaining a redacting mark", n(1), tenon.WithMarks(n(1), stamp{id: "s", redact: true}), `~ .: 1 -> redacted("s")`)

	// Parts that match are entered, their mark change first.
	wantDiff(t, "a marked list with a new element", one, tenon.WithMarks(tenon.List(num, n(1), n(2)), m),
		`~ .: marks [] -> ["m"]`, `+ .[1]: 2`)
	wantDiff(t, "tuples of other types", tenon.Tuple(n(1), s("a")), tenon.Tuple(n(1), n(2)), `~ .[1]: "a" -> 2`)
	wantDiff(t, "objects of other attributes", tenon.Object(map[string]tenon.Value{"a": n(1)}), tenon.Object(map[string]tenon.Value{"b": n(1)}),
		`- .a: 1`, `+ .b: 1`)
	wantDiff(t, "a list holding an unknown", tenon.List(num, tenon.Unknown(num)), tenon.List(num, n(3)), `~ .[0]: unknown -> 3`)
}

func TestConformance_DI033_MembersCompare(t *testing.T) {
	conformance.Covers(t, "DI-033")
	num, str := tenon.NumberType(), tenon.StringType()
	n, s := tenon.NumberFromInt, tenon.String
	// Lists compare by index, with no alignment.
	wantDiff(t, "an element inserted first", tenon.List(num, n(1), n(2)), tenon.List(num, n(0), n(1), n(2)),
		`~ .[0]: 1 -> 0`, `~ .[1]: 2 -> 1`, `+ .[2]: 2`)
	wantDiff(t, "elements removed", tenon.Tuple(n(1), s("a"), n(3)), tenon.Tuple(n(1)), `- .[1]: "a"`, `- .[2]: 3`)
	// Maps and objects by name, nested paths extended step by step.
	wantDiff(t, "map entries", tenon.Map(num, map[string]tenon.Value{"a b": n(1), "c": n(2)}), tenon.Map(num, map[string]tenon.Value{"c": n(3), "d": n(4)}),
		`- .["a b"]: 1`, `~ .["c"]: 2 -> 3`, `+ .["d"]: 4`)
	wantDiff(t, "nested attributes",
		tenon.Object(map[string]tenon.Value{"x y": tenon.Object(map[string]tenon.Value{"z": tenon.List(str, s("a"))})}),
		tenon.Object(map[string]tenon.Value{"x y": tenon.Object(map[string]tenon.Value{"z": tenon.List(str, s("b"))})}),
		`~ ."x y".z[0]: "a" -> "b"`)
	// Sets by membership, at the set's path, repeated members one for one.
	u := tenon.Narrow(tenon.Unknown(num), tenon.NotNull())
	wantDiff(t, "set members", tenon.Set(num, n(1), n(2), u), tenon.Set(num, n(2), n(3), u, u),
		`- .: member 1`, `+ .: member 3`, `+ .: member unknown(not null)`)
	wantDiff(t, "a set within an object", tenon.Object(map[string]tenon.Value{"s": tenon.Set(str, s("a"))}),
		tenon.Object(map[string]tenon.Value{"s": tenon.Set(str)}), `- .s: member "a"`)
}

func TestConformance_DI034_DeepMarksCountOnce(t *testing.T) {
	conformance.Covers(t, "DI-034")
	num := tenon.NumberType()
	n := tenon.NumberFromInt
	d, m := stamp{id: "d", deep: true}, stamp{id: "m"}
	list := tenon.List(num, n(1), n(2))
	wantDiff(t, "a deep mark added", list, tenon.WithMarks(list, d), `~ .: marks [] -> ["d"]`)
	nested := tenon.Object(map[string]tenon.Value{"a": tenon.Object(map[string]tenon.Value{"b": list})})
	wantDiff(t, "a deep mark on an outer part", nested, tenon.WithMarks(nested, d), `~ .: marks [] -> ["d"]`)
	wantDiff(t, "a deep mark on an inner part", nested, tenon.Object(map[string]tenon.Value{"a": tenon.WithMarks(nested.Attribute("a"), d)}),
		`~ .a: marks [] -> ["d"]`)
	wantDiff(t, "a deep mark on a set", tenon.Set(num, n(1)), tenon.WithMarks(tenon.Set(num, n(1)), d), `~ .: marks [] -> ["d"]`)
	// Other changes within still show, and so does a mark a member carries of
	// its own once its holder's deep mark is gone.
	wantDiff(t, "a deep mark and a change within", list, tenon.WithMarks(tenon.List(num, n(1), n(3)), d),
		`~ .: marks [] -> ["d"]`, `~ .[1]: 2 -> marked(3, "d")`)
	wantDiff(t, "a member marking of its own", tenon.WithMarks(list, d), tenon.List(num, tenon.WithMarks(n(1), d), n(2)),
		`~ .: marks ["d"] -> []`, `~ .[0]: marks [] -> ["d"]`)
	wantDiff(t, "a member mark beside a deep one", tenon.WithMarks(list, d), tenon.WithMarks(tenon.List(num, tenon.WithMarks(n(1), m), n(2)), d),
		`~ .[0]: marks [] -> ["m"]`)
}

func TestConformance_DI035_OrderAndSymmetry(t *testing.T) {
	conformance.Covers(t, "DI-035")
	num, str := tenon.NumberType(), tenon.StringType()
	n, s := tenon.NumberFromInt, tenon.String
	m := stamp{id: "m"}
	a := tenon.Object(map[string]tenon.Value{
		"B": n(1), "a": tenon.List(num, n(1), n(2)), "c": tenon.Set(str, s("x"), s("z")),
	})
	b := tenon.Object(map[string]tenon.Value{
		"B": n(2), "a": tenon.WithMarks(tenon.List(num, n(3)), m), "c": tenon.Set(str, s("y")),
	})
	wantDiff(t, "a walk from the top", a, b,
		`~ .B: 1 -> 2`, `~ .a: marks [] -> ["m"]`, `~ .a[0]: 1 -> 3`, `- .a[1]: 2`,
		`- .c: member "x"`, `+ .c: member "y"`, `- .c: member "z"`)
	wantDiff(t, "and the other way", b, a,
		`~ .B: 2 -> 1`, `~ .a: marks ["m"] -> []`, `~ .a[0]: 3 -> 1`, `+ .a[1]: 2`,
		`+ .c: member "x"`, `- .c: member "y"`, `+ .c: member "z"`)

	// Members that are not known interleave by their display forms as the
	// sets hold them, not by those of the members the changes carry, which
	// carry the sets' deep marks: read with its mark, the member added here
	// would come before the one removed.
	e := stamp{id: "e", deep: true}
	u := tenon.Unknown(num)
	wantDiff(t, "members ordered as the sets hold them", tenon.Set(num, u), tenon.WithMarks(tenon.Set(num, tenon.Narrow(u, tenon.NotNull())), e),
		`~ .: marks [] -> ["e"]`, `- .: member unknown`, `+ .: member marked(unknown(not null), "e")`)
	// The display forms are the members', without the type the sets state
	// (DI-010): read with it, as {"a": unknown(number)} and
	// {"a": unknown(number, >= 5)}, the member removed here would come first.
	objects := tenon.ObjectType(map[string]tenon.Type{"a": num})
	wantDiff(t, "members ordered by their display forms as members",
		tenon.Set(objects, tenon.Object(map[string]tenon.Value{"a": u})),
		tenon.Set(objects, tenon.Object(map[string]tenon.Value{"a": tenon.Narrow(u, tenon.NumberMin(n(5), true))})),
		`+ .: member {"a": unknown(>= 5)}`, `- .: member {"a": unknown}`)

	// Over the generator's values, every diff mirrors its reverse, and every
	// path locates the parts its change carries.
	all := values.All()
	for _, x := range all {
		for _, y := range all {
			forward, back := tenon.Diff(x, y), tenon.Diff(y, x)
			turned := make(tenon.Changes, len(forward))
			for i, c := range forward {
				turned[i] = mirrored(c)
			}
			if turned.String() != back.String() {
				t.Errorf("the diff of %v against %v is\n%sand the other way\n%s", x, y, forward, back)
			}
			for _, c := range forward {
				switch c.Kind {
				case tenon.ChangeReplaced, tenon.ChangeRemoved:
					if part, ok := partAt(x, c.Path); !ok || !tenon.Identical(part, c.Old) {
						t.Errorf("%s: the path does not locate %v in %v", c, c.Old, x)
					}
				}
				switch c.Kind {
				case tenon.ChangeReplaced, tenon.ChangeAdded:
					if part, ok := partAt(y, c.Path); !ok || !tenon.Identical(part, c.New) {
						t.Errorf("%s: the path does not locate %v in %v", c, c.New, y)
					}
				case tenon.ChangeMemberAdded, tenon.ChangeMemberRemoved:
					if part, ok := partAt(y, c.Path); !ok || part.Type().Kind() != tenon.KindSet {
						t.Errorf("%s: the path does not locate a set in %v", c, y)
					}
				}
			}
		}
	}
}

func TestConformance_DI036_DiffsWithholdRedactedContents(t *testing.T) {
	conformance.Covers(t, "DI-036")
	num, str := tenon.NumberType(), tenon.StringType()
	secret := stamp{id: "s", redact: true}
	deepSecret := stamp{id: "s", redact: true, deep: true}
	before := tenon.Object(map[string]tenon.Value{
		"password": tenon.WithMarks(tenon.String("hunter2"), secret),
		"keys":     tenon.WithMarks(tenon.Map(str, map[string]tenon.Value{"api": tenon.String("k-one")}), secret),
		"pins":     tenon.WithMarks(tenon.Set(num, tenon.NumberFromInt(1234)), deepSecret),
	})
	after := tenon.Object(map[string]tenon.Value{
		"password": tenon.WithMarks(tenon.String("hunter3"), secret),
		"keys":     tenon.WithMarks(tenon.Map(str, map[string]tenon.Value{"api": tenon.String("k-two"), "new": tenon.String("k-three")}), secret),
		"pins":     tenon.WithMarks(tenon.Set(num, tenon.NumberFromInt(5678)), deepSecret),
	})
	for _, c := range tenon.Diff(before, after) {
		if c.Path.Len() != 1 {
			t.Errorf("%s: a change within a redacted part", c)
		}
	}
	shown := tenon.Diff(before, after).String() + tenon.Diff(after, before).String()
	for _, text := range []string{"hunter", "k-", "api", "new", "1234", "5678"} {
		if strings.Contains(shown, text) {
			t.Errorf("the diff shows %q:\n%s", text, shown)
		}
	}
	wantDiff(t, "redacted parts", before, after,
		`~ .keys: redacted("s") -> redacted("s")`,
		`~ .password: redacted("s") -> redacted("s")`,
		`~ .pins: redacted("s") -> redacted("s")`)
}

func TestConformance_DI037_DiffDisplay(t *testing.T) {
	conformance.Covers(t, "DI-037")
	num := tenon.NumberType()
	n := tenon.NumberFromInt
	if got := tenon.Diff(n(1), n(1)).String(); got != "" {
		t.Errorf("the empty diff displays as %q", got)
	}
	root := tenon.Path{}
	for _, tt := range []struct {
		c    tenon.Change
		want string
	}{
		{tenon.Change{Kind: tenon.ChangeReplaced, Path: root, Old: n(1), New: n(2)}, `~ .: 1 -> 2`},
		{tenon.Change{Kind: tenon.ChangeAdded, Path: root.Index(n(0)), New: tenon.Null(num)}, `+ .[0]: null(number)`},
		{tenon.Change{Kind: tenon.ChangeRemoved, Path: root.Attribute("a b"), Old: tenon.String("x")}, `- ."a b": "x"`},
		{tenon.Change{Kind: tenon.ChangeMemberAdded, Path: root.Attribute("s"), New: n(3)}, `+ .s: member 3`},
		{tenon.Change{Kind: tenon.ChangeMemberRemoved, Path: root.Attribute("s"), Old: n(3)}, `- .s: member 3`},
		{tenon.Change{Kind: tenon.ChangeMarks, Path: root.Index(tenon.String("k")), OldMarks: []tenon.Mark{stamp{id: "b"}, stamp{id: "a"}, stamp{id: "a", policy: tenon.Isolate}}},
			`~ .["k"]: marks ["a", "b"] -> []`},
		// Within a list, set or map the parts show without their type.
		{tenon.Change{Kind: tenon.ChangeReplaced, Path: root.Index(n(0)), Old: tenon.Null(num), New: tenon.Unknown(num), InCollection: true},
			`~ .[0]: null -> unknown`},
		{tenon.Change{Kind: tenon.ChangeAdded, Path: root.Index(n(0)), New: tenon.List(num, tenon.Null(num)), InCollection: true}, `+ .[0]: [null]`},
		{tenon.Change{Kind: tenon.ChangeMemberAdded, Path: root, New: tenon.Narrow(tenon.Unknown(num), tenon.NotNull()), InCollection: true},
			`+ .: member unknown(not null)`},
	} {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("%s change displays as %s, want %s", tt.c.Kind, got, tt.want)
		}
	}
	changes := tenon.Diff(tenon.List(num, n(1)), tenon.List(num, n(2), n(3)))
	if got, want := changes.String(), "~ .[0]: 1 -> 2\n+ .[1]: 3\n"; got != want {
		t.Errorf("a diff displays as %q, want %q", got, want)
	}

	// A part keeps its type above the first list, set or map, where the two
	// values can differ in it, and leaves it out within one, where they
	// cannot: a tuple's elements and an object's attributes keep theirs at
	// the top and leave them out within a list or map.
	str, lists := tenon.StringType(), tenon.ListType(num)
	pair := tenon.TupleType(num)
	wantDiff(t, "a tuple's elements at the top", tenon.Tuple(tenon.Null(num)), tenon.Tuple(tenon.Null(str)), `~ .[0]: null(number) -> null(string)`)
	wantDiff(t, "a tuple within a list", tenon.List(pair, tenon.Tuple(tenon.Null(num))), tenon.List(pair, tenon.Tuple(n(1))), `~ .[0][0]: null -> 1`)
	wantDiff(t, "an attribute within a map",
		tenon.Map(tenon.ObjectType(map[string]tenon.Type{"a": lists}), map[string]tenon.Value{"k": tenon.Object(map[string]tenon.Value{"a": tenon.Null(lists)})}),
		tenon.Map(tenon.ObjectType(map[string]tenon.Type{"a": lists}), map[string]tenon.Value{"k": tenon.Object(map[string]tenon.Value{"a": tenon.List(num)})}),
		`~ .["k"].a: null -> []`)
	wantDiff(t, "an attribute at the top", tenon.Object(map[string]tenon.Value{"a": tenon.Null(lists)}), tenon.Object(map[string]tenon.Value{"a": tenon.List(num)}),
		`~ .a: null(list(number)) -> list(number)[]`)
	wantDiff(t, "a list within a list", tenon.List(lists, tenon.List(num, n(1))), tenon.List(lists, tenon.Null(lists), tenon.Unknown(lists)),
		`~ .[0]: [1] -> null`, `+ .[1]: unknown`)
}

// TestConformance_DI037_DiffTextGrowsWithTheValues holds the text of a diff to
// growing with the values it compares: a change within a list, set or map
// shows its parts without the type the collection's element type fixes, so k
// changed members of a type k lists deep, as replacements, additions,
// removals, member changes, and changes within tuples and objects within a
// collection, display in text that grows with k, where each part spelling its
// type grew with its square. Ordering set members that are not known reads
// them without it too. Four times k allocates under eight times as much to
// diff and display.
func TestConformance_DI037_DiffTextGrowsWithTheValues(t *testing.T) {
	conformance.Covers(t, "DI-037", "DI-035")
	num := tenon.NumberType()
	deep := func(k int) tenon.Type {
		typ := num
		for range k {
			typ = tenon.ListType(typ)
		}
		return typ
	}
	each := func(k int, v func(i int) tenon.Value) []tenon.Value {
		vs := make([]tenon.Value, k)
		for i := range vs {
			vs[i] = v(i)
		}
		return vs
	}
	entries := func(k int, v func(i int) tenon.Value) map[string]tenon.Value {
		es := make(map[string]tenon.Value, k)
		for i := range k {
			es[fmt.Sprintf("k%04d", i)] = v(i)
		}
		return es
	}
	for _, shape := range []struct {
		name string
		pair func(k int) (a, b tenon.Value)
		// want is the diff's display form at k = 2.
		want string
	}{
		{"replaced members", func(k int) (tenon.Value, tenon.Value) {
			typ := deep(k)
			return tenon.List(typ, each(k, func(int) tenon.Value { return tenon.Null(typ) })...),
				tenon.List(typ, each(k, func(int) tenon.Value { return tenon.Unknown(typ) })...)
		}, "~ .[0]: null -> unknown\n~ .[1]: null -> unknown\n"},
		{"added members", func(k int) (tenon.Value, tenon.Value) {
			typ := deep(k)
			return tenon.List(typ), tenon.List(typ, each(k, func(int) tenon.Value { return tenon.Null(typ) })...)
		}, "+ .[0]: null\n+ .[1]: null\n"},
		{"removed entries", func(k int) (tenon.Value, tenon.Value) {
			typ := deep(k)
			return tenon.Map(typ, entries(k, func(int) tenon.Value { return tenon.Null(typ) })), tenon.Map(typ, nil)
		}, "- .[\"k0000\"]: null\n- .[\"k0001\"]: null\n"},
		{"member changes", func(k int) (tenon.Value, tenon.Value) {
			typ := deep(k)
			at := func(from int) func(i int) tenon.Value {
				return func(i int) tenon.Value { return tenon.Narrow(tenon.Unknown(typ), tenon.LengthMin(int64(from+i))) }
			}
			return tenon.Set(typ, each(k, at(1))...), tenon.Set(typ, each(k, at(k+1))...)
		}, "- .: member unknown(length >= 1)\n- .: member unknown(length >= 2)\n" +
			"+ .: member unknown(length >= 3)\n+ .: member unknown(length >= 4)\n"},
		{"tuples within a list", func(k int) (tenon.Value, tenon.Value) {
			typ := deep(k)
			pair := tenon.TupleType(typ)
			return tenon.List(pair, each(k, func(int) tenon.Value { return tenon.Tuple(tenon.Null(typ)) })...),
				tenon.List(pair, each(k, func(int) tenon.Value { return tenon.Tuple(tenon.Unknown(typ)) })...)
		}, "~ .[0][0]: null -> unknown\n~ .[1][0]: null -> unknown\n"},
		{"objects within a map", func(k int) (tenon.Value, tenon.Value) {
			typ := deep(k)
			record := tenon.ObjectType(map[string]tenon.Type{"a": typ})
			return tenon.Map(record, entries(k, func(int) tenon.Value { return tenon.Object(map[string]tenon.Value{"a": tenon.Null(typ)}) })),
				tenon.Map(record, entries(k, func(int) tenon.Value { return tenon.Object(map[string]tenon.Value{"a": tenon.Unknown(typ)}) }))
		}, "~ .[\"k0000\"].a: null -> unknown\n~ .[\"k0001\"].a: null -> unknown\n"},
	} {
		a, b := shape.pair(2)
		if got := tenon.Diff(a, b).String(); got != shape.want {
			t.Errorf("%s: the diff displays as\n%swant\n%s", shape.name, got, shape.want)
		}
		var sizes [2]uint64
		for i, k := range []int{100, 400} {
			a, b := shape.pair(k)
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			_ = tenon.Diff(a, b).String()
			runtime.ReadMemStats(&after)
			sizes[i] = after.TotalAlloc - before.TotalAlloc
		}
		if grew := float64(sizes[1]) / float64(sizes[0]); grew > 8 {
			t.Errorf("%s: four times k allocated %.1f times as much to diff and display (%d bytes, then %d)", shape.name, grew, sizes[0], sizes[1])
		}
	}
}

// A change's mark lists are its own: writing to them leaves the values diffed
// as they were, at the top and within a list alike.
func TestConformance_VA005_ChangesHoldTheirOwnMarks(t *testing.T) {
	conformance.Covers(t, "VA-005", "DI-030")
	a1, a2, b1 := stamp{id: "a1"}, stamp{id: "a2"}, stamp{id: "b1"}
	str := tenon.StringType()
	for _, pair := range [][3]tenon.Value{
		{tenon.WithMarks(tenon.String("x"), a1, a2), tenon.WithMarks(tenon.String("x"), a1, a2), tenon.WithMarks(tenon.String("x"), b1)},
		{
			tenon.List(str, tenon.WithMarks(tenon.String("x"), a1, a2)),
			tenon.List(str, tenon.WithMarks(tenon.String("x"), a1, a2)),
			tenon.List(str, tenon.WithMarks(tenon.String("x"), b1)),
		},
	} {
		a, twin, b := pair[0], pair[1], pair[2]
		first := tenon.Diff(a, b)
		changes := tenon.Diff(a, b)
		if len(changes) == 0 {
			t.Fatalf("no changes between %v and %v", a, b)
		}
		for _, c := range changes {
			for i := range c.OldMarks {
				c.OldMarks[i] = stamp{id: "overwritten"}
			}
			for i := range c.NewMarks {
				c.NewMarks[i] = stamp{id: "overwritten"}
			}
		}
		if !tenon.Identical(a, twin) {
			t.Errorf("writing to a diff's mark lists changed %v", a)
		}
		if got := tenon.Diff(a, b).String(); got != first.String() {
			t.Errorf("after writing to one diff's marks, diffing again gives %s, want %s", got, first)
		}
	}
}
