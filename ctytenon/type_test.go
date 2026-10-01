package ctytenon_test

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
	"github.com/zclconf/go-cty/cty"
)

// TestTypes holds each kind of type to crossing as itself, both ways.
func TestTypes(t *testing.T) {
	var b ctytenon.Bridge
	str, num := tenon.StringType(), tenon.NumberType()
	for _, c := range []struct {
		cty   cty.Type
		tenon tenon.Type
	}{
		{cty.Bool, tenon.BoolType()},
		{cty.Number, num},
		{cty.String, str},
		{cty.List(cty.String), tenon.ListType(str)},
		{cty.Set(cty.Number), tenon.SetType(num)},
		{cty.Map(cty.List(cty.Bool)), tenon.MapType(tenon.ListType(tenon.BoolType()))},
		{cty.EmptyTuple, tenon.TupleType()},
		{cty.Tuple([]cty.Type{cty.String, cty.Number}), tenon.TupleType(str, num)},
		{cty.EmptyObject, tenon.ObjectType(nil)},
		{
			cty.Object(map[string]cty.Type{"name": cty.String, "ports": cty.List(cty.Number), "a name": cty.Set(cty.EmptyObject)}),
			tenon.ObjectType(map[string]tenon.Type{"name": str, "ports": tenon.ListType(num), "a name": tenon.SetType(tenon.ObjectType(nil))}),
		},
		// cty and tenon both normalize a name to NFC.
		{cty.Object(map[string]cty.Type{"cafe\U00000301": cty.String}), tenon.ObjectType(map[string]tenon.Type{"caf\U000000e9": str})},
	} {
		got, err := b.TypeFromCty(c.cty)
		if err != nil || got != c.tenon {
			t.Errorf("TypeFromCty(%#v) = %s, %v; want %s", c.cty, got, err, c.tenon)
		}
		back, err := b.TypeToCty(c.tenon)
		if err != nil || !back.Equals(c.cty) {
			t.Errorf("TypeToCty(%s) = %#v, %v; want %#v", c.tenon, back, err, c.cty)
		}
	}
}

// TestTypesRoundTrip carries random types across and back, from each side.
func TestTypesRoundTrip(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261001))
	for range conformance.Iterations(t, 2000) {
		tt := randomTenonType(r, 4)
		c, err := b.TypeToCty(tt)
		if err != nil {
			t.Fatalf("TypeToCty(%s): %v", tt, err)
		}
		if back, err := b.TypeFromCty(c); err != nil || back != tt {
			t.Fatalf("%s crossed to %#v and back as %s, %v", tt, c, back, err)
		}

		ct := randomCtyType(r, 4)
		tt, err = b.TypeFromCty(ct)
		if err != nil {
			t.Fatalf("TypeFromCty(%#v): %v", ct, err)
		}
		if back, err := b.TypeToCty(tt); err != nil || !back.Equals(ct) {
			t.Fatalf("%#v crossed to %s and back as %#v, %v", ct, tt, back, err)
		}
	}
}

