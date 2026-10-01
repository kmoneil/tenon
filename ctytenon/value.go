package ctytenon

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
	"github.com/zclconf/go-cty/cty"
)

// FromCty returns the tenon value of the cty value v: a value of the tenon
// type of v's type, as TypeFromCty gives it, holding the tenon values of what v
// holds. A number crosses as the decimal of the fewest digits that cty's
// parser reads as it, so that HCL's 0.1 is 0.1, or as its exact value where
// it is held in more than the 512 bits cty's parser makes. A null is the null
// of its type, and an unknown value the unknown value of its type, narrowed
// as cty's refinements of it say: not null, a number's bounds, a string's
// prefix and a collection's length. A bound crosses so that every number cty
// allows past it crosses as a number tenon allows, which can leave tenon's
// range allowing more than cty's, never less.
//
// A value whose type holds cty.DynamicPseudoType is a pending value, whose
// constraint is that type's: cty.DynamicPseudoType is [tenon.Any], and an
// object type is a closed [tenon.ObjectWith], since a value has exactly the
// attributes its type names. The untyped null, cty.NullVal of
// cty.DynamicPseudoType, is the pending value known to be null; a known value
// whose type holds cty.DynamicPseudoType, as a list holding cty.DynamicVal
// does, is the pending value known not to be null.
//
// A mark crosses as b.MarkFromCty maps it. cty hands a container's marks to
// every value read out of it, so they cross on the container and on every
// value within it, but the members of a set, which carry none.
//
// FromCty fails with a [*tenon.Error] where a part of v cannot cross, each
// diagnostic located by the part's path: a string or map key that is not
// UTF-8 (tenon.CodeStringInvalidUTF8), an infinity
// (tenon.CodeEncodeNotANumber), a number outside tenon's range
// (tenon.CodeNumberOutOfRange), attribute names tenon refuses, a mark the
// Bridge does not map (CodeUnmappedMark), and a capsule type the Bridge pairs
// with none, or a capsule value holding what its pair does not
// (CodeUnpairedCapsule). An element of a set is located by its place in cty's
// order. A failure within a value carrying a redacting mark is located at
// that value instead, and names it by its placeholder, as tenon's own
// diagnostics do, saying nothing of what it holds. It panics on cty.NilVal,
// which is not a value.
func (b Bridge) FromCty(v cty.Value) (tenon.Value, error) {
	if v.Type() == cty.NilType {
		usagePanic("FromCty called with cty.NilVal, which is not a value")
	}
	var f failures
	t := b.fromCty(v, tenon.Path{}, nil, &f)
	if len(f.list) > 0 {
		return tenon.Value{}, f.err()
	}
	return t, nil
}

// fromCty returns the tenon value of v, which lies at p within containers
// whose marks crossed as marks, adding to f what does not cross. cty hands
// those marks to v as it is read out of them, so v carries them too, beside
// its own. Where something does not cross, the value it returns stands for
// nothing.
func (b Bridge) fromCty(v cty.Value, p tenon.Path, marks []tenon.Mark, f *failures) tenon.Value {
	if v.IsMarked() {
		var own cty.ValueMarks
		v, own = v.Unmark()
		mapped, ok := b.marksFromCty(own, p, f)
		if !ok {
			return tenon.Value{}
		}
		marks = append(slices.Clip(marks), mapped...)
		defer f.redact(p, marks)()
	}
	t := b.unmarkedFromCty(v, p, marks, f)
	if t.IsZero() {
		return t
	}
	return tenon.WithMarks(t, marks...)
}

// marksFromCty returns the tenon marks of marks, cty's marks of the value at
// p, adding to f the failure of those the Bridge does not map.
func (b Bridge) marksFromCty(marks cty.ValueMarks, p tenon.Path, f *failures) ([]tenon.Mark, bool) {
	var mapped []tenon.Mark
	var unmapped []string
	for _, m := range sortedMarks(marks) {
		if b.MarkFromCty != nil {
			if t, ok := b.MarkFromCty(m); ok {
				mapped = append(mapped, t)
				continue
			}
		}
		unmapped = append(unmapped, fmt.Sprintf("%#v", m))
	}
	if len(unmapped) > 0 {
		f.add(p, CodeUnmappedMark, "the value carries the cty marks "+strings.Join(unmapped, ", ")+", which the Bridge maps to no tenon mark")
		return nil, false
	}
	return mapped, true
}

