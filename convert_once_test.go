package tenon_test

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// shapedLike returns a constraint of much the shape of t, which a value of t
// often converts to: parts left open, kept, or given as another collection,
// so that a conversion unifies members of differing types and fits them to
// what they unify to, at every level.
func shapedLike(r *rand.Rand, t tenon.Type) tenon.Constraint {
	if r.Intn(4) == 0 {
		return tenon.Any()
	}
	collection := func(elem tenon.Constraint) tenon.Constraint {
		switch r.Intn(3) {
		case 0:
			return tenon.SetOf(elem)
		case 1:
			return tenon.MapOf(elem)
		}
		return tenon.ListOf(elem)
	}
	switch t.Kind() {
	case tenon.KindList, tenon.KindSet, tenon.KindMap:
		return collection(shapedLike(r, t.ElementType()))
	case tenon.KindTuple:
		elems := t.TupleElementTypes()
		if len(elems) > 0 && r.Intn(2) == 0 {
			return collection(shapedLike(r, elems[r.Intn(len(elems))]))
		}
		members := make([]tenon.Constraint, len(elems))
		for i, e := range elems {
			members[i] = shapedLike(r, e)
		}
		return tenon.TupleOf(members...)
	case tenon.KindObject:
		names := t.AttributeNames()
		if len(names) > 0 && r.Intn(3) == 0 {
			return collection(shapedLike(r, t.AttributeType(names[r.Intn(len(names))])))
		}
		fields := map[string]tenon.Field{}
		for _, name := range names {
			if r.Intn(4) > 0 {
				fields[name] = tenon.Field{Constraint: shapedLike(r, t.AttributeType(name)), Required: r.Intn(2) == 0}
			}
		}
		if r.Intn(3) == 0 {
			fields["extra"] = tenon.Optional(tenon.Exactly(num))
		}
		return tenon.ObjectWith(fields, r.Intn(2) == 0)
	case tenon.KindNumber, tenon.KindBool:
		if r.Intn(2) == 0 {
			return tenon.Exactly(str)
		}
	}
	return tenon.Exactly(t)
}

// growing returns tuples nested depth deep around a tuple of objects: each
// level holds the level below it and a chain of one-member tuples ending in
// an object of an attribute of its own, so that converted to collections the
// element type grows at every level, and the objects below are given every
// attribute above them. Values within carry marks of every kind, some are
// null or unknown, and a secret mark redacts one now and then.
func growing(r *rand.Rand, depth int) tenon.Value {
	secret := stamp{id: "secret", redact: true}
	marks := []tenon.Mark{markPlain, markDeep, markIsolated, note{"p", "x"}, secret}
	mark := func(v tenon.Value) tenon.Value {
		if r.Intn(4) == 0 {
			return tenon.WithMarks(v, marks[r.Intn(len(marks))])
		}
		return v
	}
	object := func(name string) tenon.Value {
		o := tenon.Object(map[string]tenon.Value{name: n(int64(r.Intn(3)))})
		switch r.Intn(8) {
		case 0:
			return tenon.Null(o.Type())
		case 1:
			return tenon.Unknown(o.Type())
		}
		return mark(o)
	}
	bottom := make([]tenon.Value, 1+r.Intn(3))
	for i := range bottom {
		bottom[i] = object("x")
	}
	v := tenon.Tuple(bottom...)
	for level := depth - 1; level >= 0; level-- {
		chain := object(fmt.Sprintf("a%d", level))
		for range r.Intn(3) {
			chain = mark(tenon.Tuple(chain))
		}
		v = mark(tenon.Tuple(v, chain))
	}
	return v
}

// nestedCollections returns a constraint of depth collections around Any,
// each a list, a set or a map as r chooses.
func nestedCollections(r *rand.Rand, depth int) tenon.Constraint {
	c := tenon.Any()
	for range depth {
		switch r.Intn(4) {
		case 0:
			c = tenon.SetOf(c)
		default:
			c = tenon.ListOf(c)
		}
	}
	return c
}

// TestConformance_CV021_MembersAreBuiltOnce holds the conversion, which builds
// each member once, at the element type the levels above it settle, to the
// reference that converts a container's members and fits them to its element
// type at every level (CV-021): the same result, identical in its type, its
// contents, its marks and its failures, for random values and constraints of
// every kind under both policies, for constraints shaped like the value, and
// for values whose element type grows at every level.
func TestConformance_CV021_MembersAreBuiltOnce(t *testing.T) {
	conformance.Covers(t, "CV-021", "CV-033")
	r := rand.New(rand.NewSource(1337))
	g := generator{r}
	policies := []tenon.Policy{tenon.Safe, tenon.Unsafe}
	var cases, built int
	check := func(what string, v tenon.Value, c tenon.Constraint, p tenon.Policy) {
		t.Helper()
		if v.IsError() {
			return
		}
		cases++
		converted, fitted := tenon.ConvertBothWays(v, c, p)
		if !tenon.Identical(converted, fitted) {
			t.Errorf("%s: %v converted to %v under the %v policy gives %v, where converting and fitting at every level gives %v",
				what, v, c, p, converted, fitted)
			return
		}
		if !converted.IsError() && !converted.IsPending() {
			built++
		}
	}
	for range conformance.Iterations(t, 1500) {
		p := policies[r.Intn(2)]
		v := g.top()
		check("a random value to a random constraint", v, values.RandomConstraint(r, 3, degrees.Type()), p)
		if v.IsResolved() {
			check("a random value to a constraint of its shape", v, shapedLike(r, v.Type()), p)
		}
		depth := 1 + r.Intn(5)
		check("a growing value", growing(r, depth), nestedCollections(r, depth+1), p)
	}
	if cases < 3000 || built < 1000 {
		t.Errorf("%d cases, of which %d converted; want at least 3,000 and 1,000", cases, built)
	}
}

