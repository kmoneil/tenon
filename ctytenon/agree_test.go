package ctytenon_test

import (
	"maps"
	"math/big"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// The tests in this file hold tenon and cty to agreeing where both are right:
// they ask each the same question of random values crossed between them, and
// where cty answers, tenon answers the same. Where cty does not, tenon may,
// being the more decided (D-043), but never the other way round.

// agreement checks that tenon's answer, a Bool value, agrees with cty's: the
// same where cty's is known, and anything where it is not.
func agreement(t *testing.T, what string, cty_ cty.Value, ten tenon.Value) {
	t.Helper()
	cty_, _ = cty_.Unmark()
	ten, _ = tenon.Unmark(ten)
	if !cty_.IsKnown() {
		return
	}
	if !ten.IsKnown() || ten.AsBool() != cty_.True() {
		t.Errorf("%s: cty answers %#v, and tenon %v", what, cty_, ten)
	}
}

// pairOf returns a cty value of the type of v to compare with it: v itself,
// a value of the same type at random, or v with one part changed.
func pairOf(r *rand.Rand, v cty.Value) cty.Value {
	switch r.Intn(3) {
	case 0:
		return v
	case 1:
		return randomCtyValue(r, v.Type())
	}
	return changeOne(r, v)
}

// changeOne returns v with one part, chosen at random, made anew.
func changeOne(r *rand.Rand, v cty.Value) cty.Value {
	if !v.IsKnown() || v.IsNull() || !(v.Type().IsListType() || v.Type().IsTupleType() || v.Type().IsObjectType() || v.Type().IsMapType()) || v.LengthInt() == 0 {
		return randomCtyValue(r, v.Type())
	}
	which := r.Intn(v.LengthInt())
	i := 0
	switch {
	case v.Type().IsObjectType() || v.Type().IsMapType():
		m := map[string]cty.Value{}
		for it := v.ElementIterator(); it.Next(); i++ {
			k, e := it.Element()
			if i == which {
				e = changeOne(r, e)
			}
			m[k.AsString()] = e
		}
		if v.Type().IsObjectType() {
			return cty.ObjectVal(m)
		}
		return cty.MapVal(m)
	}
	var elems []cty.Value
	for it := v.ElementIterator(); it.Next(); i++ {
		_, e := it.Element()
		if i == which {
			e = changeOne(r, e)
		}
		elems = append(elems, e)
	}
	if v.Type().IsTupleType() {
		return cty.TupleVal(elems)
	}
	return cty.ListVal(elems)
}

// TestEqualsAgrees asks cty's Equals and tenon's of random pairs of values,
// unknown and null parts among them.
func TestEqualsAgrees(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261012))
	for range conformance.Iterations(t, 3000) {
		v := randomCtyValue(r, randomCtyType(r, 3))
		w := pairOf(r, v)
		tv, err := b.FromCty(v)
		if err != nil {
			t.Fatal(err)
		}
		tw, err := b.FromCty(w)
		if err != nil {
			t.Fatal(err)
		}
		ce, te := v.Equals(w), tenon.Equals(tv, tw)
		// cty answers false of a set holding a member with an unknown part,
		// even compared with itself (TestCtySetHoldingAnUnknownEqualsItself),
		// so its answer says nothing there.
		if ce.IsKnown() && ce.False() && (holdsPartlyUnknownMember(v) || holdsPartlyUnknownMember(w)) {
			continue
		}
		agreement(t, "Equals("+v.GoString()+", "+w.GoString()+")", ce, te)
	}
}

// holdsPartlyUnknownMember reports whether v holds, at any depth, a known set
// with a member that is known but not wholly.
func holdsPartlyUnknownMember(v cty.Value) bool {
	found := false
	cty.Walk(v, func(_ cty.Path, at cty.Value) (bool, error) {
		if at.Type().IsSetType() && at.IsKnown() && !at.IsNull() {
			for it := at.ElementIterator(); it.Next(); {
				if _, e := it.Element(); e.IsKnown() && !e.IsWhollyKnown() {
					found = true
				}
			}
		}
		return !found, nil
	})
	return found
}

