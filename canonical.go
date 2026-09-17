package tenon

import (
	"cmp"
	"slices"
	"strings"
	"sync"

	"github.com/kmoneil/tenon/internal/decimal"
)

// CanonicalCompare orders two known values, returning a negative number, zero
// or a positive number as a sorts before, together with, or after b. It is a
// total order over every known value that is not marked: it returns zero
// exactly for values that Identical reports the same.
//
// Null sorts before every other value. Values of different kinds sort in the
// order Bool, Number, String, List, Set, Map, Tuple, Object, Capsule. Within a
// kind, false sorts before true, numbers numerically, strings by the scalar
// values of the form they hold, sequences by their members, maps and objects by
// name before value, and a capsule by the order its type declares, if it
// declares one. Values that those rules leave together, which happens only
// between two types, sort by type.
//
// This is the host's order, not the language's: it is what sorts the members of
// a set and what makes an encoding deterministic. Whether a language built on
// tenon offers it to its users is that language's decision, and LessThan is the
// operation its users would otherwise reach for.
//
// CanonicalCompare panics on a value that is not known, and on a marked value,
// one that carries a mark or holds one at any depth. The order is by what
// values hold, so it would sort a marked value together with its unmarked twin,
// which Identical tells apart: compare the values UnmarkDeep returns.
func CanonicalCompare(a, b Value) int {
	return compareCanonical(canonicalOperand(a), canonicalOperand(b))
}

// canonicalOperand returns the description of a value the order is defined for.
func canonicalOperand(v Value) *node {
	n := v.data()
	if !n.isKnown() {
		usagePanic("CanonicalCompare called on %s, which is not a known value", n.describe())
	}
	if n.isMarked() {
		usagePanic("CanonicalCompare called on %s, and the canonical order is not defined for a marked value; compare the values UnmarkDeep returns",
			n.describeMarked())
	}
	return n
}

// compareCanonical orders two known values or two members of them.
func compareCanonical(a, b *node) int {
	switch {
	case a.state == stateNull && b.state == stateNull:
		// Two nulls of different types are two values, and sort by type.
		return compareTypes(a.typ, b.typ)
	case a.state == stateNull:
		return -1
	case b.state == stateNull:
		return 1
	}
	if c := cmp.Compare(a.typ.t.kind, b.typ.t.kind); c != 0 {
		// The kinds are declared in this order, so comparing them is it.
		return c
	}
	if c := compareContent(a, b); c != 0 {
		return c
	}
	// The content says they are together, so either they are one value or
	// their types are what differ.
	return compareTypes(a.typ, b.typ)
}

// compareValues orders two members of a known value, which are known too.
func compareValues(a, b Value) int { return compareCanonical(a.n, b.n) }

// compareContent orders two values of one kind by what they hold.
func compareContent(a, b *node) int {
	switch a.typ.t.kind {
	case KindBool:
		return cmp.Compare(boolOrder(a.data.(bool)), boolOrder(b.data.(bool)))
	case KindNumber:
		return a.data.(decimal.Dec).Cmp(b.data.(decimal.Dec))
	case KindString:
		return strings.Compare(a.data.(string), b.data.(string))
	case KindList, KindTuple:
		return slices.CompareFunc(a.data.([]Value), b.data.([]Value), compareValues)
	case KindSet:
		// A set holds its members in this very order, and holds each of them
		// once, so there is nothing to do here but walk them.
		return slices.CompareFunc(a.data.([]Value), b.data.([]Value), compareValues)
	case KindMap:
		return slices.CompareFunc(a.data.([]mapEntry), b.data.([]mapEntry), func(x, y mapEntry) int {
			if c := strings.Compare(x.key, y.key); c != 0 {
				return c
			}
			return compareValues(x.val, y.val)
		})
	case KindObject:
		return compareAttributes(a, b)
	}
	if a.typ != b.typ {
		// Two capsule types declare their own operations, and neither type's
		// have anything to say about the other type's values. What tells these
		// apart is the types, which is what the caller falls back to.
		return 0
	}
	return a.typ.t.capsule.order(a.data, b.data)
}

// boolOrder gives false its place before true.
func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

