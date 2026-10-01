package ctytenon_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/zclconf/go-cty/cty"
)

type point struct{ x, y int }

// name and code are two types that implement fmt.Stringer.
type (
	name string
	code int
)

func (n name) String() string { return string(n) }
func (c code) String() string { return fmt.Sprint(int(c)) }

var (
	ctyPoint   = cty.Capsule("point", reflect.TypeOf(point{}))
	tenonPoint = tenon.NewCapsule("point", tenon.CapsuleOps[point]{})
	pairing    = ctytenon.Bridge{Capsules: []ctytenon.CapsulePair{ctytenon.PairCapsules(ctyPoint, tenonPoint)}}
)

// TestCapsules holds a paired capsule type, and constraints and values of
// it, to crossing as its pair, both ways, a value holding the same pointer.
func TestCapsules(t *testing.T) {
	pt := tenonPoint.Type()
	if got, err := pairing.TypeFromCty(cty.List(ctyPoint)); err != nil || got != tenon.ListType(pt) {
		t.Errorf("TypeFromCty of a list of points = %s, %v", got, err)
	}
	if got, err := pairing.TypeToCty(tenon.MapType(pt)); err != nil || !got.Equals(cty.Map(ctyPoint)) {
		t.Errorf("TypeToCty of a map of points = %#v, %v", got, err)
	}
	if got, err := pairing.ConstraintFromCty(cty.Tuple([]cty.Type{ctyPoint, cty.DynamicPseudoType})); err != nil || !got.Equal(tenon.TupleOf(tenon.Exactly(pt), tenon.Any())) {
		t.Errorf("ConstraintFromCty of a tuple of a point = %v, %v", got, err)
	}

	p, q := &point{1, 2}, &point{3, 4}
	for _, c := range []struct {
		name  string
		cty   cty.Value
		tenon tenon.Value
	}{
		{"a capsule value", cty.CapsuleVal(ctyPoint, p), tenonPoint.Value(p)},
		{"capsule values in a list", cty.ListVal([]cty.Value{cty.CapsuleVal(ctyPoint, p), cty.CapsuleVal(ctyPoint, q)}), tenon.List(pt, tenonPoint.Value(p), tenonPoint.Value(q))},
		{"a null and an unknown capsule", cty.TupleVal([]cty.Value{cty.NullVal(ctyPoint), cty.UnknownVal(ctyPoint)}), tenon.Tuple(tenon.Null(pt), tenon.Unknown(pt))},
		{"an empty set of capsules", cty.SetValEmpty(ctyPoint), tenon.Set(pt)},
	} {
		got, err := pairing.FromCty(c.cty)
		if err != nil || !got.Equal(c.tenon) {
			t.Errorf("%s: FromCty(%#v) = %v, %v; want %v", c.name, c.cty, got, err, c.tenon)
		}
		back, err := pairing.ToCty(c.tenon)
		if err != nil || !back.RawEquals(c.cty) {
			t.Errorf("%s: ToCty(%v) = %#v, %v; want %#v", c.name, c.tenon, back, err, c.cty)
		}
	}

	// The same pointer crosses, either way.
	v, _ := pairing.FromCty(cty.CapsuleVal(ctyPoint, p))
	if got, ok := tenonPoint.Of(v); !ok || got != p {
		t.Errorf("a capsule value crossed holding %p, want %p", got, p)
	}
	c, _ := pairing.ToCty(tenonPoint.Value(q))
	if got := c.EncapsulatedValue(); got != q {
		t.Errorf("a capsule value crossed back holding %p, want %p", got, q)
	}
}