// TestCtySetHoldingAnUnknownEqualsItself probes what TestEqualsAgrees
// allows: cty answers false where a set holds a known member with an unknown
// part, even of the set compared with itself, which can be no other value.
// tenon answers unknown. go-cty has no open issue of it as of 2026-10-01.
func TestCtySetHoldingAnUnknownEqualsItself(t *testing.T) {
	s := cty.SetVal([]cty.Value{cty.ListVal([]cty.Value{cty.UnknownVal(cty.Bool)})})
	if got := s.Equals(s); !got.RawEquals(cty.False) {
		t.Errorf("cty answers %#v; the defect is fixed, and TestEqualsAgrees allows it no longer", got)
	}
	var b ctytenon.Bridge
	ts, err := b.FromCty(s)
	if err != nil {
		t.Fatal(err)
	}
	if got := tenon.Equals(ts, ts); got.IsKnown() {
		t.Errorf("tenon answers %v, want unknown", got)
	}
}

// TestNullAndKnownAgree asks whether random values are null, known at their
// top, and known throughout, of cty and of tenon.
func TestNullAndKnownAgree(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261013))
	for range conformance.Iterations(t, 3000) {
		var v cty.Value
		if r.Intn(4) == 0 {
			ct := randomCtyConstraint(r, 2).WithoutOptionalAttributesDeep()
			v = []cty.Value{cty.UnknownVal(ct), cty.NullVal(ct)}[r.Intn(2)]
		} else {
			v = randomCtyValue(r, randomCtyType(r, 3))
		}
		tv, err := b.FromCty(v)
		if err != nil {
			t.Fatal(err)
		}
		if v.IsNull() != tv.IsNull() {
			t.Errorf("%#v: cty says it is null %v, and tenon %v of %v", v, v.IsNull(), tv.IsNull(), tv)
		}
		// tenon may settle what cty leaves unknown, but not the reverse.
		if known := tv.IsNull() || tv.HasContent(); v.IsKnown() && !known {
			t.Errorf("%#v is known to cty, but %v is not to tenon", v, tv)
		}
		// A null whose type holds cty.DynamicPseudoType is known to cty, and
		// to tenon a pending value known to be null, whose type it does not
		// know (D-246); both say it is null.
		if v.IsWhollyKnown() && !tv.IsKnown() && !holdsNullOfSomeType(v) {
			t.Errorf("%#v is wholly known to cty, but %v is not known to tenon", v, tv)
		}
	}
}

// holdsNullOfSomeType reports whether v is, or holds, a null whose type
// holds cty.DynamicPseudoType.
func holdsNullOfSomeType(v cty.Value) bool {
	found := false
	cty.Walk(v, func(_ cty.Path, at cty.Value) (bool, error) {
		found = found || at.IsNull() && at.Type().HasDynamicTypes()
		return !found, nil
	})
	return found
}

// lengthOf returns cty's length of v: its function strlen's of a string,
// which counts grapheme clusters, as tenon's Length does, and Length's of a
// collection.
func lengthOf(v cty.Value) cty.Value {
	if v.Type() == cty.String {
		n, err := stdlib.Strlen(v)
		if err != nil {
			return cty.UnknownVal(cty.Number)
		}
		return n
	}
	return v.Length()
}

// TestLengthsAgree asks the length of random collections and strings, known
// and unknown, of cty and of tenon.
func TestLengthsAgree(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261014))
	asked := 0
	for range conformance.Iterations(t, 3000) {
		typ := []cty.Type{cty.String, cty.List(randomCtyType(r, 1)), cty.Set(randomCtyType(r, 1)), cty.Map(randomCtyType(r, 1))}[r.Intn(4)]
		v := randomCtyValue(r, typ)
		if v.IsNull() {
			continue
		}
		tv, err := b.FromCty(v)
		if err != nil {
			t.Fatal(err)
		}
		cl, tl := lengthOf(v), tenon.Length(tv)
		if cl.IsKnown() {
			asked++
			if !tl.IsKnown() || tl.AsBigRat().Cmp(mustRat(cl.AsBigFloat())) != 0 {
				t.Errorf("the length of %#v is %#v to cty, and %v to tenon", v, cl, tl)
			}
		}
	}
	if asked < 1000 {
		t.Errorf("compared %d known lengths, want many", asked)
	}
}