// TestCorpusTypes carries the type of every resolved value of tenon's corpus,
// and of every value within one, across and back. A capsule type, which the
// zero Bridge pairs with none, does not cross.
func TestCorpusTypes(t *testing.T) {
	var b ctytenon.Bridge
	crossed := 0
	for _, v := range within(values.All()) {
		if !v.IsResolved() {
			continue
		}
		tt := v.Type()
		c, err := b.TypeToCty(tt)
		if tt.Kind() == tenon.KindCapsule {
			if err == nil || !strings.Contains(err.Error(), "pairs with no cty type") {
				t.Errorf("TypeToCty(%s) = %#v, %v; want the capsule type refused", tt, c, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("TypeToCty(%s): %v", tt, err)
			continue
		}
		if back, err := b.TypeFromCty(c); err != nil || back != tt {
			t.Errorf("%s crossed to %#v and back as %s, %v", tt, c, back, err)
		}
		crossed++
	}
	if crossed < 150 {
		t.Errorf("crossed %d types, want the corpus's", crossed)
	}
}

// within returns vs and every value within each, outermost first.
func within(vs []tenon.Value) []tenon.Value {
	var out []tenon.Value
	var visit func(v tenon.Value)
	visit = func(v tenon.Value) {
		out = append(out, v)
		if !v.HasContent() {
			return
		}
		switch v.Type().Kind() {
		case tenon.KindList, tenon.KindSet, tenon.KindTuple:
			for e := range v.ElementsSeq() {
				visit(e)
			}
		case tenon.KindMap:
			for _, e := range v.MapEntries() {
				visit(e)
			}
		case tenon.KindObject:
			for _, a := range v.Attributes() {
				visit(a)
			}
		}
	}
	for _, v := range vs {
		visit(v)
	}
	return out
}

// TestWhatIsNotAType holds each cty type that is not one tenon can hold, and
// each tenon type cty cannot, to failing with an error that says why, naming
// the whole type.
func TestWhatIsNotAType(t *testing.T) {
	var b ctytenon.Bridge
	optional := cty.ObjectWithOptionalAttrs(map[string]cty.Type{"a": cty.String}, []string{"a"})
	for _, c := range []struct {
		cty  cty.Type
		want string
	}{
		{cty.DynamicPseudoType, "ctytenon: cty.DynamicPseudoType holds cty.DynamicPseudoType, and is a type constraint rather than a type"},
		{cty.List(cty.DynamicPseudoType), "ctytenon: cty.List(cty.DynamicPseudoType) holds cty.DynamicPseudoType"},
		{optional, "has an object type with optional attributes, and is a type constraint rather than a type"},
		{cty.Map(optional), "ctytenon: cty.Map(cty.ObjectWithOptionalAttrs("},
		{cty.Object(map[string]cty.Type{"": cty.String}), "has an object type whose attribute names tenon refuses: object.empty_name"},
		{cty.List(cty.Capsule("thing", reflect.TypeOf(0))), "holds the capsule type thing, which the Bridge pairs with no tenon type"},
	} {
		got, err := b.TypeFromCty(c.cty)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("TypeFromCty(%#v) = %s, %v; want an error containing %q", c.cty, got, err, c.want)
		}
	}

	type thing struct{}
	capsule := tenon.NewCapsule("thing", tenon.CapsuleOps[thing]{}).Type()
	got, err := b.TypeToCty(tenon.ListType(capsule))
	if want := `ctytenon: list(capsule("thing")) holds the capsule type "thing", which the Bridge pairs with no cty type`; err == nil || err.Error() != want {
		t.Errorf("TypeToCty of a capsule type = %#v, %v; want %q", got, err, want)
	}
}

// TestUsageErrors holds the zero types and constraints of either side, which
// are neither, to usage panics.
func TestUsageErrors(t *testing.T) {
	var b ctytenon.Bridge
	for _, c := range []struct {
		want string
		f    func()
	}{
		{"tenon: usage: TypeFromCty called with cty.NilType, which is not a type", func() { b.TypeFromCty(cty.NilType) }},
		{"tenon: usage: TypeToCty called with the zero Type, which is not a type", func() { b.TypeToCty(tenon.Type{}) }},
		{"tenon: usage: ConstraintFromCty called with cty.NilType, which is not a type constraint", func() { b.ConstraintFromCty(cty.NilType) }},
		{"tenon: usage: ConstraintToCty called with the zero Constraint, which is not a constraint", func() { b.ConstraintToCty(tenon.Constraint{}) }},
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

// names are attribute names a random object type draws from, NFC already,
// spaces and characters beyond ASCII among them.
var names = []string{"a", "b", "name", "a name", "caf\U000000e9", "\U0001F600", "x1", "Z"}

// randomTenonType returns a random tenon type nested at most depth deep.
func randomTenonType(r *rand.Rand, depth int) tenon.Type {
	n := 3
	if depth > 0 {
		n = 8
	}
	switch r.Intn(n) {
	case 0:
		return tenon.BoolType()
	case 1:
		return tenon.NumberType()
	case 2:
		return tenon.StringType()
	case 3:
		return tenon.ListType(randomTenonType(r, depth-1))
	case 4:
		return tenon.SetType(randomTenonType(r, depth-1))
	case 5:
		return tenon.MapType(randomTenonType(r, depth-1))
	case 6:
		elems := make([]tenon.Type, r.Intn(4))
		for i := range elems {
			elems[i] = randomTenonType(r, depth-1)
		}
		return tenon.TupleType(elems...)
	}
	attrs := map[string]tenon.Type{}
	for range r.Intn(4) {
		attrs[names[r.Intn(len(names))]] = randomTenonType(r, depth-1)
	}
	return tenon.ObjectType(attrs)
}

// randomCtyType returns a random cty type nested at most depth deep.
func randomCtyType(r *rand.Rand, depth int) cty.Type {
	n := 3
	if depth > 0 {
		n = 8
	}
	switch r.Intn(n) {
	case 0:
		return cty.Bool
	case 1:
		return cty.Number
	case 2:
		return cty.String
	case 3:
		return cty.List(randomCtyType(r, depth-1))
	case 4:
		return cty.Set(randomCtyType(r, depth-1))
	case 5:
		return cty.Map(randomCtyType(r, depth-1))
	case 6:
		elems := make([]cty.Type, r.Intn(4))
		for i := range elems {
			elems[i] = randomCtyType(r, depth-1)
		}
		return cty.Tuple(elems)
	}
	attrs := map[string]cty.Type{}
	for range r.Intn(4) {
		attrs[names[r.Intn(len(names))]] = randomCtyType(r, depth-1)
	}
	return cty.Object(attrs)
}
