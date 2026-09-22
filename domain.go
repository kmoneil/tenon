package tenon

import "math"

// The domain of a type is the values it holds, null aside. Most types hold
// more than can be counted, but a few hold a handful, and a set over one of
// those has as many members as its element type has values, null among them,
// and no more. That bounds the lengths such a set can have, and a set required
// to hold every one of them is that set (UN-005).

// maxDomainSet is the greatest number of members the set of every value of an
// element type may have for that set to be built. Counting goes as far as a
// length can ask about; building stops here, because the domain of a type can
// be far larger than the text of the type.
const maxDomainSet = 256

// setCeiling returns the greatest number of members a value of set type t can
// have, where its element type bounds that number. It reports nothing for
// every other type, and for an element type holding more values than a length
// can ask about.
func setCeiling(t Type) lengthBound {
	if t.t.kind != KindSet {
		return lengthBound{}
	}
	if n, ok := memberCount(t.t.elem); ok {
		return lengthBound{n: n, set: true}
	}
	return lengthBound{}
}

// memberCount returns how many values a member of type t can be: the values of
// its domain and null.
func memberCount(t Type) (int64, bool) {
	c, ok := domainCount(t)
	if !ok || c == math.MaxInt64 {
		return 0, false
	}
	return c + 1, true
}

// domainCount returns how many values the domain of t holds, and whether that
// number is finite and small enough to count. Number, String and Capsule hold
// more values than can be counted, and so do List and Map, whose lengths are
// unbounded; a type holding one of those holds as many. The count is kept on
// the type, since equality asks it of every set it compares.
func domainCount(t Type) (int64, bool) {
	d := t.t
	if kept := d.domain.Load(); kept != 0 {
		return kept, kept > 0
	}
	n, ok := countDomain(d)
	if !ok {
		n = -1
	}
	d.domain.Store(n)
	return n, ok
}

// countDomain counts the values of d, as domainCount describes them.
func countDomain(d *typeData) (int64, bool) {
	switch d.kind {
	case KindBool:
		return 2, true
	case KindTuple:
		n := int64(1)
		for _, e := range d.elems {
			c, ok := memberCount(e)
			if !ok {
				return 0, false
			}
			if n, ok = mulCount(n, c); !ok {
				return 0, false
			}
		}
		return n, true
	case KindObject:
		n := int64(1)
		for _, a := range d.attrs {
			c, ok := memberCount(a.typ)
			if !ok {
				return 0, false
			}
			if n, ok = mulCount(n, c); !ok {
				return 0, false
			}
		}
		return n, true
	case KindSet:
		// A set of its element type is any of the sets of its members, which
		// are the values of that type and null.
		c, ok := memberCount(d.elem)
		if !ok || c >= 62 {
			return 0, false
		}
		return 1 << uint(c), true
	}
	return 0, false
}

// mulCount multiplies two counts, reporting false where the product is more
// than can be counted.
func mulCount(a, b int64) (int64, bool) {
	if a > math.MaxInt64/b {
		return 0, false
	}
	return a * b, true
}

// fullSet returns the set of type t holding every value its element type has,
// null among them, which is the only set of that type with as many members as
// setCeiling allows. Its element type must hold few enough values to build
// (maxDomainSet).
func fullSet(t Type) Value {
	elem := t.t.elem
	return SetVal(elem, memberValues(elem)...)
}

// memberValues returns every value a member of type t can be: the values of
// its domain and null. The order is not the order a set holds them in, which
// SetVal settles.
//
// The values are built once for the type and kept on it: every set over a
// type holding few values asks for them as soon as it holds a member that is
// not known, and building them again means building up to 256 values, each of
// which builds the values within it. They are immutable, as every value is,
// and the caller reads the slice rather than writing to it.
func memberValues(t Type) []Value {
	if kept := t.t.values.Load(); kept != nil {
		return *kept
	}
	values := append(domainValues(t), NullVal(t))
	t.t.values.CompareAndSwap(nil, &values)
	return *t.t.values.Load()
}

// domainValues returns the values of the domain of t, which must be finite
// (domainCount).
func domainValues(t Type) []Value {
	switch d := t.t; d.kind {
	case KindBool:
		return []Value{Bool(false), Bool(true)}
	case KindTuple:
		var out []Value
		for _, row := range memberRows(d.elems) {
			out = append(out, TupleVal(row...))
		}
		return out
	case KindObject:
		types := make([]Type, len(d.attrs))
		for i, a := range d.attrs {
			types[i] = a.typ
		}
		var out []Value
		for _, row := range memberRows(types) {
			attrs := make(map[string]Value, len(row))
			for i, v := range row {
				attrs[d.attrs[i].name] = v
			}
			out = append(out, ObjectVal(attrs))
		}
		return out
	case KindSet:
		// Every set of members drawn from the values of the element type.
		vs := memberValues(d.elem)
		out := make([]Value, 0, 1<<uint(len(vs)))
		for mask := 0; mask < 1<<uint(len(vs)); mask++ {
			var members []Value
			for i, v := range vs {
				if mask&(1<<uint(i)) != 0 {
					members = append(members, v)
				}
			}
			out = append(out, SetVal(d.elem, members...))
		}
		return out
	}
	internalPanic("the values of %s were asked for, and it holds more than can be counted", t)
	return nil
}

// memberRows returns every way of choosing one value for each of these member
// types, each of which may be null.
func memberRows(types []Type) [][]Value {
	rows := [][]Value{nil}
	for _, t := range types {
		vs := memberValues(t)
		next := make([][]Value, 0, len(rows)*len(vs))
		for _, row := range rows {
			for _, v := range vs {
				next = append(next, append(append(make([]Value, 0, len(row)+1), row...), v))
			}
		}
		rows = next
	}
	return rows
}
