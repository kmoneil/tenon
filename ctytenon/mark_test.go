package ctytenon_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/zclconf/go-cty/cty"
)

// wrapped is the tenon mark a cty mark crosses as in these tests.
type wrapped struct{ m any }

func (w wrapped) MarkID() string               { return fmt.Sprint(w.m) }
func (wrapped) Propagation() tenon.Propagation { return tenon.Propagate }
func (wrapped) Redacting() bool                { return false }

// deep is the tenon mark the cty mark "deep" crosses as, which tenon hands to
// every value within the one it marks.
type deep struct{}

func (deep) MarkID() string                 { return "deep" }
func (deep) Propagation() tenon.Propagation { return tenon.Propagate }
func (deep) Redacting() bool                { return false }
func (deep) Deep() bool                     { return true }

// secret is the tenon mark Terraform's sensitive mark crosses as.
type secret struct{}

func (secret) MarkID() string                 { return "sensitive" }
func (secret) Propagation() tenon.Propagation { return tenon.Propagate }
func (secret) Redacting() bool                { return true }

// marking is a Bridge that maps every cty mark but "refused", and back.
var marking = ctytenon.Bridge{
	MarkFromCty: func(m any) (tenon.Mark, bool) {
		switch m {
		case "refused":
			return nil, false
		case "sensitive":
			return secret{}, true
		case "deep":
			return deep{}, true
		}
		return wrapped{m}, true
	},
	MarkToCty: func(m tenon.Mark) (any, bool) {
		switch m := m.(type) {
		case wrapped:
			return m.m, true
		case secret:
			return "sensitive", true
		case deep:
			return "deep", true
		}
		return nil, false
	},
}

func w(m any) tenon.Mark { return wrapped{m} }

// TestMarks holds marked values to crossing with their marks, both ways. A
// container's marks are on every value read out of it in cty, and on every
// value within it in tenon; the members of a set carry none, the set
// carrying them.
func TestMarks(t *testing.T) {
	a, bb := tenon.String("a"), tenon.String("b")
	for _, c := range []struct {
		name  string
		cty   cty.Value
		tenon tenon.Value
	}{
		{"a marked string", cty.StringVal("a").Mark("note"), tenon.WithMarks(a, w("note"))},
		{"two marks", cty.StringVal("a").Mark("x").Mark("y"), tenon.WithMarks(a, w("x"), w("y"))},
		{"a marked element", cty.ListVal([]cty.Value{cty.StringVal("a").Mark("x"), cty.StringVal("b")}), tenon.List(str, tenon.WithMarks(a, w("x")), bb)},
		{
			"a marked list, which hands its mark to its elements",
			cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b").Mark("y")}).Mark("x"),
			tenon.WithMarks(tenon.List(str, tenon.WithMarks(a, w("x")), tenon.WithMarks(bb, w("x"), w("y"))), w("x")),
		},
		{
			"a marked object within a map",
			cty.MapVal(map[string]cty.Value{"k": cty.ObjectVal(map[string]cty.Value{"n": cty.NumberIntVal(1)}).Mark("x")}),
			tenon.Map(tenon.ObjectType(map[string]tenon.Type{"n": num}), map[string]tenon.Value{"k": tenon.WithMarks(tenon.Object(map[string]tenon.Value{"n": tenon.WithMarks(n(1), w("x"))}), w("x"))}),
		},
		{"a marked set", cty.SetVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}).Mark("x"), tenon.WithMarks(tenon.Set(str, a, bb), w("x"))},
		{"a deep mark", cty.ListVal([]cty.Value{cty.StringVal("a")}).Mark("deep"), tenon.WithMarks(tenon.List(str, a), deep{})},
		{"a marked unknown", cty.UnknownVal(cty.Number).Mark("x"), tenon.WithMarks(tenon.Unknown(num), w("x"))},
		{"a marked null", cty.NullVal(cty.String).Mark("sensitive"), tenon.WithMarks(tenon.Null(str), secret{})},
		{"a marked pending value", cty.DynamicVal.Mark("x"), tenon.WithMarks(tenon.Pending(tenon.Any()), w("x"))},
		{"a marked pending member", cty.TupleVal([]cty.Value{cty.DynamicVal.Mark("x"), cty.NumberIntVal(1)}), tenon.Tuple(tenon.WithMarks(tenon.Pending(tenon.Any()), w("x")), n(1))},
		{
			"a marked object holding DynamicVal, which hands its mark to its members",
			cty.ObjectVal(map[string]cty.Value{"a": cty.DynamicVal, "b": cty.StringVal("b")}).Mark("x"),
			tenon.WithMarks(tenon.Object(map[string]tenon.Value{"a": tenon.WithMarks(tenon.Pending(tenon.Any()), w("x")), "b": tenon.WithMarks(bb, w("x"))}), w("x")),
		},
		{"a deep mark on a tuple holding DynamicVal", cty.TupleVal([]cty.Value{cty.DynamicVal, cty.StringVal("a")}).Mark("deep"), tenon.WithMarks(tenon.Tuple(tenon.Pending(tenon.Any()), a), deep{})},
		{
			"a marked bound",
			cty.UnknownVal(cty.Number).Refine().NumberRangeLowerBound(cty.NumberIntVal(5), true).NewValue().Mark("x"),
			tenon.WithMarks(tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true)), w("x")),
		},
	} {
		got, err := marking.FromCty(c.cty)
		if err != nil || !got.Equal(c.tenon) {
			t.Errorf("%s: FromCty(%#v) = %#v, %v; want %#v", c.name, c.cty, got, err, c.tenon)
		}
		back, err := marking.ToCty(c.tenon)
		if err != nil || !back.RawEquals(c.cty) {
			t.Errorf("%s: ToCty(%v) = %#v, %v; want %#v", c.name, c.tenon, back, err, c.cty)
		}
	}
}

