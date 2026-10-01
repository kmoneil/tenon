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
// of its type, and an unknown value the unknown value of its type; the
// refinements cty keeps of an unknown value are left behind, which leaves the
// tenon value allowing more than cty's, never less.
//
// A value whose type holds cty.DynamicPseudoType is a pending value, whose
// constraint is that type's: cty.DynamicPseudoType is [tenon.Any], and an
// object type is a closed [tenon.ObjectWith], since a value has exactly the
// attributes its type names. The untyped null, cty.NullVal of
// cty.DynamicPseudoType, is the pending value known to be null; a known value
// whose type holds cty.DynamicPseudoType, as a list holding cty.DynamicVal
// does, is the pending value known not to be null.
//
// FromCty fails with a [*tenon.Error] where a part of v cannot cross, each
// diagnostic located by the part's path: a string or map key that is not
// UTF-8 (tenon.CodeStringInvalidUTF8), an infinity
// (tenon.CodeEncodeNotANumber), a number outside tenon's range
// (tenon.CodeNumberOutOfRange), attribute names tenon refuses, a marked value
// (CodeUnmappedMark), and a capsule type (CodeUnpairedCapsule). An element of
// a set is located by its place in cty's order. It panics on cty.NilVal, which
// is not a value.
func (b Bridge) FromCty(v cty.Value) (tenon.Value, error) {
	if v.Type() == cty.NilType {
		usagePanic("FromCty called with cty.NilVal, which is not a value")
	}
	var f failures
	t := b.fromCty(v, tenon.Path{}, &f)
	if len(f) > 0 {
		return tenon.Value{}, f.err()
	}
	return t, nil
}