// mustRat returns the exact value of f.
func mustRat(f *big.Float) *big.Rat {
	r, _ := f.Rat(nil)
	return r
}

// TestConversionsAgree converts random values to types like theirs, kinds
// and primitives swapped, attributes dropped, added and made optional, and
// parts made cty.DynamicPseudoType, with cty's Convert and with tenon's,
// under the Unsafe policy, which is cty's. Where both convert, the results
// are of one type; where one fails, so does the other. The exceptions, each
// told by what the value and target hold:
//
//   - tenon keeps an attribute cty's object conversion drops (D-246), and
//     so does not settle the type of an object of attributes not known, as
//     an unknown map converted to an object is;
//   - cty's result holds cty.DynamicPseudoType where tenon settles the type,
//     as of an empty list, tenon being more decided;
//   - cty converts a set holding an unknown member to an unknown of the
//     set's own type rather than the target's (go-cty #216), so it gives a
//     value the target does not allow, or fails on the wrong type further
//     out, where tenon converts it or fails on a known member;
//   - tenon unifies Bool and Number as String under the Unsafe policy
//     (CV-042), where cty unifies them only beside a String;
//   - tenon converts a list or set to a tuple of its length under the Unsafe
//     policy (CV-022), which cty does not.
func TestConversionsAgree(t *testing.T) {
	var b ctytenon.Bridge
	r := rand.New(rand.NewSource(20261015))
	both := 0
	for range conformance.Iterations(t, 5000) {
		v := randomCtyValue(r, randomCtyType(r, 2))
		target := relatedType(r, v.Type())
		cv, cerr := convert.Convert(v, target)
		tv, err := b.FromCty(v)
		if err != nil {
			t.Fatal(err)
		}
		c, err := b.ConstraintFromCty(target)
		if err != nil {
			t.Fatal(err)
		}
		tr := tenon.Convert(tv, c, tenon.Unsafe)
		issue216 := holdsUnknownSetMember(v)
		switch {
		case cerr == nil && tr.IsError():
			if !issue216 {
				t.Errorf("cty converts %#v to %#v as %#v, and tenon fails: %v", v, target, cv, tr)
			}
		case cerr != nil && !tr.IsError():
			msg := cerr.Error()
			unified := (strings.Contains(msg, "must have the same type") || strings.Contains(msg, "must all match")) && typeHolds(v.Type(), cty.Number) && typeHolds(v.Type(), cty.Bool)
			tupled := strings.Contains(msg, "tuple required") && typeHoldsCollection(v.Type())
			if !unified && !tupled && !issue216 {
				t.Errorf("tenon converts %#v to %#v as %v, and cty fails: %v", v, target, tr, cerr)
			}
		case cerr == nil:
			both++
			ct, err := b.TypeFromCty(cv.Type())
			switch {
			case issue216:
			case err != nil && cv.Type().HasDynamicTypes():
			case err != nil:
				t.Errorf("cty converts %#v to %#v as %#v, whose type does not cross: %v", v, target, cv, err)
			case tr.IsPending():
				// An object of attributes not known, as an unknown map
				// converted to an object is, keeps them in tenon (D-246),
				// so its type is not settled where cty's, dropping them, is.
				if !typeHoldsObject(target) {
					t.Errorf("cty converts %#v to %#v as %#v, and tenon to %v", v, target, cv, tr)
				}
			case ct != tr.Type():
				// tenon keeps what cty drops, which cty drops again. The
				// values may differ where a number converts to a string:
				// cty writes every digit, tenon its canonical text (NU-020).
				back, err := b.ToCty(tr)
				again, err2 := convert.Convert(back, target)
				if err != nil || err2 != nil || !again.Type().Equals(cv.Type()) {
					t.Errorf("cty converts %#v to %#v as %#v, and tenon as %v", v, target, cv, tr)
				}
			}
		}
	}
	if both < 3000 {
		t.Errorf("both converted %d values, want most", both)
	}
}