// TestConformance_CV033_ASetKeepsWhatItsMembersGaveItWhenWidened holds a set
// that a conversion builds to the marks its members give it at every depth,
// Isolate ones included, whether or not the collection holding it widens its
// element type: two lists of tuples, one holding a number under an Isolate
// mark, converted to sets within a list whose element type takes the objects
// of both. Fitting the set to the wider type once rebuilt it from its members,
// which carry no marks, and lost the Isolate mark the member had given it.
func TestConformance_CV033_ASetKeepsWhatItsMembersGaveItWhenWidened(t *testing.T) {
	conformance.Covers(t, "CV-033", "MK-006")
	iso := stamp{id: "iso", policy: tenon.Isolate}
	a := tenon.Tuple(tenon.WithMarks(n(1), iso), tenon.Object(map[string]tenon.Value{"a": n(1)}))
	b := tenon.Tuple(n(2), tenon.Object(map[string]tenon.Value{"b": n(2)}))
	c := tenon.ListOf(tenon.SetOf(tenon.Any()))
	alone := tenon.Convert(tenon.Tuple(tenon.List(a.Type(), a)), c, tenon.Unsafe)
	widened := tenon.Convert(tenon.Tuple(tenon.List(a.Type(), a), tenon.List(b.Type(), b)), c, tenon.Unsafe)
	for _, tt := range []struct {
		name string
		v    tenon.Value
	}{{"alone", alone}, {"beside a set that widens it", widened}} {
		if tt.v.IsError() || !tenon.HasMark(tt.v.Elements()[0], iso) {
			t.Errorf("%s: the set converted from the list holding the marked number is %v, not carrying %v", tt.name, tt.v, iso)
		}
	}
	if !widened.IsError() && tenon.HasMark(widened.Elements()[1], iso) {
		t.Errorf("the set of the other list took the mark as well: %v", widened)
	}
}

// TestConformance_CV021_ConversionGrowsWithTheResult holds a conversion whose
// element type grows at every level to work in proportion to its result:
// nested tuples around a tuple of objects, each level adding an object of an
// attribute of its own, converted to lists as deep, where every object is
// given every attribute above it. Four times the levels, the objects fixed,
// allocate under eight times as much, where building each level's members
// again at the next level's element type allocated some ten times as much.
func TestConformance_CV021_ConversionGrowsWithTheResult(t *testing.T) {
	conformance.Covers(t, "CV-021", "CV-044")
	const objects = 500
	var sizes [2]uint64
	for i, depth := range []int{10, 40} {
		v, c := growingUnions(depth, objects)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		r := tenon.Convert(v, c, tenon.Safe)
		runtime.ReadMemStats(&after)
		if r.IsError() {
			t.Fatalf("%d levels did not convert: %v", depth, r)
		}
		sizes[i] = after.TotalAlloc - before.TotalAlloc
	}
	if sizes[1] > 8*sizes[0] {
		t.Errorf("four times the levels allocated %d bytes, where the first allocated %d: more than eight times as much", sizes[1], sizes[0])
	}
}

// growingUnions returns tuples nested depth deep around a tuple of objects,
// each level holding the level below and a chain of one-member tuples ending
// in an object of an attribute of its own, and the lists nested as deep that
// it converts to.
func growingUnions(depth, objects int) (tenon.Value, tenon.Constraint) {
	members := make([]tenon.Value, objects)
	for i := range members {
		members[i] = tenon.Object(map[string]tenon.Value{"x": n(int64(i))})
	}
	v := tenon.Tuple(members...)
	for level := depth - 1; level >= 0; level-- {
		chain := tenon.Object(map[string]tenon.Value{fmt.Sprintf("a%03d", level): n(1)})
		for range depth - level {
			chain = tenon.Tuple(chain)
		}
		v = tenon.Tuple(v, chain)
	}
	return v, nestedLists(depth + 1)
}

// nestedLists returns depth list constraints around Any.
func nestedLists(depth int) tenon.Constraint {
	c := tenon.Any()
	for range depth {
		c = tenon.ListOf(c)
	}
	return c
}

// BenchmarkGrowingUnions measures converting growingUnions at two depths, the
// objects fixed, for make growth to read as a pair: the result is the objects
// times the depth, and the conversion allocates in proportion to it.
func BenchmarkGrowingUnions(b *testing.B) {
	for _, depth := range []int{20, 80} {
		v, c := growingUnions(depth, 2000)
		b.Run(fmt.Sprintf("depth/%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if got := tenon.Convert(v, c, tenon.Safe); got.IsError() {
					b.Fatalf("the value did not convert: %v", got)
				}
			}
		})
	}
}
