package ctytenon

import (
	"errors"
	"math"
	"math/big"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/gotenon"
	"github.com/zclconf/go-cty/cty"
)

// narrowingsFromCty returns the narrowings that say of a tenon value what the
// refinements of v, an unknown cty value of type t, say of it: that it is not
// null; a number's bounds; a string's prefix; and a collection's length, which
// a pending list, set or map records as well.
func narrowingsFromCty(v cty.Value, t cty.Type) []tenon.Narrowing {
	r := v.Range()
	var ns []tenon.Narrowing
	if r.DefinitelyNotNull() {
		ns = append(ns, tenon.NotNull())
	}
	switch {
	case t == cty.Number:
		if min, inclusive := r.NumberLowerBound(); min.IsKnown() {
			if b, ok := boundFromCty(min.AsBigFloat(), true); ok {
				ns = append(ns, tenon.NumberMin(b, inclusive))
			}
		}
		if max, inclusive := r.NumberUpperBound(); max.IsKnown() {
			if b, ok := boundFromCty(max.AsBigFloat(), false); ok {
				ns = append(ns, tenon.NumberMax(b, inclusive))
			}
		}
	case t == cty.String:
		if p := r.StringPrefix(); p != "" {
			ns = append(ns, prefixFromCty(p))
		}
	case t.IsCollectionType():
		if min := r.LengthLowerBound(); min > 0 {
			ns = append(ns, tenon.LengthMin(int64(min)))
		}
		if max := r.LengthUpperBound(); max < math.MaxInt {
			ns = append(ns, tenon.LengthMax(int64(max)))
		}
	}
	return ns
}

// boundFromCty returns the tenon number that bounds, from below where lower
// is true and from above otherwise, every number crossing from cty that f
// bounds so: f as a number crosses, where that lies on f's side of every
// number past f, as it does wherever it is f exactly, and f's exact value
// otherwise. A number past f held in 512 bits crosses as a decimal past the
// one f crosses as, but one held in more crosses as itself, which can lie
// between f and its decimal. It reports false where neither crosses, as an
// infinity does, and the bound is then none.
func boundFromCty(f *big.Float, lower bool) (tenon.Value, bool) {
	n := numberFromCty(f)
	if n.IsError() {
		return tenon.Value{}, false
	}
	exact, _ := f.Rat(nil)
	if c := n.AsBigRat().Cmp(exact); c == 0 || lower == (c < 0) {
		return n, true
	}
	v, err := gotenon.Encode(f)
	if e := (*tenon.Error)(nil); errors.As(err, &e) {
		return tenon.Value{}, false
	}
	return v, true
}

// prefixFromCty returns the narrowing to strings beginning with p, a prefix
// cty keeps of an unknown string. cty has already cut from it what text
// following it could change, and tenon's StringPrefix would cut again, so a
// prefix it would cut is given an x after it, which StringPrefix cuts instead,
// as Value.GoString writes one.
func prefixFromCty(p string) tenon.Narrowing {
	if kept := tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix(p)).Range().StringPrefix(); kept == p {
		return tenon.StringPrefix(p)
	}
	return tenon.StringPrefix(p + "x")
}

// refineToCty returns u, an unknown cty value of type t, refined by what r,
// the range of a tenon value, says that cty can: that it is not null; a
// number's bounds; a string's prefix; and a collection's length. Where r says
// none of them, u is unrefined.
//
// A bound crosses as a number does, and every number past it crosses as one
// at it or past it, since cty's parser reads a larger decimal as no smaller
// a number. A number just past the bound can cross as the bound itself,
// so a bound that excludes itself includes itself in cty. What cty cannot
// say is left behind: a string's length, and a set's listed members.
func refineToCty(u cty.Value, t cty.Type, r tenon.Range) cty.Value {
	var refine []func(*cty.RefinementBuilder) *cty.RefinementBuilder
	if !r.AllowsNull() {
		refine = append(refine, (*cty.RefinementBuilder).NotNull)
	}
	switch {
	case t == cty.Number:
		// A bound carries the marks of the value it narrows, which cross
		// on the value.
		if min, _, ok := r.NumberMin(); ok {
			min, _ = tenon.UnmarkDeep(min)
			refine = append(refine, func(b *cty.RefinementBuilder) *cty.RefinementBuilder {
				return b.NumberRangeLowerBound(numberToCty(min), true)
			})
		}
		if max, _, ok := r.NumberMax(); ok {
			max, _ = tenon.UnmarkDeep(max)
			refine = append(refine, func(b *cty.RefinementBuilder) *cty.RefinementBuilder {
				return b.NumberRangeUpperBound(numberToCty(max), true)
			})
		}
	case t == cty.String:
		if p := r.StringPrefix(); p != "" {
			refine = append(refine, func(b *cty.RefinementBuilder) *cty.RefinementBuilder { return b.StringPrefixFull(p) })
		}
	case t.IsCollectionType():
		if min := r.LengthMin(); min > 0 {
			refine = append(refine, func(b *cty.RefinementBuilder) *cty.RefinementBuilder {
				return b.CollectionLengthLowerBound(int(min))
			})
		}
		if max, ok := r.LengthMax(); ok {
			refine = append(refine, func(b *cty.RefinementBuilder) *cty.RefinementBuilder {
				return b.CollectionLengthUpperBound(int(max))
			})
		}
	}
	// A value refined by nothing is not cty's unknown value of its type.
	if len(refine) == 0 {
		return u
	}
	return u.RefineWith(refine...)
}

// notNull reports whether v, a pending value, is known not to be null, which
// it says without a range.
func notNull(v tenon.Value) bool {
	n := tenon.IsNull(v)
	return n.IsKnown() && !n.AsBool()
}

// pendingLengthsToCty returns u, cty's unknown collection of type t for the
// pending value v, refined by the lengths v records, which Length of it
// bounds, and by v's not being null where it is known not to be.
func pendingLengthsToCty(u cty.Value, t cty.Type, v tenon.Value) cty.Value {
	b, refined := u.Refine(), false
	if notNull(v) {
		b, refined = b.NotNull(), true
	}
	if l := tenon.Length(v); !l.IsError() {
		r := l.Range()
		if min, _, ok := r.NumberMin(); ok {
			if n, ok := min.AsInt64(); ok && n > 0 {
				b, refined = b.CollectionLengthLowerBound(int(n)), true
			}
		}
		if max, _, ok := r.NumberMax(); ok {
			if n, ok := max.AsInt64(); ok {
				b, refined = b.CollectionLengthUpperBound(int(n)), true
			}
		}
	}
	// A value refined by nothing is not cty's unknown value of its type.
	if !refined {
		return u
	}
	return b.NewValue()
}
