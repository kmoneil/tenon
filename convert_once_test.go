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

// TestConformance_CV033_ASetIsMadeAtItsOwnTypeAndThenWidened holds a set that
// a conversion makes to what it is made at its own element type, whether or
// not a level above widens that type: the marks it gathers from the values
// within its members, an Isolate one on a value that the wider type rebuilds
// among them (CV-033); the redacting marks it gathers, which a level above
// whose element type takes attribute names from it carries, whether the set's
// own type is settled or left open (CV-021); and the members it keeps, none of
// them one that is not known and could only be a value it holds (EQ-041).
func TestConformance_CV033_ASetIsMadeAtItsOwnTypeAndThenWidened(t *testing.T) {
	conformance.Covers(t, "CV-033", "EQ-041", "CV-021")
	iso := stamp{id: "iso", policy: tenon.Isolate}
	secret := stamp{id: "secret", redact: true}
	sets := tenon.ListOf(tenon.SetOf(tenon.Any()))

	// An Isolate mark on an empty tuple within a member, which the set's own
	// element type, list(tuple([])), keeps as it is, and a sibling's widening
	// it to list(list(string)) would rebuild.
	marked := tenon.Tuple(tenon.Tuple(tenon.WithMarks(tenon.Tuple(), iso)), tenon.Tuple())
	widening := tenon.Tuple(tenon.Tuple(tenon.Tuple(s("x"))))
	alone := tenon.Convert(tenon.Tuple(marked), sets, uns)
	widened := tenon.Convert(tenon.Tuple(widening, marked), sets, uns)
	if alone.IsError() || !tenon.HasMark(alone.Elements()[0], iso) {
		t.Errorf("alone: %v, want the set carrying %v", alone, iso)
	}
	if widened.IsError() || !tenon.HasMark(widened.Elements()[1], iso) {
		t.Errorf("widened: %v, want the second set carrying %v", widened, iso)
	}

	// A set that holds an unknown empty tuple beside the empty tuple and null
	// holds every value its element type has, so the unknown member is no
	// member, and the set is known, widened or not.
	unknownBeside := tenon.Tuple(tenon.Unknown(tenon.TupleType()), tenon.Tuple(), tenon.Null(tenon.TupleType()))
	nums := tenon.ListType(num)
	wantValue(t, "a set holding every empty tuple, alone", tenon.Convert(tenon.Tuple(unknownBeside), sets, uns).Elements()[0],
		tenon.Set(tenon.TupleType(), tenon.Tuple(), tenon.Null(tenon.TupleType())))
	wantValue(t, "a set holding every empty tuple, widened", tenon.Convert(tenon.Tuple(unknownBeside, tenon.Tuple(tenon.Tuple(n(1)))), sets, uns).Elements()[0],
		tenon.Set(nums, tenon.List(num), tenon.Null(nums)))

	// A redacted string within a member's object: the set gathers its mark,
	// and the list whose element type names the set's attributes carries it.
	object := tenon.Object(map[string]tenon.Value{"a0": tenon.Tuple(tenon.WithMarks(s("x"), secret), tenon.Tuple())})
	r := tenon.Convert(tenon.Tuple(tenon.Tuple(object)), tenon.ListOf(tenon.SetOf(tenon.ObjectWith(nil, false))), uns)
	if r.IsError() || !tenon.HasMark(r, secret) {
		t.Errorf("a redacted value within a set's member: %v, want the list carrying %v", r, secret)
	}
	// The same where the set's element type is left open until a sibling
	// settles it.
	field := tenon.ObjectWith(map[string]tenon.Field{"a": tenon.Required(tenon.ListOf(tenon.Any())), "b": tenon.Required(tenon.Any())}, true)
	open := tenon.Tuple(tenon.Object(map[string]tenon.Value{"a": tenon.Tuple(), "b": tenon.WithMarks(s("y"), secret)}))
	settling := tenon.Tuple(tenon.Object(map[string]tenon.Value{"a": tenon.Tuple(s("x")), "b": s("z")}))
	r = tenon.Convert(tenon.Tuple(open, settling), tenon.ListOf(tenon.SetOf(field)), uns)
	if r.IsError() || !tenon.HasMark(r, secret) {
		t.Errorf("a redacted value within a member of a set whose type is left open: %v, want the list carrying %v", r, secret)
	}
}

// TestConformance_CV021_SetsGrowWithTheResult holds a conversion to sets
// nested as deep as the value, each level widening the element type of the
// one below, to work in proportion to its result, as lists do
// (TestConformance_CV021_ConversionGrowsWithTheResult): four times the levels
// allocate under eight times as much, with no mark and with an Isolate mark
// on a value within a member of the innermost set, which that set is made at
// its own type for and widened. Made at its own type and widened at every
// level, each set was made again for every set holding it.
func TestConformance_CV021_SetsGrowWithTheResult(t *testing.T) {
	conformance.Covers(t, "CV-021", "CV-033")
	iso := stamp{id: "iso", policy: tenon.Isolate}
	for _, mark := range []bool{false, true} {
		var sizes [2]uint64
		for i, depth := range []int{10, 40} {
			v, _ := growingUnions(depth, 500)
			if mark {
				v = markInnermost(v, iso)
			}
			c := tenon.Any()
			for range depth + 1 {
				c = tenon.SetOf(c)
			}
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			r := tenon.Convert(v, c, tenon.Unsafe)
			runtime.ReadMemStats(&after)
			if r.IsError() {
				t.Fatalf("%d levels did not convert: %.200v", depth, r)
			}
			sizes[i] = after.TotalAlloc - before.TotalAlloc
		}
		if sizes[1] > 8*sizes[0] {
			t.Errorf("an Isolate mark %v: four times the levels allocated %d bytes, where the first allocated %d: more than eight times as much", mark, sizes[1], sizes[0])
		}
	}
}

// markInnermost returns v, as growingUnions makes it, with the attribute of
// the first object of the innermost tuple carrying m.
func markInnermost(v tenon.Value, m tenon.Mark) tenon.Value {
	if inner := v.Index(0); inner.Type().Kind() == tenon.KindTuple {
		elems := v.Elements()
		elems[0] = markInnermost(inner, m)
		return tenon.Tuple(elems...)
	}
	elems := v.Elements()
	elems[0] = tenon.Object(map[string]tenon.Value{"x": tenon.WithMarks(elems[0].Attribute("x"), m)})
	return tenon.Tuple(elems...)
}
