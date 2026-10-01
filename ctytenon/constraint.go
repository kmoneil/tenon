package ctytenon

import (
	"fmt"
	"maps"
	"slices"

	"github.com/kmoneil/tenon"
	"github.com/zclconf/go-cty/cty"
)

// ConstraintFromCty returns the tenon constraint of the cty type constraint t,
// a type that may hold cty.DynamicPseudoType and object types with optional
// attributes, as the target of cty's conversions and Terraform's variable
// types are. cty.DynamicPseudoType, which every type conforms to, is
// [tenon.Any]; a list, set or map is ListOf, SetOf or MapOf the constraint of
// its elements, and a tuple TupleOf its elements' constraints; and a part that
// is one type, holding neither cty.DynamicPseudoType nor an object type, is
// Exactly that type, so that the constraint is written as [tenon.Unify] writes
// one.
//
// An object type is an open [tenon.ObjectWith], with a field for each
// attribute, optional where the attribute is. The difference is cty's: its
// conversion to an object type accepts an object with attributes the type
// does not name and drops them, where tenon's never drops an attribute. The
// open constraint accepts every object cty's conversion does, and keeps what
// cty would drop.
//
// It fails where an object type's attribute names are names tenon refuses, as
// [tenon.CheckAttributeNames] says, or where t holds a capsule type. It
// panics on cty.NilType, which is not a type constraint.
func (b Bridge) ConstraintFromCty(t cty.Type) (tenon.Constraint, error) {
	if t == cty.NilType {
		usagePanic("ConstraintFromCty called with cty.NilType, which is not a type constraint")
	}
	return constraintFromCty(t, t)
}

// constraintFromCty returns the tenon constraint of t, a part of whole, which
// the error of a part that does not cross names.
func constraintFromCty(t, whole cty.Type) (tenon.Constraint, error) {
	switch {
	case t == cty.DynamicPseudoType:
		return tenon.Any(), nil
	case t.IsListType(), t.IsSetType(), t.IsMapType():
		elem, err := constraintFromCty(t.ElementType(), whole)
		if err != nil {
			return tenon.Constraint{}, err
		}
		if elem.Kind() == tenon.ConstraintExactly {
			switch {
			case t.IsListType():
				return tenon.Exactly(tenon.ListType(elem.Type())), nil
			case t.IsSetType():
				return tenon.Exactly(tenon.SetType(elem.Type())), nil
			}
			return tenon.Exactly(tenon.MapType(elem.Type())), nil
		}
		switch {
		case t.IsListType():
			return tenon.ListOf(elem), nil
		case t.IsSetType():
			return tenon.SetOf(elem), nil
		}
		return tenon.MapOf(elem), nil
	case t.IsTupleType():
		members := make([]tenon.Constraint, 0, len(t.TupleElementTypes()))
		types := make([]tenon.Type, 0, len(t.TupleElementTypes()))
		for _, e := range t.TupleElementTypes() {
			member, err := constraintFromCty(e, whole)
			if err != nil {
				return tenon.Constraint{}, err
			}
			members = append(members, member)
			if member.Kind() == tenon.ConstraintExactly {
				types = append(types, member.Type())
			}
		}
		if len(types) == len(members) {
			return tenon.Exactly(tenon.TupleType(types...)), nil
		}
		return tenon.TupleOf(members...), nil
	case t.IsObjectType():
		attrs := t.AttributeTypes()
		if err := tenon.CheckAttributeNames(slices.Sorted(maps.Keys(attrs))...); err != nil {
			return tenon.Constraint{}, fmt.Errorf("ctytenon: %#v has an object type whose attribute names tenon refuses: %w", whole, err)
		}
		fields := make(map[string]tenon.Field, len(attrs))
		for name, a := range attrs {
			attr, err := constraintFromCty(a, whole)
			if err != nil {
				return tenon.Constraint{}, err
			}
			fields[name] = tenon.Field{Constraint: attr, Required: !t.AttributeOptional(name)}
		}
		return tenon.ObjectWith(fields, false), nil
	}
	typ, err := typeFromCty(t, whole)
	if err != nil {
		return tenon.Constraint{}, err
	}
	return tenon.Exactly(typ), nil
}

// ConstraintToCty returns the cty type constraint of the tenon constraint c,
// as ConstraintFromCty gives the tenon constraint of a cty one: [tenon.Any] is
// cty.DynamicPseudoType, and Exactly a type is that type's cty type; ListOf,
// SetOf, MapOf and TupleOf are cty's list, set, map and tuple types of their
// parts' constraints; and ObjectWith is an object type with an attribute for
// each field, optional where the field is.
//
// An object type accepts, as the target of cty's conversion, an object with
// attributes it does not name, dropping them. A closed ObjectWith refuses such
// an object, so it crosses as a constraint that accepts more than it does,
// which crossing back gives as an open one; Exactly an object type crosses as
// the open ObjectWith of its attributes too. Every type c accepts, the cty
// constraint accepts.
//
// It fails where c holds a OneOf, which no cty type constraint says, or a
// capsule type. It panics on the zero Constraint, which is not a constraint.
func (b Bridge) ConstraintToCty(c tenon.Constraint) (cty.Type, error) {
	if c.IsZero() {
		usagePanic("ConstraintToCty called with the zero Constraint, which is not a constraint")
	}
	return constraintToCty(c, c)
}

// constraintToCty returns the cty type constraint of c, a part of whole, which
// the error of a part that does not cross names.
func constraintToCty(c, whole tenon.Constraint) (cty.Type, error) {
	switch c.Kind() {
	case tenon.ConstraintAny:
		return cty.DynamicPseudoType, nil
	case tenon.ConstraintExactly:
		return typeToCty(c.Type(), whole)
	case tenon.ConstraintListOf, tenon.ConstraintSetOf, tenon.ConstraintMapOf:
		elem, err := constraintToCty(c.Element(), whole)
		if err != nil {
			return cty.NilType, err
		}
		switch c.Kind() {
		case tenon.ConstraintListOf:
			return cty.List(elem), nil
		case tenon.ConstraintSetOf:
			return cty.Set(elem), nil
		}
		return cty.Map(elem), nil
	case tenon.ConstraintTupleOf:
		parts := c.Members()
		elems := make([]cty.Type, 0, len(parts))
		for _, m := range parts {
			elem, err := constraintToCty(m, whole)
			if err != nil {
				return cty.NilType, err
			}
			elems = append(elems, elem)
		}
		return cty.Tuple(elems), nil
	case tenon.ConstraintObjectWith:
		names := c.FieldNames()
		attrs := make(map[string]cty.Type, len(names))
		var optional []string
		for _, name := range names {
			f, _ := c.LookupField(name)
			attr, err := constraintToCty(f.Constraint, whole)
			if err != nil {
				return cty.NilType, err
			}
			attrs[name] = attr
			if !f.Required {
				optional = append(optional, name)
			}
		}
		return cty.ObjectWithOptionalAttrs(attrs, optional), nil
	}
	return cty.NilType, fmt.Errorf("ctytenon: %s holds %s, a OneOf, which no cty type constraint says", whole, c)
}
