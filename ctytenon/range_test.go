package ctytenon_test

import (
	"math/big"
	"math/rand"
	"strconv"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/zclconf/go-cty/cty"
)

// TestRanges holds each refinement cty and tenon can both say to crossing as
// itself, both ways.
func TestRanges(t *testing.T) {
	var b ctytenon.Bridge
	for _, c := range []struct {
		name  string
		cty   cty.Value
		tenon tenon.Value
	}{
		{"not null", cty.UnknownVal(cty.String).RefineNotNull(), tenon.Narrow(tenon.Unknown(str), tenon.NotNull())},
		{"a bool not null", cty.UnknownVal(cty.Bool).RefineNotNull(), tenon.Narrow(tenon.Unknown(boo), tenon.NotNull())},
		{
			"number bounds",
			cty.UnknownVal(cty.Number).Refine().NumberRangeInclusive(cty.NumberIntVal(5), cty.NumberIntVal(10)).NewValue(),
			tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), true), tenon.NumberMax(n(10), true)),
		},
		{"a prefix kept whole", cty.UnknownVal(cty.String).Refine().StringPrefixFull("web-").NewValue(), tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("web-"))},
		// cty cuts the e, which a combining acute would change, and tenon
		// would cut the f as well, which a combining dot above would.
		{"a prefix cty has cut", cty.UnknownVal(cty.String).Refine().StringPrefix("cafe").NewValue(), tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("caf"+"x"))},
		{
			"length bounds",
			cty.UnknownVal(cty.List(cty.String)).Refine().CollectionLengthLowerBound(1).CollectionLengthUpperBound(3).NewValue(),
			tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMin(1), tenon.LengthMax(3)),
		},
		{"a pending value not null", cty.UnknownVal(cty.List(cty.DynamicPseudoType)).RefineNotNull(), tenon.Narrow(tenon.Pending(tenon.ListOf(tenon.Any())), tenon.NotNull())},
		{
			"refinements within a container",
			cty.ListVal([]cty.Value{cty.UnknownVal(cty.Number).Refine().NotNull().NumberRangeLowerBound(cty.Zero, true).NewValue()}),
			tenon.List(num, tenon.Narrow(tenon.Unknown(num), tenon.NotNull(), tenon.NumberMin(n(0), true))),
		},
	} {
		got, err := b.FromCty(c.cty)
		if err != nil || !got.Equal(c.tenon) {
			t.Errorf("%s: FromCty(%#v) = %v, %v; want %v", c.name, c.cty, got, err, c.tenon)
		}
		back, err := b.ToCty(c.tenon)
		if err != nil || !back.RawEquals(c.cty) {
			t.Errorf("%s: ToCty(%v) = %#v, %v; want %#v", c.name, c.tenon, back, err, c.cty)
		}
	}
}

// TestRangesThatWiden holds the ranges that cross as one allowing more to
// doing so: a bound that excludes itself, which includes itself in cty, and
// what the other side has no refinement for.
func TestRangesThatWiden(t *testing.T) {
	var b ctytenon.Bridge
	for _, c := range []struct {
		name  string
		tenon tenon.Value
		cty   cty.Value
	}{
		{
			"a bound excluding itself",
			tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), false)),
			cty.UnknownVal(cty.Number).Refine().NumberRangeLowerBound(cty.NumberIntVal(5), true).NewValue(),
		},
		{"a string's length", tenon.Narrow(tenon.Unknown(str), tenon.LengthMin(3)), cty.UnknownVal(cty.String)},
	} {
		if got, err := b.ToCty(c.tenon); err != nil || !got.RawEquals(c.cty) {
			t.Errorf("%s: ToCty(%v) = %#v, %v; want %#v", c.name, c.tenon, got, err, c.cty)
		}
	}
	for _, c := range []struct {
		name  string
		cty   cty.Value
		tenon tenon.Value
	}{
		{
			"a bound excluding itself",
			cty.UnknownVal(cty.Number).Refine().NumberRangeLowerBound(cty.NumberIntVal(5), false).NewValue(),
			tenon.Narrow(tenon.Unknown(num), tenon.NumberMin(n(5), false)),
		},
	} {
		if got, err := b.FromCty(c.cty); err != nil || !got.Equal(c.tenon) {
			t.Errorf("%s: FromCty(%#v) = %v, %v; want %v", c.name, c.cty, got, err, c.tenon)
		}
	}
}