// sortedMarks returns cty's marks in the order of their Go syntax, so that
// what crosses does not depend on a map's order.
func sortedMarks(marks cty.ValueMarks) []any {
	out := make([]any, 0, len(marks))
	for m := range marks {
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b any) int { return strings.Compare(fmt.Sprintf("%#v", a), fmt.Sprintf("%#v", b)) })
	return out
}

// unmarkedFromCty returns the tenon value of v, which carries no mark of its
// own, as fromCty does, the values within it carrying marks. A failure is
// located at the part that fails: a container's type is asked of only where
// no part of it says the type, as of a null or an empty list.
func (b Bridge) unmarkedFromCty(v cty.Value, p tenon.Path, marks []tenon.Mark, f *failures) tenon.Value {
	// Optional attributes are a conversion's: a value has every attribute
	// its type names.
	t := v.Type().WithoutOptionalAttributesDeep()
	known := v.IsKnown() && !v.IsNull()
	if t.HasDynamicTypes() {
		// What a known one holds is not carried, but it may fail to cross
		// all the same.
		if _, ok := b.parts(v, t, p, marks, f); known && !ok {
			return tenon.Value{}
		}
		c, err := b.constraintFromCty(t, t, true)
		if err != nil {
			f.crossing(p, err)
			return tenon.Value{}
		}
		switch {
		case !v.IsKnown():
			return tenon.Narrow(tenon.Pending(c), narrowingsFromCty(v, t, true)...)
		case v.IsNull():
			return tenon.Narrow(tenon.Pending(c), tenon.NullOnly())
		}
		return tenon.Narrow(tenon.Pending(c), tenon.NotNull())
	}
	switch {
	case !known || t.IsCapsuleType():
		typ, err := b.typeFromCty(t, t)
		switch {
		case err != nil:
			f.crossing(p, err)
			return tenon.Value{}
		case !v.IsKnown():
			return tenon.Narrow(tenon.Unknown(typ), narrowingsFromCty(v, t, false)...)
		case v.IsNull():
			return tenon.Null(typ)
		}
		return b.capsuleFromCty(v, t, p, f)
	case t == cty.Bool:
		return tenon.Bool(v.True())
	case t == cty.Number:
		return f.data(p, numberFromCty(v.AsBigFloat()))
	case t == cty.String:
		return f.data(p, tenon.String(v.AsString()))
	}
	parts, ok := b.parts(v, t, p, marks, f)
	switch {
	case !ok:
		return tenon.Value{}
	case t.IsTupleType():
		return tenon.Tuple(parts.elems...)
	case t.IsObjectType():
		return f.data(p, tenon.Object(parts.entries))
	}
	elem, err := b.typeFromCty(t.ElementType(), t)
	switch {
	case err != nil:
		f.crossing(p, err)
		return tenon.Value{}
	case t.IsListType():
		return tenon.List(elem, parts.elems...)
	case t.IsSetType():
		return tenon.Set(elem, parts.elems...)
	}
	return f.data(p, tenon.Map(elem, parts.entries))
}

// crossed is what the parts of a cty collection or structural value cross as:
// the elements of a list, set or tuple, and the entries of a map or object.
type crossed struct {
	elems   []tenon.Value
	entries map[string]tenon.Value
}