// holdsUnknownSetMember reports whether v holds, at any depth, a known set
// with a member that is not wholly known.
func holdsUnknownSetMember(v cty.Value) bool {
	found := false
	cty.Walk(v, func(_ cty.Path, at cty.Value) (bool, error) {
		if at.Type().IsSetType() && at.IsKnown() && !at.IsNull() && !at.IsWhollyKnown() {
			found = true
		}
		return !found, nil
	})
	return found
}

// typeHolds reports whether t is or holds the primitive type p.
func typeHolds(t, p cty.Type) bool {
	switch {
	case t.Equals(p):
		return true
	case t.IsCollectionType():
		return typeHolds(t.ElementType(), p)
	case t.IsTupleType():
		for _, e := range t.TupleElementTypes() {
			if typeHolds(e, p) {
				return true
			}
		}
	case t.IsObjectType():
		for _, a := range t.AttributeTypes() {
			if typeHolds(a, p) {
				return true
			}
		}
	}
	return false
}

// typeHoldsObject reports whether t is or holds an object type.
func typeHoldsObject(t cty.Type) bool {
	switch {
	case t.IsObjectType():
		return true
	case t.IsCollectionType():
		return typeHoldsObject(t.ElementType())
	case t.IsTupleType():
		for _, e := range t.TupleElementTypes() {
			if typeHoldsObject(e) {
				return true
			}
		}
	}
	return false
}

// typeHoldsCollection reports whether t is or holds a list or a set type.
func typeHoldsCollection(t cty.Type) bool {
	switch {
	case t.IsListType(), t.IsSetType():
		return true
	case t.IsMapType():
		return typeHoldsCollection(t.ElementType())
	case t.IsTupleType():
		for _, e := range t.TupleElementTypes() {
			if typeHoldsCollection(e) {
				return true
			}
		}
	case t.IsObjectType():
		for _, a := range t.AttributeTypes() {
			if typeHoldsCollection(a) {
				return true
			}
		}
	}
	return false
}

// relatedType returns a type like t: kinds and primitives swapped,
// attributes dropped, added and made optional, and parts made
// cty.DynamicPseudoType, but not within a collection's element type, where
// cty and tenon unify the members' types by rules of their own
// (TestConversionsDiffer).
func relatedType(r *rand.Rand, t cty.Type) cty.Type {
	return related(r, t, false)
}

// related is relatedType, within a collection's element type where elem is
// true.
func related(r *rand.Rand, t cty.Type, elem bool) cty.Type {
	if !elem && r.Intn(6) == 0 {
		return cty.DynamicPseudoType
	}
	switch {
	case t.IsPrimitiveType():
		return []cty.Type{cty.String, cty.Number, cty.Bool, t, t}[r.Intn(5)]
	case t.IsListType(), t.IsSetType():
		e := related(r, t.ElementType(), true)
		return []cty.Type{cty.List(e), cty.Set(e), cty.List(e)}[r.Intn(3)]
	case t.IsMapType():
		return cty.Map(related(r, t.ElementType(), true))
	case t.IsTupleType():
		var elems []cty.Type
		for _, e := range t.TupleElementTypes() {
			elems = append(elems, related(r, e, elem))
		}
		if r.Intn(4) == 0 && len(elems) > 0 {
			return cty.List(related(r, t.TupleElementTypes()[0], true))
		}
		return cty.Tuple(elems)
	case t.IsObjectType():
		attrs := map[string]cty.Type{}
		var optional []string
		for _, name := range slices.Sorted(maps.Keys(t.AttributeTypes())) {
			if r.Intn(5) == 0 {
				continue
			}
			attrs[name] = related(r, t.AttributeType(name), elem)
			if r.Intn(4) == 0 {
				optional = append(optional, name)
			}
		}
		if r.Intn(4) == 0 {
			attrs["extra"] = cty.String
			optional = append(optional, "extra")
		}
		return cty.ObjectWithOptionalAttrs(attrs, optional)
	}
	return t
}

