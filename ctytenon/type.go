package ctytenon

import (
	"fmt"
	"maps"
	"slices"

	"github.com/kmoneil/tenon"
	"github.com/zclconf/go-cty/cty"
)

// TypeFromCty returns the tenon type of the cty type t: cty.Bool, cty.Number
// and cty.String are tenon's Bool, Number and String; a list, set or map is
// one of the tenon type of its elements, and a tuple or an object one of its
// elements' or attributes' tenon types.
//
// It fails where t is not a type tenon can hold: where t holds
// cty.DynamicPseudoType or is an object type with optional attributes, which
// make it a type constraint rather than a type, as [Bridge.ConstraintFromCty]
// carries one; where an object type's attribute names are names tenon
// refuses, as [tenon.CheckAttributeNames] says; or where t holds a capsule
// type the Bridge pairs with none: a paired one is its pair (Bridge.Capsules).
// It panics on cty.NilType, which is not a type.
func (b Bridge) TypeFromCty(t cty.Type) (tenon.Type, error) {
	if t == cty.NilType {
		usagePanic("TypeFromCty called with cty.NilType, which is not a type")
	}
	if t.HasDynamicTypes() {
		return tenon.Type{}, fmt.Errorf("ctytenon: %#v holds cty.DynamicPseudoType, and is a type constraint rather than a type", t)
	}
	return b.typeFromCty(t, t)
}

// typeFromCty returns the tenon type of t, a part of whole, which the error
// of a part that does not cross names.
func (b Bridge) typeFromCty(t, whole cty.Type) (tenon.Type, error) {
	switch {
	case t == cty.Bool:
		return tenon.BoolType(), nil
	case t == cty.Number:
		return tenon.NumberType(), nil
	case t == cty.String:
		return tenon.StringType(), nil
	case t.IsListType(), t.IsSetType(), t.IsMapType():
		elem, err := b.typeFromCty(t.ElementType(), whole)
		if err != nil {
			return tenon.Type{}, err
		}
		switch {
		case t.IsListType():
			return tenon.ListType(elem), nil
		case t.IsSetType():
			return tenon.SetType(elem), nil
		}
		return tenon.MapType(elem), nil
	case t.IsTupleType():
		elems := make([]tenon.Type, 0, len(t.TupleElementTypes()))
		for _, e := range t.TupleElementTypes() {
			elem, err := b.typeFromCty(e, whole)
			if err != nil {
				return tenon.Type{}, err
			}
			elems = append(elems, elem)
		}
		return tenon.TupleType(elems...), nil
	case t.IsObjectType():
		if len(t.OptionalAttributes()) > 0 {
			return tenon.Type{}, fmt.Errorf("ctytenon: %#v has an object type with optional attributes, and is a type constraint rather than a type", whole)
		}
		attrs := t.AttributeTypes()
		if err := checkNames(attrs, whole); err != nil {
			return tenon.Type{}, err
		}
		fields := make(map[string]tenon.Type, len(attrs))
		for name, a := range attrs {
			attr, err := b.typeFromCty(a, whole)
			if err != nil {
				return tenon.Type{}, err
			}
			fields[name] = attr
		}
		return tenon.ObjectType(fields), nil
	}
	// What is left is a capsule type: cty has no other kind of type, and the
	// callers have taken cty.DynamicPseudoType out.
	if pair, ok := b.pairOfCty(t); ok {
		return pair.tenon, nil
	}
	return tenon.Type{}, &crossingError{code: CodeUnpairedCapsule, msg: fmt.Sprintf("%#v holds the capsule type %s, which the Bridge pairs with no tenon type", whole, t.FriendlyName())}
}

// checkNames returns the error of an object type of whole, with attributes
// attrs, whose names tenon refuses, and nil where it takes them all.
func checkNames(attrs map[string]cty.Type, whole cty.Type) error {
	if err := tenon.CheckAttributeNames(slices.Sorted(maps.Keys(attrs))...); err != nil {
		return &crossingError{msg: fmt.Sprintf("%#v has an object type whose attribute names tenon refuses", whole), cause: err}
	}
	return nil
}

// TypeToCty returns the cty type of the tenon type t, as TypeFromCty gives
// the tenon type of a cty one. It fails where t holds a capsule type the
// Bridge pairs with none, and panics on the zero Type, which is not a type.
func (b Bridge) TypeToCty(t tenon.Type) (cty.Type, error) {
	if t.IsZero() {
		usagePanic("TypeToCty called with the zero Type, which is not a type")
	}
	return b.typeToCty(t, t)
}

// typeToCty returns the cty type of t, a part of whole, a type or a
// constraint, which the error of a part that does not cross names.
func (b Bridge) typeToCty(t tenon.Type, whole fmt.Stringer) (cty.Type, error) {
	switch t.Kind() {
	case tenon.KindBool:
		return cty.Bool, nil
	case tenon.KindNumber:
		return cty.Number, nil
	case tenon.KindString:
		return cty.String, nil
	case tenon.KindList, tenon.KindSet, tenon.KindMap:
		elem, err := b.typeToCty(t.ElementType(), whole)
		if err != nil {
			return cty.NilType, err
		}
		switch t.Kind() {
		case tenon.KindList:
			return cty.List(elem), nil
		case tenon.KindSet:
			return cty.Set(elem), nil
		}
		return cty.Map(elem), nil
	case tenon.KindTuple:
		parts := t.TupleElementTypes()
		elems := make([]cty.Type, 0, len(parts))
		for _, e := range parts {
			elem, err := b.typeToCty(e, whole)
			if err != nil {
				return cty.NilType, err
			}
			elems = append(elems, elem)
		}
		return cty.Tuple(elems), nil
	case tenon.KindObject:
		attrs := make(map[string]cty.Type, len(t.AttributeNames()))
		for _, name := range t.AttributeNames() {
			attr, err := b.typeToCty(t.AttributeType(name), whole)
			if err != nil {
				return cty.NilType, err
			}
			attrs[name] = attr
		}
		return cty.Object(attrs), nil
	}
	if pair, ok := b.pairOfTenon(t); ok {
		return pair.cty, nil
	}
	return cty.NilType, &crossingError{code: CodeUnpairedCapsule, msg: fmt.Sprintf("%s holds the capsule type %q, which the Bridge pairs with no cty type", whole, t.CapsuleName())}
}

// crossingError is why a type or a constraint does not cross, where a value
// of it can meet the same: what it says of the whole, and the code a value's
// crossing reports it with, or the tenon error that says why.
type crossingError struct {
	code  tenon.Code
	msg   string
	cause error
}

func (e *crossingError) Error() string {
	if e.cause != nil {
		return "ctytenon: " + e.msg + ": " + e.cause.Error()
	}
	return "ctytenon: " + e.msg
}

func (e *crossingError) Unwrap() error { return e.cause }