// parts returns the tenon values of what v, a value of type t lying at p and
// read out with marks, holds, adding to f what does not cross, and false where
// something does. A value that is not known, or null, holds nothing. The
// members of a set carry no marks, the set carrying them.
func (b Bridge) parts(v cty.Value, t cty.Type, p tenon.Path, marks []tenon.Mark, f *failures) (crossed, bool) {
	var c crossed
	if !v.IsKnown() || v.IsNull() {
		return c, true
	}
	n := len(f.list)
	switch {
	case t.IsListType(), t.IsSetType(), t.IsTupleType():
		within := marks
		if t.IsSetType() {
			within = nil
		}
		c.elems = make([]tenon.Value, 0, v.LengthInt())
		for it := v.ElementIterator(); it.Next(); {
			_, e := it.Element()
			c.elems = append(c.elems, b.fromCty(e, p.Index(tenon.NumberFromInt(int64(len(c.elems)))), within, f))
		}
	case t.IsObjectType():
		// A name tenon refuses cannot step into the object.
		if err := checkNames(t.AttributeTypes(), t); err != nil {
			f.crossing(p, err)
			return c, false
		}
		c.entries = make(map[string]tenon.Value, v.LengthInt())
		for it := v.ElementIterator(); it.Next(); {
			k, e := it.Element()
			c.entries[k.AsString()] = b.fromCty(e, p.Attribute(k.AsString()), marks, f)
		}
	case t.IsMapType():
		c.entries = make(map[string]tenon.Value, v.LengthInt())
		for it := v.ElementIterator(); it.Next(); {
			k, e := it.Element()
			key := k.AsString()
			if !utf8.ValidString(key) {
				f.add(p, tenon.CodeStringInvalidUTF8, "the map key "+strconv.Quote(key)+" is not valid UTF-8")
				continue
			}
			c.entries[key] = b.fromCty(e, p.Index(tenon.String(key)), marks, f)
		}
	}
	return c, len(f.list) == n
}

// ToCty returns the cty value of the tenon value v: a value of the cty type of
// v's type, as TypeToCty gives it, holding the cty values of what v holds.
// A number crosses as the number cty's parser reads from its text, at 512
// bits. A null is the null of its type, and an unknown value the unknown value
// of its type, refined as its range says where cty can say it: not null, a
// number's bounds, a string's prefix and a collection's length. A bound that
// excludes itself includes itself in cty, since a number just past it can
// cross as the bound itself; a string's length, and a set's listed members,
// have no refinement and are left behind. Either leaves cty's range allowing
// more than tenon's, never less.
//
// A pending value is an unknown value, or a null where it is known to be null,
// whose type is the cty type of its constraint where cty has one type for all
// the types the constraint allows: [tenon.Any] is cty.DynamicPseudoType, and
// so is an ObjectWith that is open or has optional fields, which allows
// objects of attributes no one cty object type names.
//
// A mark crosses as b.MarkToCty maps it. cty hands a container's marks to
// every value read out of it, so a value within one carries in cty only the
// marks its containers do not.
//
// An error value crosses as the [*tenon.Error] holding it. ToCty fails with a
// [*tenon.Error] where a part of v cannot cross, each diagnostic located by
// the part's path: a mark the Bridge does not map (CodeUnmappedMark), a
// capsule type it pairs with none (CodeUnpairedCapsule), and a pending value
// whose constraint
// holds a OneOf (CodeOneOf). A failure within a value carrying a redacting
// mark is located at that value, as FromCty's are. It panics on the zero
// Value, which is not a value.
func (b Bridge) ToCty(v tenon.Value) (cty.Value, error) {
	if v.IsZero() {
		usagePanic("ToCty called with the zero Value, which is not a value")
	}
	if v.IsError() {
		return cty.NilVal, tenon.NewError(v)
	}
	var f failures
	c := b.toCty(v, tenon.Path{}, nil, &f)
	if len(f.list) > 0 {
		return cty.NilVal, f.err()
	}
	return c, nil
}

// toCty returns the cty value of v, which lies at p within containers whose
// marks crossed as marks, adding to f what does not cross. cty hands those
// marks to v as it is read out of them, so v carries in cty only its marks
// that they do not. Where something does not cross, the value it returns
// stands for nothing.
func (b Bridge) toCty(v tenon.Value, p tenon.Path, marks cty.ValueMarks, f *failures) cty.Value {
	if v, tmarks := tenon.Unmark(v); len(tmarks) > 0 {
		mapped, ok := b.marksToCty(tmarks, p, f)
		if !ok {
			return cty.NilVal
		}
		defer f.redact(p, tmarks)()
		n := len(f.list)
		c := b.unmarkedToCty(v, p, union(marks, mapped), f)
		if len(f.list) > n {
			return cty.NilVal
		}
		own := make(cty.ValueMarks, len(mapped))
		for m := range mapped {
			if _, held := marks[m]; !held {
				own[m] = struct{}{}
			}
		}
		if len(own) == 0 {
			return c
		}
		return c.WithMarks(own)
	}
	return b.unmarkedToCty(v, p, marks, f)
}