// TestMarksThatSpread holds a mark on a tenon container that tenon does not
// hand to the values within, one that is not deep, to being on them once the
// container crosses to cty and back, since cty hands it to them.
func TestMarksThatSpread(t *testing.T) {
	v := tenon.WithMarks(tenon.Tuple(tenon.String("a"), tenon.List(num, n(1))), w("x"))
	want := tenon.WithMarks(tenon.Tuple(tenon.WithMarks(tenon.String("a"), w("x")), tenon.WithMarks(tenon.List(num, tenon.WithMarks(n(1), w("x"))), w("x"))), w("x"))
	c, err := marking.ToCty(v)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := marking.FromCty(c); err != nil || !got.Equal(want) {
		t.Errorf("%v crossed as %#v and back as %v, %v; want %v", v, c, got, err, want)
	}
}

// TestMarksThatDoNotCross holds a mark the Bridge does not map, either way,
// to failing at the value that carries it, and a failure within a value
// carrying a redacting mark to being located at that value and naming it by
// its placeholder, saying nothing of what it holds.
func TestMarksThatDoNotCross(t *testing.T) {
	thing := cty.Capsule("thing", reflect.TypeOf(0))
	one := 1
	for _, c := range []struct {
		name string
		cty  cty.Value
		want []tenon.Diagnostic
	}{
		{
			"an unmapped mark",
			cty.ObjectVal(map[string]cty.Value{"a": cty.StringVal("x").Mark("refused").Mark("note")}),
			[]tenon.Diagnostic{{Code: ctytenon.CodeUnmappedMark, Path: pathOf("a"), Message: `the value carries the cty marks "refused", which the Bridge maps to no tenon mark`}},
		},
		{
			"failures within a sensitive value",
			cty.ObjectVal(map[string]cty.Value{"creds": cty.ObjectVal(map[string]cty.Value{
				"key":   cty.CapsuleVal(thing, &one),
				"token": cty.StringVal("\xff"),
				"also":  cty.StringVal("\xfe"),
			}).Mark("sensitive")}),
			[]tenon.Diagnostic{
				{Code: tenon.CodeStringInvalidUTF8, Path: pathOf("creds"), Message: `redacted("sensitive") holds a part that does not cross`},
				{Code: ctytenon.CodeUnpairedCapsule, Path: pathOf("creds"), Message: `redacted("sensitive") holds a part that does not cross`},
			},
		},
	} {
		got, err := marking.FromCty(c.cty)
		wantFailure(t, c.name+": FromCty", got, err, c.want)
	}

	type thingT struct{}
	capsule := tenon.NewCapsule("thing", tenon.CapsuleOps[thingT]{})
	for _, c := range []struct {
		name  string
		tenon tenon.Value
		want  []tenon.Diagnostic
	}{
		{"an unmapped mark", tenon.List(str, tenon.WithMarks(tenon.String("a"), label("audited"))), []tenon.Diagnostic{{Code: ctytenon.CodeUnmappedMark, Path: pathOf(0), Message: `the value carries the marks "audited", which the Bridge maps to no cty mark`}}},
		{
			"a failure within a redacted value",
			tenon.Object(map[string]tenon.Value{"creds": tenon.WithMarks(tenon.Map(capsule.Type(), map[string]tenon.Value{"key": capsule.Value(&thingT{})}), secret{})}),
			[]tenon.Diagnostic{{Code: ctytenon.CodeUnpairedCapsule, Path: pathOf("creds"), Message: `redacted("sensitive") holds a part that does not cross`}},
		},
	} {
		got, err := marking.ToCty(c.tenon)
		wantFailure(t, c.name+": ToCty", got, err, c.want)
	}
}