// TestConversionsDiffer pins where cty's conversion and tenon's differ by
// rule, so that a change on either side is seen: the rules each has for
// unifying the types of a collection's members, tenon's (CV-042, CV-044)
// commutative and associative and cty's not, tenon settling each member's
// type before unifying them where cty unifies first; the conversions tenon
// has that cty does not; and the text a number converts to.
func TestConversionsDiffer(t *testing.T) {
	var b ctytenon.Bridge
	tup := func(vs ...cty.Value) cty.Value { return cty.TupleVal(vs) }
	str, num := cty.StringVal("a"), cty.NumberIntVal(1)
	for _, c := range []struct {
		name   string
		v      cty.Value
		target cty.Type
		cty    string // cty's result, or its error
		tenon  string // tenon's result
	}{
		{
			"a number and a bool unify as a string in tenon (CV-042)",
			tup(num, cty.True), cty.List(cty.DynamicPseudoType),
			"all list elements must have the same type",
			`list(string)["1", "true"]`,
		},
		{
			"a list converts to a tuple of its length in tenon (CV-022)",
			cty.ListVal([]cty.Value{str}), cty.Tuple([]cty.Type{cty.String}),
			"tuple required",
			`["a"]`,
		},
		{
			"objects of different attributes unify as an object in tenon (CV-044), as a map in cty",
			tup(cty.ObjectVal(map[string]cty.Value{"a": str}), cty.ObjectVal(map[string]cty.Value{"b": str})), cty.List(cty.DynamicPseudoType),
			`cty.ListVal([]cty.Value{cty.MapVal(map[string]cty.Value{"a":cty.StringVal("a")}), cty.MapVal(map[string]cty.Value{"b":cty.StringVal("a")})})`,
			`list(object({"a": string, "b": string}))[{"a": "a", "b": null}, {"a": null, "b": "a"}]`,
		},
		{
			"an empty tuple settles no element type beside one that does, in tenon (CV-044)",
			tup(tup(str), cty.EmptyTupleVal), cty.List(cty.List(cty.DynamicPseudoType)),
			`cty.ListVal([]cty.Value{cty.ListVal([]cty.Value{cty.StringVal("a")}), cty.ListValEmpty(cty.String)})`,
			`error(convert.no_common_type: "nothing settles an element type for any: there are no members, and the constraint admits more than one type" at .[1])`,
		},
		{
			"a number converts to a string as its canonical text in tenon (NU-020), every digit in cty",
			cty.MustParseNumberVal("1e30"), cty.String,
			`cty.StringVal("1000000000000000000000000000000")`,
			`"1e30"`,
		},
		{
			"an empty tuple settles no element type, in tenon (CV-044)",
			cty.EmptyTupleVal, cty.List(cty.DynamicPseudoType),
			`cty.ListValEmpty(cty.DynamicPseudoType)`,
			`error(convert.no_common_type: "nothing settles an element type for any: there are no members, and the constraint admits more than one type")`,
		},
	} {
		cv, cerr := convert.Convert(c.v, c.target)
		got := ""
		if cerr != nil {
			got = cerr.Error()
		} else {
			got = cv.GoString()
		}
		if got != c.cty {
			t.Errorf("%s: cty converts %#v to %#v as %s; want %s", c.name, c.v, c.target, got, c.cty)
		}
		tv, err := b.FromCty(c.v)
		if err != nil {
			t.Fatal(err)
		}
		k, err := b.ConstraintFromCty(c.target)
		if err != nil {
			t.Fatal(err)
		}
		if got := tenon.Convert(tv, k, tenon.Unsafe).String(); got != c.tenon {
			t.Errorf("%s: tenon converts %v to %v as %s; want %s", c.name, tv, k, got, c.tenon)
		}
	}
}