// TestRangesAllowWhatTheirSourceAllows crosses random refined unknown values
// each way, and holds the crossing to allowing every value its source allows:
// for random values inside the source's range, as the source itself says,
// the crossed range does not exclude their crossing, as the other side says,
// cty by ValueRange.Includes and tenon by Equals. A bound HCL's parser could
// write, and one just past it, are among them.
func TestRangesAllowWhatTheirSourceAllows(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261007))
	for range conformance.Iterations(t, 1000) {
		u, inside := randomRefinedCty(r)
		tu, err := b.FromCty(u)
		if err != nil {
			t.Fatalf("FromCty(%#v): %v", u, err)
		}
		for _, w := range inside {
			if in := u.Range().Includes(w); in.IsKnown() && in.False() {
				t.Fatalf("the range of %#v excludes %#v, which was made inside it", u, w)
			}
			tw, err := b.FromCty(w)
			if err != nil {
				t.Fatalf("FromCty(%#v): %v", w, err)
			}
			if eq := tenon.Equals(tu, tw); eq.IsKnown() && !eq.AsBool() {
				t.Fatalf("%#v crossed as %v, which excludes %v, the crossing of %#v", u, tu, tw, w)
			}
		}

		v, inside2 := randomNarrowedTenon(r)
		cv, err := b.ToCty(v)
		if err != nil {
			t.Fatalf("ToCty(%v): %v", v, err)
		}
		for _, x := range inside2 {
			if eq := tenon.Equals(v, x); eq.IsKnown() && !eq.AsBool() {
				t.Fatalf("%v excludes %v, which was made inside it", v, x)
			}
			cx, err := b.ToCty(x)
			if err != nil {
				t.Fatalf("ToCty(%v): %v", x, err)
			}
			if !cv.IsKnown() {
				if in := cv.Range().Includes(cx); in.IsKnown() && in.False() {
					t.Fatalf("%v crossed as %#v, which excludes %#v, the crossing of %v", v, cv, cx, x)
				}
			}
		}
	}
}

// randomBound returns a random number HCL's parser could make, and a number
// just past it upward and downward: one bit at 512 bits away, or, held in
// 1,024 bits, far closer than any number of 512.
func randomBound(r *rand.Rand) (cty.Value, cty.Value, cty.Value) {
	text := digits(r, 1+r.Intn(20)) + "e" + strconv.Itoa(r.Intn(41)-20)
	if r.Intn(2) == 0 {
		text = "-" + text
	}
	f := cty.MustParseNumberVal(text).AsBigFloat()
	prec := uint(512)
	if r.Intn(2) == 0 {
		prec = 1024
	}
	step := new(big.Float).SetMantExp(new(big.Float).SetInt64(1), f.MantExp(nil)-int(prec))
	up := new(big.Float).SetPrec(prec).Add(f, step)
	down := new(big.Float).SetPrec(prec).Sub(f, step)
	return cty.NumberVal(f), cty.NumberVal(up), cty.NumberVal(down)
}

// randomRefinedCty returns a random unknown cty value refined at random, and
// values inside its range.
func randomRefinedCty(r *rand.Rand) (cty.Value, []cty.Value) {
	var inside []cty.Value
	var u cty.Value
	notNull := r.Intn(2) == 0
	switch r.Intn(4) {
	case 0:
		lo, loUp, loDown := randomBound(r)
		hi, hiUp, hiDown := randomBound(r)
		if lo.GreaterThan(hi).True() {
			lo, loUp, hi, hiDown = hi, hiUp, lo, loDown
		}
		loInc, hiInc := r.Intn(2) == 0, r.Intn(2) == 0
		if lo.Equals(hi).True() {
			loInc, hiInc = true, true
		}
		bld := cty.UnknownVal(cty.Number).Refine()
		if r.Intn(4) != 0 {
			bld = bld.NumberRangeLowerBound(lo, loInc)
			if loInc {
				inside = append(inside, lo)
			}
			if loUp.LessThan(hi).True() {
				inside = append(inside, loUp)
			}
		}
		if r.Intn(4) != 0 {
			bld = bld.NumberRangeUpperBound(hi, hiInc)
			if hiInc {
				inside = append(inside, hi)
			}
			if hiDown.GreaterThan(lo).True() {
				inside = append(inside, hiDown)
			}
		}
		if notNull {
			bld = bld.NotNull()
		}
		u = bld.NewValue()
	case 1:
		prefix := names[r.Intn(len(names))]
		bld := cty.UnknownVal(cty.String).Refine().StringPrefix(prefix)
		if notNull {
			bld = bld.NotNull()
		}
		u = bld.NewValue()
		for _, rest := range []string{"", "x", "-1", "\U00000301"} {
			inside = append(inside, cty.StringVal(prefix+rest))
		}
	case 2:
		min := r.Intn(3)
		max := min + r.Intn(3)
		ty := []cty.Type{cty.List(cty.String), cty.Set(cty.Number), cty.Map(cty.Bool)}[r.Intn(3)]
		bld := cty.UnknownVal(ty).Refine().CollectionLengthLowerBound(min).CollectionLengthUpperBound(max)
		if notNull {
			bld = bld.NotNull()
		}
		u = bld.NewValue()
		for length := min; length <= max; length++ {
			inside = append(inside, collectionOf(ty, length))
		}
	default:
		w := []cty.Value{cty.True, cty.EmptyObjectVal, cty.TupleVal([]cty.Value{cty.NumberIntVal(1)})}[r.Intn(3)]
		u = cty.UnknownVal(w.Type())
		if notNull {
			u = u.RefineNotNull()
		}
		inside = append(inside, w)
	}
	if !notNull {
		inside = append(inside, cty.NullVal(u.Type()))
	}
	return u, inside
}