// compareAttributes orders two objects by attribute, name before value, which
// the type holds in name order already.
func compareAttributes(a, b *node) int {
	x, y := a.data.([]Value), b.data.([]Value)
	xa, ya := a.typ.t.attrs, b.typ.t.attrs
	for i := 0; i < len(xa) && i < len(ya); i++ {
		if c := strings.Compare(xa[i].name, ya[i].name); c != 0 {
			return c
		}
		if c := compareValues(x[i], y[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(xa), len(ya))
}

// compareTypes orders two types the way the values of one kind are ordered,
// which is what decides between values that are otherwise together.
func compareTypes(a, b Type) int {
	if a == b {
		return 0
	}
	if c := cmp.Compare(a.t.kind, b.t.kind); c != 0 {
		return c
	}
	switch a.t.kind {
	case KindList, KindSet, KindMap:
		return compareTypes(a.t.elem, b.t.elem)
	case KindTuple:
		return slices.CompareFunc(a.t.elems, b.t.elems, compareTypes)
	case KindObject:
		return slices.CompareFunc(a.t.attrs, b.t.attrs, func(x, y attribute) int {
			if c := strings.Compare(x.name, y.name); c != 0 {
				return c
			}
			return compareTypes(x.typ, y.typ)
		})
	case KindCapsule:
		if c := strings.Compare(a.t.capsule.name, b.t.capsule.name); c != 0 {
			return c
		}
		// Two capsule types of one name are told apart by which was made
		// first, which holds for the rest of the run and no longer.
		return cmp.Compare(a.t.id, b.t.id)
	}
	// Bool, Number and String have one type each, which a == b caught.
	return 0
}

// capsuleOrder numbers the capsule values that are compared without their type
// declaring an order. A number is kept for the rest of the run, so any two
// values sort the same way however often they are compared, and a run that
// sorts the same values again gets the same answer.
//
// A number belongs to an equality class and not to a pointer. Values the type
// reports equal are one value, so an order that gave them two numbers could
// sort a third value between them, and [EQ-045] would not hold. A type that
// declares no equality has none to ask, and every pointer is a class of its
// own.
//
// The numbers are held until the process ends. A capsule type whose values are
// sorted often, or held in sets, should declare Compare and avoid this.
var capsuleOrder struct {
	mu sync.Mutex
	// byPointer numbers the values of types that declare no equality.
	byPointer map[any]uint64
	// byClass numbers the equality classes of types that do, one list per type
	// and declared hash.
	byClass map[capsuleClassKey][]capsuleClass
	next    uint64
}

// capsuleClassKey is a capsule type together with one of its declared hashes.
// Values whose hashes differ are told apart by the hash before the numbering
// is reached, so only a collision brings two of them into one list.
type capsuleClassKey struct {
	d    *capsuleData
	hash uint64
}

// capsuleClass is one equality class: a value standing for it, and the number
// the class was given.
type capsuleClass struct {
	rep any
	n   uint64
}

// order orders two values of one capsule type: by the declared order where
// there is one, and otherwise by anything the type has said that tells them
// apart.
//
// Values the type reports equal sort together, which they must: they are one
// value, and an order that split them would not be an order over values. What
// is left is told apart by the hash, which a type declares whenever it declares
// equality, and by the numbers this run has given to values whose hashes
// collide.
func (d *capsuleData) order(a, b any) int {
	if d.compare != nil {
		return cmp.Compare(d.compare(a, b), 0)
	}
	if d.equal(a, b) {
		return 0
	}
	if d.hash != nil {
		if c := cmp.Compare(d.hash(a), d.hash(b)); c != 0 {
			return c
		}
	}
	return cmp.Compare(d.number(a), d.number(b))
}

// number returns the number this run has given the equality class that v
// belongs to under d, giving the class one if this is the first time it has
// been asked for.
//
// The type's equality is asked outside the lock. It is the caller's code and
// may do anything, comparing capsule values among it, which under the lock
// would deadlock. The insertion then reads the list again and compares against
// whatever arrived while the lock was down, so one class is never given two
// numbers.
func (d *capsuleData) number(v any) uint64 {
	if d.equals == nil {
		// Equality is the pointer, so every pointer is a class of its own.
		capsuleOrder.mu.Lock()
		defer capsuleOrder.mu.Unlock()
		if n, ok := capsuleOrder.byPointer[v]; ok {
			return n
		}
		if capsuleOrder.byPointer == nil {
			capsuleOrder.byPointer = map[any]uint64{}
		}
		capsuleOrder.next++
		capsuleOrder.byPointer[v] = capsuleOrder.next
		return capsuleOrder.next
	}
	// A type declaring equality declares a hash too, which Capsule requires,
	// so the list holds just the values whose hashes collide with v's.
	key := capsuleClassKey{d, d.hash(v)}
	scanned := 0
	for {
		capsuleOrder.mu.Lock()
		list := capsuleOrder.byClass[key]
		capsuleOrder.mu.Unlock()
		for _, c := range list[scanned:] {
			if d.equals(c.rep, v) {
				return c.n
			}
		}
		scanned = len(list)
		capsuleOrder.mu.Lock()
		if len(capsuleOrder.byClass[key]) == scanned {
			if capsuleOrder.byClass == nil {
				capsuleOrder.byClass = map[capsuleClassKey][]capsuleClass{}
			}
			capsuleOrder.next++
			n := capsuleOrder.next
			capsuleOrder.byClass[key] = append(capsuleOrder.byClass[key], capsuleClass{v, n})
			capsuleOrder.mu.Unlock()
			return n
		}
		capsuleOrder.mu.Unlock()
		// Classes arrived while the lock was down; ask about those too.
	}
}
