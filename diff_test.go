package tenon_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/conformance/values"
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

// partAt returns the part of v that p locates, and whether there is one.
func partAt(v tenon.Value, p tenon.Path) (tenon.Value, bool) {
	for _, s := range p.Steps() {
		if !v.HasContent() {
			return tenon.Value{}, false
		}
		switch k := v.Type().Kind(); {
		case s.Kind() == tenon.StepAttribute && k == tenon.KindObject && v.Type().HasAttribute(s.Name()):
			v = v.Attribute(s.Name())
		case s.Kind() == tenon.StepIndex && (k == tenon.KindList || k == tenon.KindTuple):
			i, ok := s.Key().AsInt64()
			if !ok || i < 0 || int(i) >= v.Len() {
				return tenon.Value{}, false
			}
			v = v.Index(int(i))
		case s.Kind() == tenon.StepIndex && k == tenon.KindMap && s.Key().Type() == tenon.StringType():
			e, ok := v.MapElement(s.Key().AsString())
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
	m := tenon.Change{Kind: c.Kind, Path: c.Path, Old: c.New, New: c.Old, OldMarks: c.NewMarks, NewMarks: c.OldMarks}
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
	blank := tenon.Capsule("blank", tenon.CapsuleOps[celsius]{})
	blankTuple := tenon.Tuple(blank, num)
	m1 := tenon.TupleVal(tenon.CapsuleVal(blank, &celsius{1}), tenon.Unknown(num))
	m2 := tenon.TupleVal(tenon.CapsuleVal(blank, &celsius{2}), tenon.Unknown(num))
	if m1.String() != m2.String() {
		t.Fatalf("the members read %s and %s, not alike", m1, m2)
	}
	mirrorOf(t, "no operations", tenon.SetVal(blankTuple, m1), tenon.SetVal(blankTuple, m2))

	// A capsule type that encodes but does not display: the members read
	// alike and their encodings differ, so the one with the lesser encoding
	// leads from either side, in any run.
	coded := tenon.Capsule("coded", tenon.CapsuleOps[celsius]{
		Equals: func(a, b *celsius) bool { return *a == *b },
		Hash:   func(v *celsius) uint64 { return uint64(v.degrees) },
		Encoding: &tenon.CapsuleEncoding[celsius]{
			ID:     "t/coded",
			Type:   num,
			Encode: func(v *celsius) tenon.Value { return tenon.NumberFromInt(v.degrees) },
			Decode: func(v tenon.Value) (*celsius, []tenon.Diagnostic) { i, _ := v.AsInt64(); return &celsius{i}, nil },
		},
	})
	codedTuple := tenon.Tuple(coded, num)
	lesser := tenon.TupleVal(tenon.CapsuleVal(coded, &celsius{1}), tenon.Unknown(num))
	greater := tenon.TupleVal(tenon.CapsuleVal(coded, &celsius{2}), tenon.Unknown(num))
	if lesser.String() != greater.String() {
		t.Fatalf("the members read %s and %s, not alike", lesser, greater)
	}
	forward := mirrorOf(t, "encodings differ", tenon.SetVal(codedTuple, lesser), tenon.SetVal(codedTuple, greater))
	if len(forward) != 2 || forward[0].Kind != tenon.ChangeMemberRemoved || !tenon.Identical(forward[0].Old, lesser) {
		t.Errorf("the member with the lesser encoding does not lead: %s", forward)
	}
}

func TestConformance_DI030_ChangesAndWhatTheyCarry(t *testing.T) {
	conformance.Covers(t, "DI-030")
	num, str := tenon.NumberType(), tenon.StringType()
	n, s := tenon.NumberFromInt, tenon.String
	m := stamp{id: "m"}
	a := tenon.ObjectVal(map[string]tenon.Value{
		"gone": s("x"), "list": tenon.ListVal(num, n(1)), "set": tenon.SetVal(str, s("a")), "same": n(1),
		"swap": n(1), "marked": tenon.ListVal(num),
	})
	b := tenon.ObjectVal(map[string]tenon.Value{
		"list": tenon.ListVal(num, n(1), n(2)), "new": s("y"), "set": tenon.SetVal(str, s("b")), "same": n(1),
		"swap": s("1"), "marked": tenon.WithMarks(tenon.ListVal(num), m),
	})
	got := tenon.Diff(a, b)
	root := tenon.Path{}
	want := tenon.Changes{
		{Kind: tenon.ChangeRemoved, Path: root.Attribute("gone"), Old: s("x")},
		{Kind: tenon.ChangeAdded, Path: root.Attribute("list").Index(n(1)), New: n(2)},
		{Kind: tenon.ChangeMarks, Path: root.Attribute("marked"), NewMarks: []tenon.Mark{m}},
		{Kind: tenon.ChangeAdded, Path: root.Attribute("new"), New: s("y")},
		{Kind: tenon.ChangeMemberRemoved, Path: root.Attribute("set"), Old: s("a")},
		{Kind: tenon.ChangeMemberAdded, Path: root.Attribute("set"), New: s("b")},
		{Kind: tenon.ChangeReplaced, Path: root.Attribute("swap"), Old: n(1), New: s("1")},
	}
	if len(got) != len(want) {
		t.Fatalf("the diff has %d changes, want %d:\n%s", len(got), len(want), got)
	}
	same := func(x, y tenon.Value) bool {
		return x == (tenon.Value{}) && y == (tenon.Value{}) || x != (tenon.Value{}) && y != (tenon.Value{}) && tenon.Identical(x, y)
	}
	for i, c := range got {
		w := want[i]
		if c.Kind != w.Kind || !c.Path.Equal(w.Path) || !same(c.Old, w.Old) || !same(c.New, w.New) ||
			!slices.Equal(c.OldMarks, w.OldMarks) || !slices.Equal(c.NewMarks, w.NewMarks) {
			t.Errorf("change %d is %s (%s), want %s (%s)", i, c, c.Kind, w, w.Kind)
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
	l := tenon.ListVal(tenon.NumberType(), tenon.NumberFromInt(1))
	if len(tenon.Diff(l, tenon.WithMarks(l, d))) == 0 {
		t.Error("a list and its deeply marked twin have an empty diff")
	}
}

func TestConformance_DI032_WhereTheDiffLooksWithin(t *testing.T) {
	conformance.Covers(t, "DI-032")
	num, str := tenon.NumberType(), tenon.StringType()
	n, s := tenon.NumberFromInt, tenon.String
	one := tenon.ListVal(num, n(1))
	// Parts that do not match are replaced whole.
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		{"kinds that differ", tenon.TupleVal(n(1)), one, `~ .: [1] -> list(number)[1]`},
		{"a map and an object", tenon.MapVal(num, map[string]tenon.Value{"a": n(1)}), tenon.ObjectVal(map[string]tenon.Value{"a": n(1)}),
			`~ .: map(number){"a": 1} -> {"a": 1}`},
		{"element types that differ", tenon.ListVal(num), tenon.ListVal(str), `~ .: list(number)[] -> list(string)[]`},
		{"a null", tenon.NullVal(tenon.List(num)), one, `~ .: null(list(number)) -> list(number)[1]`},
		{"an unknown", tenon.Unknown(tenon.List(num)), one, `~ .: unknown(list(number)) -> list(number)[1]`},
		{"a pending value", tenon.Pending(tenon.Any()), one, `~ .: pending(any) -> list(number)[1]`},
		{"an error value", tenon.ErrorVal(tenon.Diagnostic{Code: "app.failed", Message: "x"}), one, `~ .: error(app.failed: "x") -> list(number)[1]`},
		{"scalars", s("a"), s("b"), `~ .: "a" -> "b"`},
		{"a redacted container", tenon.WithMarks(one, stamp{id: "s", redact: true}), tenon.WithMarks(tenon.ListVal(num, n(2)), stamp{id: "s", redact: true}),
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
	wantDiff(t, "a marked list with a new element", one, tenon.WithMarks(tenon.ListVal(num, n(1), n(2)), m),
		`~ .: marks [] -> ["m"]`, `+ .[1]: 2`)
	wantDiff(t, "tuples of other types", tenon.TupleVal(n(1), s("a")), tenon.TupleVal(n(1), n(2)), `~ .[1]: "a" -> 2`)
	wantDiff(t, "objects of other attributes", tenon.ObjectVal(map[string]tenon.Value{"a": n(1)}), tenon.ObjectVal(map[string]tenon.Value{"b": n(1)}),
		`- .a: 1`, `+ .b: 1`)
	wantDiff(t, "a list holding an unknown", tenon.ListVal(num, tenon.Unknown(num)), tenon.ListVal(num, n(3)), `~ .[0]: unknown(number) -> 3`)
}

func TestConformance_DI033_MembersCompare(t *testing.T) {
	conformance.Covers(t, "DI-033")
	num, str := tenon.NumberType(), tenon.StringType()
	n, s := tenon.NumberFromInt, tenon.String
	// Lists compare by index, with no alignment.
	wantDiff(t, "an element inserted first", tenon.ListVal(num, n(1), n(2)), tenon.ListVal(num, n(0), n(1), n(2)),
		`~ .[0]: 1 -> 0`, `~ .[1]: 2 -> 1`, `+ .[2]: 2`)
	wantDiff(t, "elements removed", tenon.TupleVal(n(1), s("a"), n(3)), tenon.TupleVal(n(1)), `- .[1]: "a"`, `- .[2]: 3`)
	// Maps and objects by name, nested paths extended step by step.
	wantDiff(t, "map entries", tenon.MapVal(num, map[string]tenon.Value{"a b": n(1), "c": n(2)}), tenon.MapVal(num, map[string]tenon.Value{"c": n(3), "d": n(4)}),
		`- .["a b"]: 1`, `~ .["c"]: 2 -> 3`, `+ .["d"]: 4`)
	wantDiff(t, "nested attributes",
		tenon.ObjectVal(map[string]tenon.Value{"x y": tenon.ObjectVal(map[string]tenon.Value{"z": tenon.ListVal(str, s("a"))})}),
		tenon.ObjectVal(map[string]tenon.Value{"x y": tenon.ObjectVal(map[string]tenon.Value{"z": tenon.ListVal(str, s("b"))})}),
		`~ ."x y".z[0]: "a" -> "b"`)
	// Sets by membership, at the set's path, repeated members one for one.
	u := tenon.Narrow(tenon.Unknown(num), tenon.NotNull())
	wantDiff(t, "set members", tenon.SetVal(num, n(1), n(2), u), tenon.SetVal(num, n(2), n(3), u, u),
		`- .: member 1`, `+ .: member 3`, `+ .: member unknown(number, not null)`)
	wantDiff(t, "a set within an object", tenon.ObjectVal(map[string]tenon.Value{"s": tenon.SetVal(str, s("a"))}),
		tenon.ObjectVal(map[string]tenon.Value{"s": tenon.SetVal(str)}), `- .s: member "a"`)
}

func TestConformance_DI034_DeepMarksCountOnce(t *testing.T) {
	conformance.Covers(t, "DI-034")
	num := tenon.NumberType()
	n := tenon.NumberFromInt
	d, m := stamp{id: "d", deep: true}, stamp{id: "m"}
	list := tenon.ListVal(num, n(1), n(2))
	wantDiff(t, "a deep mark added", list, tenon.WithMarks(list, d), `~ .: marks [] -> ["d"]`)
	nested := tenon.ObjectVal(map[string]tenon.Value{"a": tenon.ObjectVal(map[string]tenon.Value{"b": list})})
	wantDiff(t, "a deep mark on an outer part", nested, tenon.WithMarks(nested, d), `~ .: marks [] -> ["d"]`)
	wantDiff(t, "a deep mark on an inner part", nested, tenon.ObjectVal(map[string]tenon.Value{"a": tenon.WithMarks(nested.Attribute("a"), d)}),
		`~ .a: marks [] -> ["d"]`)
	wantDiff(t, "a deep mark on a set", tenon.SetVal(num, n(1)), tenon.WithMarks(tenon.SetVal(num, n(1)), d), `~ .: marks [] -> ["d"]`)
	// Other changes within still show, and so does a mark a member carries of
	// its own once its holder's deep mark is gone.
	wantDiff(t, "a deep mark and a change within", list, tenon.WithMarks(tenon.ListVal(num, n(1), n(3)), d),
		`~ .: marks [] -> ["d"]`, `~ .[1]: 2 -> marked(3, "d")`)
	wantDiff(t, "a member marking of its own", tenon.WithMarks(list, d), tenon.ListVal(num, tenon.WithMarks(n(1), d), n(2)),
		`~ .: marks ["d"] -> []`, `~ .[0]: marks [] -> ["d"]`)
	wantDiff(t, "a member mark beside a deep one", tenon.WithMarks(list, d), tenon.WithMarks(tenon.ListVal(num, tenon.WithMarks(n(1), m), n(2)), d),
		`~ .[0]: marks [] -> ["m"]`)
}

func TestConformance_DI035_OrderAndSymmetry(t *testing.T) {
	conformance.Covers(t, "DI-035")
	num, str := tenon.NumberType(), tenon.StringType()
	n, s := tenon.NumberFromInt, tenon.String
	m := stamp{id: "m"}
	a := tenon.ObjectVal(map[string]tenon.Value{
		"B": n(1), "a": tenon.ListVal(num, n(1), n(2)), "c": tenon.SetVal(str, s("x"), s("z")),
	})
	b := tenon.ObjectVal(map[string]tenon.Value{
		"B": n(2), "a": tenon.WithMarks(tenon.ListVal(num, n(3)), m), "c": tenon.SetVal(str, s("y")),
	})
	wantDiff(t, "a walk from the top", a, b,
		`~ .B: 1 -> 2`, `~ .a: marks [] -> ["m"]`, `~ .a[0]: 1 -> 3`, `- .a[1]: 2`,
		`- .c: member "x"`, `+ .c: member "y"`, `- .c: member "z"`)
	wantDiff(t, "and the other way", b, a,
		`~ .B: 2 -> 1`, `~ .a: marks ["m"] -> []`, `~ .a[0]: 3 -> 1`, `+ .a[1]: 2`,
		`+ .c: member "x"`, `- .c: member "y"`, `+ .c: member "z"`)

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
	before := tenon.ObjectVal(map[string]tenon.Value{
		"password": tenon.WithMarks(tenon.String("hunter2"), secret),
		"keys":     tenon.WithMarks(tenon.MapVal(str, map[string]tenon.Value{"api": tenon.String("k-one")}), secret),
		"pins":     tenon.WithMarks(tenon.SetVal(num, tenon.NumberFromInt(1234)), deepSecret),
	})
	after := tenon.ObjectVal(map[string]tenon.Value{
		"password": tenon.WithMarks(tenon.String("hunter3"), secret),
		"keys":     tenon.WithMarks(tenon.MapVal(str, map[string]tenon.Value{"api": tenon.String("k-two"), "new": tenon.String("k-three")}), secret),
		"pins":     tenon.WithMarks(tenon.SetVal(num, tenon.NumberFromInt(5678)), deepSecret),
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
		{tenon.Change{Kind: tenon.ChangeAdded, Path: root.Index(n(0)), New: tenon.NullVal(num)}, `+ .[0]: null(number)`},
		{tenon.Change{Kind: tenon.ChangeRemoved, Path: root.Attribute("a b"), Old: tenon.String("x")}, `- ."a b": "x"`},
		{tenon.Change{Kind: tenon.ChangeMemberAdded, Path: root.Attribute("s"), New: n(3)}, `+ .s: member 3`},
		{tenon.Change{Kind: tenon.ChangeMemberRemoved, Path: root.Attribute("s"), Old: n(3)}, `- .s: member 3`},
		{tenon.Change{Kind: tenon.ChangeMarks, Path: root.Index(tenon.String("k")), OldMarks: []tenon.Mark{stamp{id: "b"}, stamp{id: "a"}, stamp{id: "a", policy: tenon.Isolate}}},
			`~ .["k"]: marks ["a", "b"] -> []`},
	} {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("%s change displays as %s, want %s", tt.c.Kind, got, tt.want)
		}
	}
	changes := tenon.Diff(tenon.ListVal(num, n(1)), tenon.ListVal(num, n(2), n(3)))
	if got, want := changes.String(), "~ .[0]: 1 -> 2\n+ .[1]: 3\n"; got != want {
		t.Errorf("a diff displays as %q, want %q", got, want)
	}
}