// TestCapsulesThatDoNotCross holds a capsule type paired with none, and a
// capsule value holding what its pair does not, to failing where they lie.
func TestCapsulesThatDoNotCross(t *testing.T) {
	other := cty.Capsule("other", reflect.TypeOf(0))
	// A capsule type of an interface type holds pointers to values of any
	// type that implements it, where its pair holds one.
	stringer := cty.Capsule("stringer", reflect.TypeOf((*fmt.Stringer)(nil)).Elem())
	stringers := ctytenon.Bridge{Capsules: []ctytenon.CapsulePair{ctytenon.PairCapsules(stringer, tenon.NewCapsule("name", tenon.CapsuleOps[name]{}))}}
	one := 1
	for _, c := range []struct {
		name   string
		bridge ctytenon.Bridge
		cty    cty.Value
		want   []tenon.Diagnostic
	}{
		{"an unpaired capsule type", pairing, cty.ObjectVal(map[string]cty.Value{"o": cty.CapsuleVal(other, &one)}), []tenon.Diagnostic{{Code: ctytenon.CodeUnpairedCapsule, Path: pathOf("o")}}},
		{
			"a pointer of another type",
			stringers,
			cty.ListVal([]cty.Value{cty.CapsuleVal(stringer, new(name)), cty.CapsuleVal(stringer, new(code))}),
			[]tenon.Diagnostic{{Code: ctytenon.CodeUnpairedCapsule, Path: pathOf(1), Message: `the value holds a *ctytenon_test.code, which the tenon capsule type capsule("name") does not hold`}},
		},
		{"a nil pointer", pairing, cty.CapsuleVal(ctyPoint, (*point)(nil)), []tenon.Diagnostic{{Code: ctytenon.CodeUnpairedCapsule, Message: "the value holds a nil *ctytenon_test.point, which no tenon capsule value holds"}}},
	} {
		got, err := c.bridge.FromCty(c.cty)
		wantFailure(t, c.name+": FromCty", got, err, c.want)
	}

	type thing struct{}
	unpaired := tenon.NewCapsule("thing", tenon.CapsuleOps[thing]{})
	got, err := pairing.ToCty(tenon.List(unpaired.Type(), unpaired.Value(&thing{})))
	wantFailure(t, "an unpaired tenon capsule type: ToCty", got, err, []tenon.Diagnostic{{Code: ctytenon.CodeUnpairedCapsule, Path: pathOf(0)}})
}

// TestPairCapsulesUsageErrors holds what is not a pair to usage panics.
func TestPairCapsulesUsageErrors(t *testing.T) {
	for _, c := range []struct {
		want string
		f    func()
	}{
		{"tenon: usage: PairCapsules called with cty.String, which is not a cty capsule type", func() { ctytenon.PairCapsules(cty.String, tenonPoint) }},
		{"tenon: usage: PairCapsules called with a nil *tenon.CapsuleType", func() { ctytenon.PairCapsules[point](ctyPoint, nil) }},
		{`tenon: usage: PairCapsules called with cty.Capsule("count", reflect.TypeOf(0)), whose values cannot hold a *ctytenon_test.point`, func() { ctytenon.PairCapsules(cty.Capsule("count", reflect.TypeOf(0)), tenonPoint) }},
		{"tenon: usage: Bridge.Capsules holds the zero CapsulePair, which pairs nothing; PairCapsules makes one", func() {
			ctytenon.Bridge{Capsules: []ctytenon.CapsulePair{{}}}.TypeFromCty(ctyPoint)
		}},
	} {
		func() {
			defer func() {
				if r := recover(); r != c.want {
					t.Errorf("recovered %v, want %q", r, c.want)
				}
			}()
			c.f()
		}()
	}
}

// TestCapsulesRoundTrip carries random lists, maps and objects of paired
// capsule values, nulls and unknown values among them, to tenon and back,
// and back again, each holding the same pointers.
func TestCapsulesRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(20261010))
	for range conformance.Iterations(t, 1000) {
		var elems []cty.Value
		for range 1 + r.Intn(4) {
			switch r.Intn(5) {
			case 0:
				elems = append(elems, cty.NullVal(ctyPoint))
			case 1:
				elems = append(elems, cty.UnknownVal(ctyPoint))
			default:
				elems = append(elems, cty.CapsuleVal(ctyPoint, &point{r.Intn(9), r.Intn(9)}))
			}
		}
		attrs := map[string]cty.Value{"list": cty.ListVal(elems), "first": elems[0]}
		v := cty.ObjectVal(attrs)
		tv, err := pairing.FromCty(v)
		if err != nil {
			t.Fatalf("FromCty(%#v): %v", v, err)
		}
		back, err := pairing.ToCty(tv)
		if err != nil || !back.RawEquals(v) {
			t.Fatalf("%#v crossed as %v and back as %#v, %v", v, tv, back, err)
		}
		if again, err := pairing.FromCty(back); err != nil || !again.Equal(tv) {
			t.Fatalf("%#v crossed back as %v, %v; want %v", back, again, err, tv)
		}
	}
}