// collectionOf returns a known collection of type ty with length members.
func collectionOf(ty cty.Type, length int) cty.Value {
	switch {
	case length == 0 && ty.IsListType():
		return cty.ListValEmpty(ty.ElementType())
	case length == 0 && ty.IsSetType():
		return cty.SetValEmpty(ty.ElementType())
	case length == 0:
		return cty.MapValEmpty(ty.ElementType())
	case ty.IsListType():
		elems := make([]cty.Value, length)
		for i := range elems {
			elems[i] = cty.StringVal(strconv.Itoa(i))
		}
		return cty.ListVal(elems)
	case ty.IsSetType():
		elems := make([]cty.Value, length)
		for i := range elems {
			elems[i] = cty.NumberIntVal(int64(i))
		}
		return cty.SetVal(elems)
	}
	entries := map[string]cty.Value{}
	for i := range length {
		entries[strconv.Itoa(i)] = cty.True
	}
	return cty.MapVal(entries)
}

// randomNarrowedTenon returns a random unknown tenon value narrowed at
// random, and values inside its range.
func randomNarrowedTenon(r *rand.Rand) (tenon.Value, []tenon.Value) {
	var inside []tenon.Value
	var ns []tenon.Narrowing
	var typ tenon.Type
	notNull := r.Intn(2) == 0
	if notNull {
		ns = append(ns, tenon.NotNull())
	}
	switch r.Intn(3) {
	case 0:
		typ = num
		lo := tenon.NumberFromText(digits(r, 1+r.Intn(30)) + "e" + strconv.Itoa(r.Intn(41)-30))
		hi := tenon.Add(lo, tenon.NumberFromText(digits(r, 1+r.Intn(30))+"e"+strconv.Itoa(r.Intn(41)-30)))
		loInc, hiInc := r.Intn(2) == 0, r.Intn(2) == 0
		ns = append(ns, tenon.NumberMin(lo, loInc), tenon.NumberMax(hi, hiInc))
		inside = append(inside, tenon.Div(tenon.Add(lo, hi), n(2)))
		// A number just past a bound can cross as the bound itself, as one
		// 1e-200 past a bound of 1e-30 or more does.
		for _, tiny := range []tenon.Value{tenon.NumberFromText("1e-60"), tenon.NumberFromText("1e-200")} {
			inside = append(inside, tenon.Add(lo, tiny), tenon.Sub(hi, tiny))
		}
		if loInc {
			inside = append(inside, lo)
		}
		if hiInc {
			inside = append(inside, hi)
		}
	case 1:
		typ = str
		prefix := names[r.Intn(len(names))]
		ns = append(ns, tenon.StringPrefix(prefix))
		for _, rest := range []string{"", "x", "-1", "\U00000301"} {
			inside = append(inside, tenon.String(prefix+rest))
		}
	default:
		min := int64(r.Intn(3))
		max := min + int64(r.Intn(3))
		typ = []tenon.Type{tenon.ListType(str), tenon.SetType(num), tenon.MapType(boo)}[r.Intn(3)]
		ns = append(ns, tenon.LengthMin(min), tenon.LengthMax(max))
		for length := min; length <= max; length++ {
			inside = append(inside, tenonCollectionOf(typ, int(length)))
		}
	}
	if !notNull {
		inside = append(inside, tenon.Null(typ))
	}
	return tenon.Narrow(tenon.Unknown(typ), ns...), inside
}