// fromCty returns the tenon value of v, which lies at p, adding to f what does
// not cross. Where something does, the value it returns stands for nothing.
// A failure is located at the part that fails: a container's type is asked
// of only where no part of it says the type, as of a null or an empty list.
func (b Bridge) fromCty(v cty.Value, p tenon.Path, f *failures) tenon.Value {
	if v.IsMarked() {
		f.add(p, CodeUnmappedMark, "the value carries the cty marks "+ctyMarks(v.Marks())+", which the Bridge maps to no tenon mark")
		return tenon.Value{}
	}
	// Optional attributes are a conversion's: a value has every attribute
	// its type names.
	t := v.Type().WithoutOptionalAttributesDeep()
	known := v.IsKnown() && !v.IsNull()
	if t.HasDynamicTypes() {
		// What a known one holds is not carried, but it may fail to cross
		// all the same.
		if _, ok := b.parts(v, t, p, f); known && !ok {
			return tenon.Value{}
		}
		c, err := constraintFromCty(t, t, true)
		if err != nil {
			f.crossing(p, err)
			return tenon.Value{}
		}
		switch {
		case !v.IsKnown():
			return tenon.Pending(c)
		case v.IsNull():
			return tenon.Narrow(tenon.Pending(c), tenon.NullOnly())
		}
		return tenon.Narrow(tenon.Pending(c), tenon.NotNull())
	}
	switch {
	case !known || t.IsCapsuleType():
		typ, err := typeFromCty(t, t)
		switch {
		case err != nil:
			f.crossing(p, err)
			return tenon.Value{}
		case !v.IsKnown():
			return tenon.Unknown(typ)
		}
		return tenon.Null(typ)
	case t == cty.Bool:
		return tenon.Bool(v.True())
	case t == cty.Number:
		return f.data(p, numberFromCty(v.AsBigFloat()))
	case t == cty.String:
		return f.data(p, tenon.String(v.AsString()))
	}
	parts, ok := b.parts(v, t, p, f)
	switch {
	case !ok:
		return tenon.Value{}
	case t.IsTupleType():
		return tenon.Tuple(parts.elems...)
	case t.IsObjectType():
		return f.data(p, tenon.Object(parts.entries))
	}
	elem, err := typeFromCty(t.ElementType(), t)
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

// parts returns the tenon values of what v, a value of type t lying at p,
// holds, adding to f what does not cross, and false where something does. A
// value that is not known, or null, holds nothing.
func (b Bridge) parts(v cty.Value, t cty.Type, p tenon.Path, f *failures) (crossed, bool) {
	var c crossed
	if !v.IsKnown() || v.IsNull() {
		return c, true
	}
	n := len(*f)
	switch {
	case t.IsListType(), t.IsSetType(), t.IsTupleType():
		c.elems = make([]tenon.Value, 0, v.LengthInt())
		for it := v.ElementIterator(); it.Next(); {
			_, e := it.Element()
			c.elems = append(c.elems, b.fromCty(e, p.Index(tenon.NumberFromInt(int64(len(c.elems)))), f))
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
			c.entries[k.AsString()] = b.fromCty(e, p.Attribute(k.AsString()), f)
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
			c.entries[key] = b.fromCty(e, p.Index(tenon.String(key)), f)
		}
	}
	return c, len(*f) == n
}

// ctyMarks returns cty's marks as Go syntax, in order.
func ctyMarks(marks cty.ValueMarks) string {
	names := make([]string, 0, len(marks))
	for m := range marks {
		names = append(names, fmt.Sprintf("%#v", m))
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// ToCty returns the cty value of the tenon value v: a value of the cty type of
// v's type, as TypeToCty gives it, holding the cty values of what v holds.
// A number crosses as the number cty's parser reads from its text, at 512
// bits. A null is the null of its type, and an unknown value the unknown value
// of its type; what tenon knows of the range of an unknown value is left
// behind, which leaves the cty value allowing more than tenon's, never less.
//
// A pending value is an unknown value, or a null where it is known to be null,
// whose type is the cty type of its constraint where cty has one type for all
// the types the constraint allows: [tenon.Any] is cty.DynamicPseudoType, and
// so is an ObjectWith that is open or has optional fields, which allows
// objects of attributes no one cty object type names.
//
// An error value crosses as the [*tenon.Error] holding it. ToCty fails with a
// [*tenon.Error] where a part of v cannot cross, each diagnostic located by
// the part's path: a marked value (CodeUnmappedMark), a capsule type
// (CodeUnpairedCapsule), and a pending value whose constraint holds a OneOf
// (CodeOneOf). It panics on the zero Value, which is not a value.
func (b Bridge) ToCty(v tenon.Value) (cty.Value, error) {
	if v.IsZero() {
		usagePanic("ToCty called with the zero Value, which is not a value")
	}
	if v.IsError() {
		return cty.NilVal, tenon.NewError(v)
	}
	var f failures
	c := b.toCty(v, tenon.Path{}, &f)
	if len(f) > 0 {
		return cty.NilVal, f.err()
	}
	return c, nil
}

// toCty returns the cty value of v, which lies at p, adding to f what does not
// cross. Where something does, the value it returns stands for nothing. A
// failure is located at the part that fails, as fromCty's are.
func (b Bridge) toCty(v tenon.Value, p tenon.Path, f *failures) cty.Value {
	if _, marks := tenon.Unmark(v); len(marks) > 0 {
		ids := make([]string, len(marks))
		for i, m := range marks {
			ids[i] = strconv.Quote(m.MarkID())
		}
		f.add(p, CodeUnmappedMark, "the value carries the marks "+strings.Join(ids, ", ")+", which the Bridge maps to no cty mark")
		return cty.NilVal
	}
	if v.IsPending() {
		t, err := constraintToCty(v.Constraint(), v.Constraint(), true)
		switch {
		case err != nil:
			f.crossing(p, err)
			return cty.NilVal
		case v.IsNull():
			return cty.NullVal(t)
		}
		return cty.UnknownVal(t)
	}
	kind := v.Type().Kind()
	if !v.HasContent() || kind == tenon.KindCapsule {
		t, err := typeToCty(v.Type(), v.Type())
		switch {
		case err != nil:
			f.crossing(p, err)
			return cty.NilVal
		case v.IsNull():
			return cty.NullVal(t)
		}
		return cty.UnknownVal(t)
	}
	switch kind {
	case tenon.KindBool:
		return cty.BoolVal(v.AsBool())
	case tenon.KindNumber:
		return numberToCty(v)
	case tenon.KindString:
		return cty.StringVal(v.AsString())
	}
	n := len(*f)
	var elems []cty.Value
	var entries map[string]cty.Value
	switch kind {
	case tenon.KindList, tenon.KindSet, tenon.KindTuple:
		elems = make([]cty.Value, 0, v.Len())
		for e := range v.ElementsSeq() {
			elems = append(elems, b.toCty(e, p.Index(tenon.NumberFromInt(int64(len(elems)))), f))
		}
	case tenon.KindMap:
		entries = make(map[string]cty.Value, v.Len())
		for k, e := range v.MapEntries() {
			entries[k] = b.toCty(e, p.Index(tenon.String(k)), f)
		}
	default:
		entries = make(map[string]cty.Value, v.Len())
		for name, e := range v.Attributes() {
			entries[name] = b.toCty(e, p.Attribute(name), f)
		}
	}
	switch {
	case len(*f) > n:
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
	elem, err := typeToCty(v.Type().ElementType(), v.Type())
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

// failures collects the diagnostics of a crossing, each located by its path.
type failures []tenon.Diagnostic

// err returns the error the failures make.
func (f failures) err() error { return tenon.NewError(tenon.ErrorVal(f...)) }

func (f *failures) add(p tenon.Path, code tenon.Code, message string) {
	*f = append(*f, tenon.Diagnostic{Code: code, Message: message, Path: p})
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
		*f = append(*f, d)
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
