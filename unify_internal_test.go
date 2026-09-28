package tenon

import (
	"math/rand"
	"slices"
	"strconv"
	"testing"
)

// TestAllFoldsInCanonicalOrder holds the fold a rule makes of a list, as a
// tuple against a tuple of another length makes, to the canonical order
// CV-045 names, whatever order the rule gathered the list in. Two OneOfs of 65
// objects and a number fail as soon as the number meets the first OneOf, but
// folded as given, the OneOfs would form their pairs first and pass the bound.
func TestAllFoldsInCanonicalOrder(t *testing.T) {
	singles := func(prefix string) Constraint {
		members := make([]Constraint, 65)
		for i := range members {
			members[i] = ObjectWith(map[string]Field{prefix + strconv.Itoa(i): Required(Exactly(NumberType()))}, false)
		}
		return canonical(OneOf(members...))
	}
	a, b, n := singles("a"), singles("b"), Exactly(NumberType())
	for _, list := range [][]Constraint{{a, b, n}, {n, a, b}, {b, n, a}} {
		un := newUnifier(Safe)
		for _, c := range list {
			un.left = saturatingAdd(un.left, saturatingMul(unifyBound, un.size(c)))
		}
		if _, ok := un.all(list); ok || un.over {
			t.Errorf("folding %d constraints in the order given: ok %t, refused %t; want a failure that is no refusal", len(list), ok, un.over)
		}
	}
}

// TestFoldUnifiesObjectsAsPairsWould holds the fold's union of objects to
// folding them pair by pair, as CV-042 and CV-045 state the fold: the same
// result, the same failure, the same refusal, and the same weight of pairs
// formed, over lists of objects whose fields hold OneOfs, beside other
// constraints, with a bound drawn small enough to refuse some.
func TestFoldUnifiesObjectsAsPairsWould(t *testing.T) {
	r := rand.New(rand.NewSource(20260927))
	names := []string{"a", "b", "c", "d"}
	var leaf func(depth int) Constraint
	leaf = func(depth int) Constraint {
		switch n := r.Intn(8); {
		case n <= 1:
			return Exactly(NumberType())
		case n == 2:
			return Exactly(StringType())
		case n == 3 && depth > 0:
			return OneOf(leaf(depth-1), leaf(depth-1), leaf(depth-1))
		case n == 4 && depth > 0:
			return object(r, names, leaf, depth-1)
		case n == 5:
			return Exactly(ObjectType(map[string]Type{names[r.Intn(len(names))]: NumberType()}))
		case n == 6:
			return ListOf(Exactly(NumberType()))
		}
		return Any()
	}
	pairwise := func(un *unifier, u Constraint, cs []Constraint) (Constraint, bool) {
		for _, c := range cs {
			var ok bool
			if u, ok = un.pair(u, c); !ok {
				return Constraint{}, false
			}
		}
		return u, true
	}
	// The objects of a list are held at one position as a list chooses: as
	// they are, as the elements of collections, in a tuple beside another
	// object, or in a field; now and then one of them at another.
	// A list or a set, as r chooses for each: sorted by their first
	// position, the constraints of a list can meet a set before a list at
	// the next.
	collection := func(c Constraint) Constraint {
		if r.Intn(2) == 0 {
			return ListOf(c)
		}
		return SetOf(c)
	}
	wraps := []func(Constraint) Constraint{
		func(c Constraint) Constraint { return c },
		func(c Constraint) Constraint { return ListOf(c) },
		func(c Constraint) Constraint { return SetOf(c) },
		func(c Constraint) Constraint { return MapOf(c) },
		func(c Constraint) Constraint { return TupleOf(c, object(r, names, leaf, 1)) },
		func(c Constraint) Constraint { return ObjectWith(map[string]Field{"x": Required(c)}, r.Intn(2) == 0) },
		func(c Constraint) Constraint { return TupleOf(collection(c), collection(object(r, names, leaf, 1))) },
		func(c Constraint) Constraint {
			return ObjectWith(map[string]Field{"x": Required(collection(c)), "y": Optional(collection(object(r, names, leaf, 1)))}, r.Intn(2) == 0)
		},
	}
	folds, refused, unioned, joined := 0, 0, 0, 0
	for range 6000 {
		list := make([]Constraint, 2+r.Intn(10))
		held := wraps[r.Intn(len(wraps))]
		for i := range list {
			wrap := held
			if r.Intn(8) == 0 {
				wrap = wraps[r.Intn(len(wraps))]
			}
			switch r.Intn(20) {
			case 0:
				list[i] = leaf(1)
			case 1:
				list[i] = MapOf(Exactly(NumberType()))
			default:
				list[i] = wrap(object(r, names, leaf, 2))
			}
		}
		for i, c := range list {
			list[i] = canonical(c)
		}
		slices.SortFunc(list, compareConstraints)
		var total int64
		for _, c := range list {
			total = saturatingAdd(total, newUnifier(Safe).size(c))
		}
		left := saturatingMul(unifyBound, total)
		if r.Intn(2) == 0 {
			left = r.Int63n(total + 1)
		}
		for _, p := range []Policy{Safe, Unsafe} {
			a, b := newUnifier(p), newUnifier(p)
			a.left, b.left = left, left
			x, okx := a.fold(list[0], list[1:])
			y, oky := pairwise(b, list[0], list[1:])
			// Pairing a list with a set pairs their elements the other way
			// about, so the pairs within a OneOf are weighed in another
			// order; what is left after a refusal may differ, and nothing
			// shows it.
			if okx != oky || a.over != b.over || !a.over && a.left != b.left || okx && !x.equal(y) {
				t.Fatalf("under %s, folding %v: the union gives %v (%t, refused %t, %d left), pairs give %v (%t, refused %t, %d left)",
					p, list, x, okx, a.over, a.left, y, oky, b.over, b.left)
			}
			folds++
			if a.over {
				refused++
			}
			if meetPartByPart(list[0], list[1]) {
				unioned++
				if okx {
					joined++
				}
			}
		}
	}
	if refused < folds/50 || unioned < folds/2 || joined < folds/40 {
		t.Errorf("of %d folds, %d were refused, %d unified objects and %d of those succeeded: the lists exercise too little", folds, refused, unioned, joined)
	}
}

// object returns an ObjectWith of fields named from names, each constrained
// by leaf, some required, open or closed at random.
func object(r *rand.Rand, names []string, leaf func(int) Constraint, depth int) Constraint {
	fields := map[string]Field{}
	for _, name := range names {
		switch r.Intn(3) {
		case 0:
			fields[name] = Required(leaf(depth))
		case 1:
			fields[name] = Optional(leaf(depth))
		}
	}
	return ObjectWith(fields, r.Intn(2) == 0)
}

// meetPartByPart reports whether a fold holds a and b as the union of their
// parts: collections that meet, tuples of one length, or objects.
func meetPartByPart(a, b Constraint) bool {
	x, y := spelledOut(a), spelledOut(b)
	switch {
	case isCollectionOf(x.c.kind):
		return collectionsMeet(x.c.kind, y.c.kind)
	case x.c.kind == ConstraintTupleOf:
		return y.c.kind == ConstraintTupleOf && len(x.c.members) == len(y.c.members)
	}
	return objectLike(a) && objectLike(b)
}