// TestMarksRoundTripFromCty marks random cty values at random paths, each
// path its own mark, and carries them to tenon and back, which gives each as
// it was, marks and all.
func TestMarksRoundTripFromCty(t *testing.T) {
	r := rand.New(rand.NewSource(20261008))
	marked := 0
	for range conformance.Iterations(t, 2000) {
		v := markAtRandom(r, randomCtyValue(r, randomCtyType(r, 3)), cty.Path{})
		if _, pvm := v.UnmarkDeepWithPaths(); len(pvm) > 0 {
			marked++
		}
		tv, err := marking.FromCty(v)
		if err != nil {
			t.Fatalf("FromCty(%#v): %v", v, err)
		}
		back, err := marking.ToCty(tv)
		if err != nil {
			t.Fatalf("%#v crossed as %v and failed back: %v", v, tv, err)
		}
		if back.RawEquals(v) {
			continue
		}
		// tenon can know more than cty said (TestValuesRoundTripFromCty),
		// but the marks are where they were.
		if again, err := marking.FromCty(back); err != nil || !again.Equal(tv) || markPaths(back) != markPaths(v) {
			t.Fatalf("%#v crossed as %v and back as %#v", v, tv, back)
		}
	}
	if marked < 800 {
		t.Errorf("marked %d of the values, want many", marked)
	}
}

// markPaths returns the paths of v's marks and the marks at each, in order.
func markPaths(v cty.Value) string {
	_, pvm := v.UnmarkDeepWithPaths()
	out := make([]string, len(pvm))
	for i, pm := range pvm {
		out[i] = fmt.Sprintf("%#v: %#v", pm.Path, pm.Marks)
	}
	slices.Sort(out)
	return strings.Join(out, "\n")
}

