package ctytenon

import (
	"fmt"

	"github.com/kmoneil/tenon"
	"github.com/zclconf/go-cty/cty"
)

// A CapsulePair pairs a cty capsule type with a tenon capsule type, so that
// each crosses as the other, and a value of either as a value of the other
// holding the same pointer. PairCapsules makes one; the zero CapsulePair
// pairs nothing, and a Bridge holding one panics when it looks for a pair.
type CapsulePair struct {
	cty     cty.Type
	tenon   tenon.Type
	fromCty func(ptr any) (tenon.Value, string)
	toCty   func(v tenon.Value) any
}

// PairCapsules returns the pair of the cty capsule type c and the tenon
// capsule type t, whose values both hold a *E, for a Bridge's Capsules. It
// panics if c is not a capsule type whose values can hold a *E, or if t is
// nil.
func PairCapsules[E any](c cty.Type, t *tenon.CapsuleType[E]) CapsulePair {
	switch {
	case c == cty.NilType || !c.IsCapsuleType():
		usagePanic("PairCapsules called with %#v, which is not a cty capsule type", c)
	case t == nil:
		usagePanic("PairCapsules called with a nil *tenon.CapsuleType")
	case !holds[E](c):
		usagePanic("PairCapsules called with %#v, whose values cannot hold a %T", c, (*E)(nil))
	}
	return CapsulePair{
		cty:   c,
		tenon: t.Type(),
		fromCty: func(ptr any) (tenon.Value, string) {
			p, ok := ptr.(*E)
			switch {
			case !ok:
				return tenon.Value{}, fmt.Sprintf("the value holds a %T, which the tenon capsule type %s does not hold", ptr, t.Type())
			case p == nil:
				return tenon.Value{}, fmt.Sprintf("the value holds a nil %T, which no tenon capsule value holds", p)
			}
			return t.Value(p), ""
		},
		toCty: func(v tenon.Value) any {
			p, _ := t.Of(v)
			return p
		},
	}
}

// holds reports whether values of the cty capsule type c can hold a *E,
// asking cty, which panics on a pointer they cannot hold.
func holds[E any](c cty.Type) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	cty.CapsuleVal(c, new(E))
	return true
}

// pairOfCty returns the pair of the cty capsule type t, the first the Bridge
// holds, and whether there is one.
func (b Bridge) pairOfCty(t cty.Type) (CapsulePair, bool) {
	for _, p := range b.Capsules {
		if p.fromCty == nil {
			usagePanic("Bridge.Capsules holds the zero CapsulePair, which pairs nothing; PairCapsules makes one")
		}
		if p.cty.Equals(t) {
			return p, true
		}
	}
	return CapsulePair{}, false
}

// pairOfTenon returns the pair of the tenon capsule type t, the first the
// Bridge holds, and whether there is one.
func (b Bridge) pairOfTenon(t tenon.Type) (CapsulePair, bool) {
	for _, p := range b.Capsules {
		if p.fromCty == nil {
			usagePanic("Bridge.Capsules holds the zero CapsulePair, which pairs nothing; PairCapsules makes one")
		}
		if p.tenon == t {
			return p, true
		}
	}
	return CapsulePair{}, false
}

// capsuleFromCty returns the tenon value of v, a known value of the capsule
// type t, which the Bridge pairs, lying at p, adding to f why it does not
// cross where it does not: it holds a pointer of another type than the
// pair's, as a capsule type of an interface type can, or a nil pointer.
func (b Bridge) capsuleFromCty(v cty.Value, t cty.Type, p tenon.Path, f *failures) tenon.Value {
	pair, _ := b.pairOfCty(t)
	tv, why := pair.fromCty(v.EncapsulatedValue())
	if why != "" {
		f.add(p, CodeUnpairedCapsule, why)
	}
	return tv
}