// tenonCollectionOf returns a known collection of type typ with length
// members.
func tenonCollectionOf(typ tenon.Type, length int) tenon.Value {
	switch typ.Kind() {
	case tenon.KindList:
		elems := make([]tenon.Value, length)
		for i := range elems {
			elems[i] = tenon.String(strconv.Itoa(i))
		}
		return tenon.List(str, elems...)
	case tenon.KindSet:
		elems := make([]tenon.Value, length)
		for i := range elems {
			elems[i] = n(int64(i))
		}
		return tenon.Set(num, elems...)
	}
	entries := map[string]tenon.Value{}
	for i := range length {
		entries[strconv.Itoa(i)] = tenon.Bool(true)
	}
	return tenon.Map(boo, entries)
}

// TestPendingCollectionLengthsCross holds the length of a list, set or map
// whose element type cty has not settled to crossing both ways: an unknown
// one's length refinements, a known one's length, a set's as a range since
// its members that are not known may turn out to be one, and a pending
// tenon collection's lengths back to cty's refinements, which cty's length
// then reads.
func TestPendingCollectionLengthsCross(t *testing.T) {
	var b ctytenon.Bridge
	dynList, dynSet, dynMap := cty.List(cty.DynamicPseudoType), cty.Set(cty.DynamicPseudoType), cty.Map(cty.DynamicPseudoType)
	lists, sets, maps := tenon.ListOf(tenon.Any()), tenon.SetOf(tenon.Any()), tenon.MapOf(tenon.Any())
	for _, c := range []struct {
		name  string
		cty   cty.Value
		tenon tenon.Value
	}{
		{"an unknown list of at least two", cty.UnknownVal(dynList).Refine().CollectionLengthLowerBound(2).NewValue(),
			tenon.Narrow(tenon.Pending(lists), tenon.LengthMin(2))},
		{"an unknown set, not null, of at most three", cty.UnknownVal(dynSet).Refine().NotNull().CollectionLengthUpperBound(3).NewValue(),
			tenon.Narrow(tenon.Pending(sets), tenon.NotNull(), tenon.LengthMax(3))},
		{"an unknown map of one to four", cty.UnknownVal(dynMap).Refine().CollectionLengthLowerBound(1).CollectionLengthUpperBound(4).NewValue(),
			tenon.Narrow(tenon.Pending(maps), tenon.LengthMin(1), tenon.LengthMax(4))},
	} {
		got, err := b.FromCty(c.cty)
		if err != nil || !got.Equal(c.tenon) {
			t.Errorf("%s: FromCty(%#v) = %v, %v; want %v", c.name, c.cty, got, err, c.tenon)
			continue
		}
		if back, err := b.ToCty(got); err != nil || !back.RawEquals(c.cty) {
			t.Errorf("%s: ToCty(%v) = %#v, %v; want %#v", c.name, got, back, err, c.cty)
		}
	}

	for _, c := range []struct {
		name   string
		cty    cty.Value
		lo, hi int64
	}{
		{"a known list of two", cty.ListVal([]cty.Value{cty.DynamicVal, cty.DynamicVal}), 2, 2},
		{"a known map of one", cty.MapVal(map[string]cty.Value{"a": cty.DynamicVal}), 1, 1},
		{"a known set of two", cty.SetVal([]cty.Value{cty.DynamicVal, cty.DynamicVal}), 1, 2},
		{"a known empty list", cty.ListValEmpty(cty.DynamicPseudoType), 0, 0},
	} {
		got, err := b.FromCty(c.cty)
		if err != nil {
			t.Errorf("%s: FromCty: %v", c.name, err)
			continue
		}
		l := tenon.Length(got)
		lo, _, _ := l.Range().NumberMin()
		hi, _, ok := l.Range().NumberMax()
		if l.IsKnown() {
			lo, hi, ok = l, l, true
		}
		gotLo, _ := lo.AsInt64()
		gotHi, _ := hi.AsInt64()
		if !ok || gotLo != c.lo || gotHi != c.hi {
			t.Errorf("%s: FromCty(%#v) = %v, whose length is %v; want %d to %d", c.name, c.cty, got, l, c.lo, c.hi)
		}
		back, err := b.ToCty(got)
		if err != nil {
			t.Errorf("%s: ToCty(%v): %v", c.name, got, err)
			continue
		}
		if r := back.Range(); r.LengthLowerBound() != int(c.lo) || r.LengthUpperBound() != int(c.hi) {
			t.Errorf("%s: ToCty(%v) = %#v; want its length %d to %d", c.name, got, back, c.lo, c.hi)
		}
	}
}