// markAtRandom returns v with marks at random places, at p, other than within
// a set, each place's mark its path.
func markAtRandom(r *rand.Rand, v cty.Value, p cty.Path) cty.Value {
	if v.IsKnown() && !v.IsNull() && (v.Type().IsListType() || v.Type().IsTupleType() || v.Type().IsMapType() || v.Type().IsObjectType()) {
		switch {
		case v.Type().IsMapType() || v.Type().IsObjectType():
			m := map[string]cty.Value{}
			for it := v.ElementIterator(); it.Next(); {
				k, e := it.Element()
				m[k.AsString()] = markAtRandom(r, e, append(p, cty.IndexStep{Key: k}))
			}
			switch {
			case v.Type().IsObjectType():
				v = cty.ObjectVal(m)
			case len(m) > 0:
				v = cty.MapVal(m)
			}
		default:
			var elems []cty.Value
			for it := v.ElementIterator(); it.Next(); {
				k, e := it.Element()
				elems = append(elems, markAtRandom(r, e, append(p, cty.IndexStep{Key: k})))
			}
			switch {
			case v.Type().IsTupleType():
				v = cty.TupleVal(elems)
			case len(elems) > 0:
				v = cty.ListVal(elems)
			}
		}
	}
	if r.Intn(3) == 0 {
		return v.Mark(fmt.Sprintf("%#v", p))
	}
	return v
}

// TestMarksRoundTripFromTenon marks random tenon values at random places, and
// carries them to cty and back, which gives each as it was but that a mark on
// a container is on every value within it, as cty hands it to them.
func TestMarksRoundTripFromTenon(t *testing.T) {
	r := rand.New(rand.NewSource(20261009))
	for range conformance.Iterations(t, 2000) {
		v := tenonMarkAtRandom(r, randomTenonValue(r, randomTenonType(r, 3)), "m")
		c, err := marking.ToCty(v)
		if err != nil {
			t.Fatalf("ToCty(%v): %v", v, err)
		}
		back, err := marking.FromCty(c)
		if want := spread(v, nil); err != nil || !back.Equal(want) {
			t.Fatalf("%v crossed as %#v and back as %v, %v; want %v", v, c, back, err, want)
		}
	}
}

// tenonMarkAtRandom returns v with marks at random places, other than the
// members of a set, each place's mark named for it.
func tenonMarkAtRandom(r *rand.Rand, v tenon.Value, at string) tenon.Value {
	v = rebuild(v, func(i int, key string, e tenon.Value) tenon.Value {
		return tenonMarkAtRandom(r, e, at+"."+key+fmt.Sprint(i))
	})
	if r.Intn(3) == 0 {
		return tenon.WithMarks(v, w(at))
	}
	return v
}

// spread returns v with the marks of each container on every value within it
// but the members of a set, as cty hands them, beside within, the marks of the
// containers around v.
func spread(v tenon.Value, within []tenon.Mark) tenon.Value {
	u, own := tenon.Unmark(v)
	marks := append(append([]tenon.Mark(nil), within...), own...)
	u = rebuild(u, func(_ int, _ string, e tenon.Value) tenon.Value { return spread(e, marks) })
	return tenon.WithMarks(u, marks...)
}

// rebuild returns v, a value that may hold others, holding each as f gives
// it: the members of a list, tuple, map or object, but not a set's.
func rebuild(v tenon.Value, f func(i int, key string, e tenon.Value) tenon.Value) tenon.Value {
	u, marks := tenon.Unmark(v)
	if !u.HasContent() {
		return v
	}
	switch u.Type().Kind() {
	case tenon.KindList, tenon.KindTuple:
		var elems []tenon.Value
		for e := range u.ElementsSeq() {
			elems = append(elems, f(len(elems), "", e))
		}
		if u.Type().Kind() == tenon.KindTuple {
			u = tenon.Tuple(elems...)
		} else {
			u = tenon.List(u.Type().ElementType(), elems...)
		}
	case tenon.KindMap:
		entries := map[string]tenon.Value{}
		for k, e := range u.MapEntries() {
			entries[k] = f(0, k, e)
		}
		u = tenon.Map(u.Type().ElementType(), entries)
	case tenon.KindObject:
		attrs := map[string]tenon.Value{}
		for k, e := range u.Attributes() {
			attrs[k] = f(0, k, e)
		}
		u = tenon.Object(attrs)
	}
	return tenon.WithMarks(u, marks...)
}