// union returns the marks of a and b together.
func union(a, b cty.ValueMarks) cty.ValueMarks {
	u := make(cty.ValueMarks, len(a)+len(b))
	for m := range a {
		u[m] = struct{}{}
	}
	for m := range b {
		u[m] = struct{}{}
	}
	return u
}

// marksToCty returns the cty marks of marks, tenon's marks of the value at p,
// adding to f the failure of those the Bridge does not map.
func (b Bridge) marksToCty(marks []tenon.Mark, p tenon.Path, f *failures) (cty.ValueMarks, bool) {
	mapped := make(cty.ValueMarks, len(marks))
	var unmapped []string
	for _, m := range marks {
		if b.MarkToCty != nil {
			if c, ok := b.MarkToCty(m); ok {
				mapped[c] = struct{}{}
				continue
			}
		}
		unmapped = append(unmapped, strconv.Quote(m.MarkID()))
	}
	if len(unmapped) > 0 {
		f.add(p, CodeUnmappedMark, "the value carries the marks "+strings.Join(unmapped, ", ")+", which the Bridge maps to no cty mark")
		return nil, false
	}
	return mapped, true
}

// unmarkedToCty returns the cty value of v, which carries no mark of its own,
// as toCty does, the values within it carrying marks. A failure is located at
// the part that fails, as fromCty's are.
func (b Bridge) unmarkedToCty(v tenon.Value, p tenon.Path, marks cty.ValueMarks, f *failures) cty.Value {
	if v.IsPending() {
		t, err := b.constraintToCty(v.Constraint(), v.Constraint(), true)
		switch {
		case err != nil:
			f.crossing(p, err)
			return cty.NilVal
		case v.IsNull():
			return cty.NullVal(t)
		case t != cty.DynamicPseudoType && notNull(v):
			return cty.UnknownVal(t).RefineNotNull()
		}
		return cty.UnknownVal(t)
	}
	kind := v.Type().Kind()
	if !v.HasContent() || kind == tenon.KindCapsule {
		t, err := b.typeToCty(v.Type(), v.Type())
		switch {
		case err != nil:
			f.crossing(p, err)
			return cty.NilVal
		case v.IsNull():
			return cty.NullVal(t)
		case !v.HasContent():
			return refineToCty(cty.UnknownVal(t), t, v.Range())
		}
		pair, _ := b.pairOfTenon(v.Type())
		return cty.CapsuleVal(t, pair.toCty(v))
	}
	switch kind {
	case tenon.KindBool:
		return cty.BoolVal(v.AsBool())
	case tenon.KindNumber:
		return numberToCty(v)
	case tenon.KindString:
		return cty.StringVal(v.AsString())
	}
	n := len(f.list)
	var elems []cty.Value
	var entries map[string]cty.Value
	switch kind {
	case tenon.KindList, tenon.KindSet, tenon.KindTuple:
		elems = make([]cty.Value, 0, v.Len())
		for e := range v.ElementsSeq() {
			elems = append(elems, b.toCty(e, p.Index(tenon.NumberFromInt(int64(len(elems)))), marks, f))
		}
	case tenon.KindMap:
		entries = make(map[string]cty.Value, v.Len())
		for k, e := range v.MapEntries() {
			entries[k] = b.toCty(e, p.Index(tenon.String(k)), marks, f)
		}
	default:
		entries = make(map[string]cty.Value, v.Len())
		for name, e := range v.Attributes() {
			entries[name] = b.toCty(e, p.Attribute(name), marks, f)
		}
	}
	switch {
	case len(f.list) > n:
		return cty.NilVal
	case kind == tenon.KindTuple:
		return cty.TupleVal(elems)
	case kind == tenon.KindObject:
		return cty.ObjectVal(entries)
	case v.Len() > 0 && kind == tenon.KindList:
		return cty.ListVal(elems)
	case v.Len() > 0 && kind == tenon.KindSet:
		return cty.SetVal(elems)
	case v.Len() > 0:
		return cty.MapVal(entries)
	}
	// An empty list, set or map names its element type, which no part says.
	elem, err := b.typeToCty(v.Type().ElementType(), v.Type())
	switch {
	case err != nil:
		f.crossing(p, err)
		return cty.NilVal
	case kind == tenon.KindList:
		return cty.ListValEmpty(elem)
	case kind == tenon.KindSet:
		return cty.SetValEmpty(elem)
	}
	return cty.MapValEmpty(elem)
}

// failures collects the diagnostics of a crossing, each located by its path,
// and each once.
type failures struct {
	list []tenon.Diagnostic
	// hidden, where it is not nil, is the outermost value being walked that
	// carries a redacting mark. A failure within it is located at it, and
	// names it by its placeholder, saying nothing of what it holds (MK-011).
	hidden *redaction
}

// redaction is the value failures are hidden within: where it lies, the
// placeholder that names it, and the codes of its failures so far.
type redaction struct {
	path        tenon.Path
	placeholder string
	codes       map[tenon.Code]bool
}

// err returns the error the failures make.
func (f *failures) err() error { return tenon.NewError(tenon.ErrorVal(f.list...)) }

// redact hides the failures within the value at p, where marks, its marks,
// hold a redacting one and no value around it has hidden them already, and
// returns what ends the hiding.
func (f *failures) redact(p tenon.Path, marks []tenon.Mark) func() {
	if f.hidden != nil {
		return func() {}
	}
	var redacting []tenon.Mark
	for _, m := range marks {
		if m.Redacting() {
			redacting = append(redacting, m)
		}
	}
	if len(redacting) == 0 {
		return func() {}
	}
	// tenon's display of a value under redacting marks is their placeholder.
	f.hidden = &redaction{path: p, placeholder: tenon.WithMarks(tenon.Null(tenon.BoolType()), redacting...).String(), codes: map[tenon.Code]bool{}}
	return func() { f.hidden = nil }
}

func (f *failures) add(p tenon.Path, code tenon.Code, message string) {
	f.append(tenon.Diagnostic{Code: code, Message: message, Path: p})
}

// append adds d, hidden where failures are, and once.
func (f *failures) append(d tenon.Diagnostic) {
	if h := f.hidden; h != nil {
		if h.codes[d.Code] {
			return
		}
		h.codes[d.Code] = true
		d.Path, d.Message = h.path, h.placeholder+" holds a part that does not cross"
	}
	f.list = append(f.list, d)
}

// data returns v, a value made from cty's data, unless it is an error value,
// whose diagnostics it adds, as arising at p.
func (f *failures) data(p tenon.Path, v tenon.Value) tenon.Value {
	if v.IsError() {
		f.within(p, v.Diagnostics())
		return tenon.Value{}
	}
	return v
}

// within adds diags, located within the value at p.
func (f *failures) within(p tenon.Path, diags []tenon.Diagnostic) {
	for _, d := range diags {
		at := p
		for _, s := range d.Path.Steps() {
			if s.Kind() == tenon.StepAttribute {
				at = at.Attribute(s.Name())
			} else {
				at = at.Index(s.Key())
			}
		}
		d.Path = at
		f.append(d)
	}
}

// crossing adds err, the failure of a type or constraint at p to cross.
func (f *failures) crossing(p tenon.Path, err error) {
	var c *crossingError
	if !errors.As(err, &c) {
		internalPanic("a crossing failed with %v", err)
	}
	if e := (*tenon.Error)(nil); errors.As(c.cause, &e) {
		f.within(p, e.Diagnostics())
		return
	}
	f.add(p, c.code, c.msg)
}
