package stdlib

import (
	"slices"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon"
)

// collection returns a parameter taking a collection or a structure, which
// a function reads as it stands: not known yet or pending, it answers from
// what the value already says (LB-012), and it carries the marks of what it
// reads rather than of everything within (LB-020).
func collection(name, description string) tenon.Param {
	return tenon.Param{
		Name: name, Description: description, Constraint: tenon.Any(),
		AllowUnknown: true, AllowPending: true, AllowMarked: true,
	}
}

// kindsOf returns the kinds of the types the constraint c admits.
func kindsOf(c tenon.Constraint) []tenon.Kind {
	switch c.Kind() {
	case tenon.ConstraintExactly:
		return []tenon.Kind{c.Type().Kind()}
	case tenon.ConstraintListOf:
		return []tenon.Kind{tenon.KindList}
	case tenon.ConstraintSetOf:
		return []tenon.Kind{tenon.KindSet}
	case tenon.ConstraintMapOf:
		return []tenon.Kind{tenon.KindMap}
	case tenon.ConstraintTupleOf:
		return []tenon.Kind{tenon.KindTuple}
	case tenon.ConstraintObjectWith:
		return []tenon.Kind{tenon.KindObject}
	case tenon.ConstraintOneOf:
		var out []tenon.Kind
		for _, m := range c.Members() {
			out = append(out, kindsOf(m)...)
		}
		return out
	}
	return []tenon.Kind{tenon.KindBool, tenon.KindNumber, tenon.KindString, tenon.KindList, tenon.KindSet,
		tenon.KindMap, tenon.KindTuple, tenon.KindObject, tenon.KindCapsule}
}

// kindOf returns the kind of the value v, unmarked, and false where v is
// pending, its type not settled.
func kindOf(v tenon.Value) (tenon.Kind, bool) {
	if v.IsPending() {
		return 0, false
	}
	return v.Type().Kind(), true
}

// mayBe reports whether v, unmarked, is or may turn out to be of one of the
// kinds given.
func mayBe(v tenon.Value, kinds ...tenon.Kind) bool {
	if k, ok := kindOf(v); ok {
		return slices.Contains(kinds, k)
	}
	for _, k := range kindsOf(v.Constraint()) {
		if slices.Contains(kinds, k) {
			return true
		}
	}
	return false
}

// wrongKind returns the refusal of argument i, of a kind the function does
// not take: what it takes, and what the argument is, its type withheld where
// a redacting mark withholds it.
func wrongKind(fn string, i int, v tenon.Value, takes string) error {
	u, marks := tenon.Unmark(v)
	what := "a pending value of " + "no type it takes"
	if !u.IsPending() {
		what = u.Type().String()
	}
	for _, m := range marks {
		if m.Redacting() {
			what = v.String()
			break
		}
	}
	return tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{
		Code:    tenon.CodeOperationWrongType,
		Message: fn + " takes " + takes + ", not " + what,
		Path:    argument(i),
	}))
}

// invalid returns the failure of argument i, outside what the function has
// a meaning for (LB-030).
func invalid(i int, message string) tenon.Value {
	return tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeFunctionInvalidArgument, Message: message, Path: argument(i)})
}

// lengthOf returns how many members the list, set, map, tuple or object c,
// unmarked, holds: known where it says, as a tuple's or an object's type
// does, and otherwise the least and greatest it may hold, ok false where
// nothing bounds it.
func lengthOf(c tenon.Value) (lo, hi int64, ok bool) {
	if k, resolved := kindOf(c); resolved && k == tenon.KindSet {
		// A set holding a member not known yet may hold fewer members than
		// it lists (EQ-042): tenon's Length says how many.
		n := tenon.Length(c)
		if n.IsKnown() {
			v, _ := n.AsInt64()
			return v, v, true
		}
		l, _, least := n.Range().NumberMin()
		h, _, bounded := n.Range().NumberMax()
		if least {
			lo, _ = l.AsInt64()
		}
		if bounded {
			hi, _ = h.AsInt64()
		}
		return lo, hi, bounded
	}
	if c.HasMembers() {
		n := int64(c.Len())
		return n, n, true
	}
	if k, resolved := kindOf(c); resolved {
		switch k {
		case tenon.KindTuple:
			n := int64(c.Type().TupleLength())
			return n, n, true
		case tenon.KindObject:
			n := int64(len(c.Type().AttributeNames()))
			return n, n, true
		}
	}
	if c.IsPending() {
		return 0, 0, false
	}
	r := c.Range()
	lo = r.LengthMin()
	hi, ok = r.LengthMax()
	return lo, hi, ok
}

// LengthFunc is how many members a value holds: the elements of a list, a
// set or a tuple, the entries of a map, the attributes of an object, and the
// grapheme clusters of a string. A list, set or map not known yet answers
// within the lengths its range allows, a tuple or an object the count its
// type gives, known or not. go-cty's takes no string and no object, where
// the consumers' own length takes both. Only the value's own marks reach the
// answer: a length reads how many members there are, not what they are.
var LengthFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Length",
	Description: "Returns the number of elements in the given collection or structure, or of characters in a string.",
	Params:      []tenon.Param{collection("value", "The collection, structure or string.")},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if u, _ := tenon.Unmark(args[0]); !mayBe(u, tenon.KindList, tenon.KindSet, tenon.KindMap, tenon.KindTuple, tenon.KindObject, tenon.KindString) {
			return tenon.Constraint{}, wrongKind("Length", 0, args[0], "a list, a set, a map, a tuple, an object or a string")
		}
		return number, nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		v := args[0]
		u, _ := tenon.Unmark(v)
		var r tenon.Value
		switch k, resolved := kindOf(u); {
		case resolved && (k == tenon.KindTuple || k == tenon.KindObject), !resolved && u.HasMembers():
			// A tuple's or an object's type says how many members it has,
			// as a pending one holding its members does; a set holding a
			// member not known yet may hold fewer than it lists (EQ-042),
			// which tenon's Length answers below.
			n, _, _ := lengthOf(u)
			r = tenon.NumberFromInt(n)
		case resolved || !mayBe(u, tenon.KindTuple, tenon.KindObject):
			// A list, a set, a map or a string, or a pending value that can
			// be nothing else, whose lengths tenon's Length answers.
			r = tenon.Length(u)
		default:
			r = tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull(), tenon.NumberMin(zero, true))
		}
		return tenon.WithMarks(r, propagating(v)...), nil
	},
})

// HasIndexFunc reports whether a list, a tuple, a map or an object has the
// key given: a whole number from zero to one less than its length for a list
// or a tuple, a key or an attribute name for a map or an object. A key of
// another kind is not one, and the answer is false. A list not known yet
// answers from the lengths its range allows, and a key not known yet from its
// range, where they settle it. Only the collection's own marks and the key's
// reach the answer: it reads the collection's length or its keys, not its
// members.
var HasIndexFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "HasIndex",
	Description: "Returns true if the given collection has the given key, or false otherwise.",
	Params: []tenon.Param{
		collection("collection", "The list, tuple, map or object."),
		{Name: "key", Description: "The key to look for.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true},
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if u, _ := tenon.Unmark(args[0]); !mayBe(u, tenon.KindList, tenon.KindTuple, tenon.KindMap, tenon.KindObject) {
			return tenon.Constraint{}, wrongKind("HasIndex", 0, args[0], "a list, a tuple, a map or an object")
		}
		return boolean, nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		c, _ := tenon.Unmark(args[0])
		return tenon.WithMarks(hasIndex(c, args[1]), propagating(args[0])...), nil
	},
})

// hasIndex answers HasIndex of the unmarked collection c.
func hasIndex(c, key tenon.Value) tenon.Value {
	unknown := tenon.Unknown(tenon.BoolType())
	k, resolved := kindOf(c)
	if !resolved && !c.HasMembers() {
		return unknown
	}
	if c.HasMembers() && !resolved {
		// A pending tuple or object holding its members.
		if c.Constraint().Kind() == tenon.ConstraintObjectWith {
			k = tenon.KindObject
		} else {
			k = tenon.KindTuple
		}
	}
	switch k {
	case tenon.KindList, tenon.KindTuple:
		if !mayBe(key, tenon.KindNumber) {
			return tenon.Bool(false)
		}
		if key.IsPending() {
			return unknown
		}
		lo, hi, bounded := lengthOf(c)
		return indexWithin(key, lo, hi, bounded)
	default:
		if !mayBe(key, tenon.KindString) {
			return tenon.Bool(false)
		}
		if !key.IsKnown() {
			return unknown
		}
		name := key.AsString()
		if k == tenon.KindObject {
			if c.HasMembers() {
				_, ok := c.LookupAttribute(name)
				return tenon.Bool(ok)
			}
			return tenon.Bool(c.Type().HasAttribute(name))
		}
		if !c.HasMembers() {
			return unknown
		}
		_, ok := c.LookupMapElement(name)
		return tenon.Bool(ok)
	}
}

// indexWithin answers whether the number key is a whole number from zero to
// one less than a length between lo and hi (unbounded above where bounded is
// false): known where every length and every key the ranges allow agree.
func indexWithin(key tenon.Value, lo, hi int64, bounded bool) tenon.Value {
	unknown := tenon.Unknown(tenon.BoolType())
	if key.IsKnown() {
		if fractional(key) || less(key, zero) {
			return tenon.Bool(false)
		}
		switch {
		case less(key, tenon.NumberFromInt(lo)):
			return tenon.Bool(true)
		case bounded && !less(key, tenon.NumberFromInt(hi)):
			return tenon.Bool(false)
		}
		return unknown
	}
	// A key not known yet: false where its range lies wholly below zero or
	// at or above every length allowed.
	klo, khi := ends(key)
	switch {
	case khi.ok && (less(khi.v, zero) || khi.v.Equal(zero) && !khi.inclusive):
		return tenon.Bool(false)
	case bounded && klo.ok && !less(klo.v, tenon.NumberFromInt(hi)):
		return tenon.Bool(false)
	}
	return unknown
}

// member returns the member of the unmarked collection c at the position or
// name key, known, and whether it has one; a position is read by floor
// modulus of the length where wrap is set.
func member(c, key tenon.Value) (tenon.Value, bool) {
	k, resolved := kindOf(c)
	if !resolved {
		if c.Constraint().Kind() == tenon.ConstraintObjectWith {
			k = tenon.KindObject
		} else {
			k = tenon.KindTuple
		}
	}
	switch k {
	case tenon.KindList, tenon.KindTuple:
		i, ok := key.AsInt64()
		if !ok || i < 0 || i >= int64(c.Len()) {
			return tenon.Value{}, false
		}
		return c.Index(int(i)), true
	case tenon.KindMap:
		return c.LookupMapElement(key.AsString())
	}
	return c.LookupAttribute(key.AsString())
}

// distinct returns the constraint of one of the types given: Exactly the
// one where they agree, and OneOf the distinct ones otherwise.
func distinct(types []tenon.Type) tenon.Constraint {
	var cs []tenon.Constraint
	var seen []tenon.Type
	for _, t := range types {
		if !slices.ContainsFunc(seen, t.Equal) {
			seen = append(seen, t)
			cs = append(cs, tenon.Exactly(t))
		}
	}
	if len(cs) == 1 {
		return cs[0]
	}
	return tenon.OneOf(cs...)
}

// memberTypes returns the types of the members of the resolved tuple or
// object type t.
func memberTypes(t tenon.Type) []tenon.Type {
	if t.Kind() == tenon.KindTuple {
		return t.TupleElementTypes()
	}
	var out []tenon.Type
	for _, name := range t.AttributeNames() {
		out = append(out, t.AttributeType(name))
	}
	return out
}

// IndexFunc is the member of a list, a tuple, a map or an object at a key:
// the language's coll[key] as a function. A list's or a tuple's key is a
// whole number from zero to one less than its length, a map's or an object's
// a string it holds; another key fails with tenon.CodeFunctionInvalidArgument,
// and a key of the wrong kind with tenon.CodeOperationWrongType, both at the
// key. An index into a list known to be too short fails now. Not known yet,
// the answer is the unknown of the member's type, which may be null, as a
// member may be: for a tuple or an object and a key not known yet, one of
// its members' types. Only the collection's own marks, the key's and the
// member's reach the answer, not the other members'.
var IndexFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Index",
	Description: "Returns the element of the given collection at the given key.",
	Params: []tenon.Param{
		collection("collection", "The list, tuple, map or object."),
		{Name: "key", Description: "The key of the element.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true},
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		return indexResult("Index", args, false)
	},
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return indexed("Index", args, rc, false), nil
	},
})

// indexResult derives the result of Index or, where wrap is set, Element.
func indexResult(fn string, args []tenon.Value, wrap bool) (tenon.Constraint, error) {
	c, _ := tenon.Unmark(args[0])
	key := args[1]
	takes := "a list, a tuple, a map or an object"
	kinds := []tenon.Kind{tenon.KindList, tenon.KindTuple, tenon.KindMap, tenon.KindObject}
	if wrap {
		takes, kinds = "a list or a tuple", kinds[:2]
	}
	if !mayBe(c, kinds...) {
		return tenon.Constraint{}, wrongKind(fn, 0, args[0], takes)
	}
	if c.IsPending() {
		if el := c.Constraint(); el.Kind() == tenon.ConstraintListOf || el.Kind() == tenon.ConstraintMapOf {
			return el.Element(), nil
		}
		return tenon.Any(), nil
	}
	t := c.Type()
	numeric := t.Kind() == tenon.KindList || t.Kind() == tenon.KindTuple
	if !key.IsPending() {
		if want := map[bool]tenon.Kind{true: tenon.KindNumber, false: tenon.KindString}[numeric]; key.Type().Kind() != want {
			return tenon.Constraint{}, wrongKind(fn, 1, key, map[bool]string{true: "a number", false: "a string"}[numeric]+" as the key of a "+strings.ToLower(t.Kind().String()))
		}
	}
	switch t.Kind() {
	case tenon.KindList, tenon.KindMap:
		return tenon.Exactly(t.ElementType()), nil
	}
	types := memberTypes(t)
	if len(types) == 0 {
		return tenon.Constraint{}, tenon.NewError(invalid(0, fn+" has no member to give of an empty "+strings.ToLower(t.Kind().String())))
	}
	if !key.IsKnown() {
		return distinct(types), nil
	}
	if t.Kind() == tenon.KindObject {
		at, ok := t.LookupAttributeType(key.AsString())
		if !ok {
			return tenon.Constraint{}, tenon.NewError(invalid(1, fn+": the object has no attribute "+key.String()))
		}
		return tenon.Exactly(at), nil
	}
	i, ok := position(key, int64(len(types)), wrap)
	if !ok {
		return tenon.Constraint{}, tenon.NewError(invalid(1, outOfRange(fn, key, int64(len(types)))))
	}
	return tenon.Exactly(types[i]), nil
}

// position returns the position the number key names in a sequence of n
// members, n positive: the key itself where it is a whole number from zero
// to n - 1, or, where wrap is set, any whole number taken by floor modulus.
func position(key tenon.Value, n int64, wrap bool) (int64, bool) {
	if fractional(key) {
		return 0, false
	}
	if wrap {
		r := tenon.Mod(key, tenon.NumberFromInt(n))
		if less(r, zero) {
			r = tenon.Add(r, tenon.NumberFromInt(n))
		}
		i, _ := r.AsInt64()
		return i, true
	}
	i, ok := key.AsInt64()
	return i, ok && i >= 0 && i < n
}

// outOfRange returns the message of a key that names no member of a
// sequence of n members.
func outOfRange(fn string, key tenon.Value, n int64) string {
	switch {
	case fractional(key):
		return fn + ": the key " + key.String() + " is not a whole number"
	case n == 0:
		return fn + ": the key " + key.String() + " names no member of an empty sequence"
	}
	return fn + ": the key " + key.String() + " is outside 0 to " + tenon.NumberFromInt(n-1).String()
}

// indexed answers Index or, where wrap is set, Element.
func indexed(fn string, args []tenon.Value, rc tenon.Constraint, wrap bool) tenon.Value {
	c, _ := tenon.Unmark(args[0])
	key := args[1]
	marks := propagating(args[0])
	answer := func(v tenon.Value) tenon.Value { return tenon.WithMarks(v, marks...) }
	if !c.HasMembers() {
		// A list, a map or a pending value not known yet: a list known to
		// be shorter than every key the key allows fails now.
		if k, _ := kindOf(c); k == tenon.KindList && key.IsKnown() {
			_, hi, bounded := lengthOf(c)
			switch {
			case wrap && bounded && hi == 0:
				return answer(invalid(0, fn+" has no element to give of an empty list"))
			case !wrap && fractional(key):
				return answer(invalid(1, fn+": the key "+key.String()+" is not a whole number"))
			case !wrap && less(key, zero):
				return answer(invalid(1, fn+": the key "+key.String()+" is negative, and names no element"))
			case !wrap && bounded && !less(key, tenon.NumberFromInt(hi)):
				return answer(invalid(1, fn+": the key "+key.String()+" is at or past the most elements the list holds, "+tenon.NumberFromInt(hi).String()))
			}
		}
		if wrap && fractional0(key) {
			return answer(invalid(1, fn+": the index "+key.String()+" is not a whole number"))
		}
		return answer(unknownOf(rc))
	}
	if !key.IsKnown() {
		return answer(unknownOf(rc))
	}
	n := int64(c.Len())
	if k, _ := kindOf(c); k == tenon.KindList || k == tenon.KindTuple || c.IsPending() && c.Constraint().Kind() == tenon.ConstraintTupleOf {
		if wrap && n == 0 {
			return answer(invalid(0, fn+" has no element to give of an empty list"))
		}
		i, ok := position(key, n, wrap)
		if !ok {
			return answer(invalid(1, outOfRange(fn, key, n)))
		}
		return answer(c.Index(int(i)))
	}
	m, ok := member(c, key)
	if !ok {
		return answer(invalid(1, fn+": no member is named "+key.String()))
	}
	return answer(m)
}

// fractional0 reports whether the value key is a known number that is not
// a whole number.
func fractional0(key tenon.Value) bool { return key.IsKnown() && fractional(key) }

// ElementFunc is the element of a list or a tuple at an index taken by floor
// modulus of its length, so that an index past the end wraps around and a
// negative one counts from the end: element([a, b, c], 4) is b and
// element([a, b, c], -1) is c. The index is a whole number of any magnitude;
// one that is not fails with tenon.CodeFunctionInvalidArgument, and so does
// an empty list, known to be empty or not yet known but bounded to none.
// Not known yet, the answer is the unknown of the element's type, which may
// be null; for a tuple and an index not known yet, one of its elements'
// types. Only the list's own marks, the index's and the element's reach the
// answer.
var ElementFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Element",
	Description: "Retrieves a single element from a list, wrapping its index around the list's length.",
	Params: []tenon.Param{
		collection("list", "The list or tuple."),
		{Name: "index", Description: "The index, taken modulo the length.", Constraint: number, AllowUnknown: true},
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		return indexResult("Element", args, true)
	},
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return indexed("Element", args, rc, true), nil
	},
})

// stringValues returns the String values of names.
func stringValues(names []string) []tenon.Value {
	out := make([]tenon.Value, len(names))
	for i, n := range names {
		out[i] = tenon.String(n)
	}
	return out
}

// attributeNames returns the names of the unmarked object c, a pending one
// holding its attributes included, in order, and false where they are not
// settled.
func attributeNames(c tenon.Value) ([]string, bool) {
	if c.IsPending() {
		if !c.HasMembers() || c.Constraint().Kind() != tenon.ConstraintObjectWith {
			return nil, false
		}
		var names []string
		for name := range c.Attributes() {
			names = append(names, name)
		}
		return names, true
	}
	return c.Type().AttributeNames(), true
}

// isObject reports whether the unmarked value c is an object, or a pending
// one holding its attributes.
func isObject(c tenon.Value) bool {
	if c.IsPending() {
		return c.HasMembers() && c.Constraint().Kind() == tenon.ConstraintObjectWith
	}
	return c.Type().Kind() == tenon.KindObject
}

// KeysFunc is the keys of a map, or the attribute names of an object, in
// the canonical order of strings (EQ-045): a list for a map, a tuple for an
// object, whose names its type gives whether or not the object is known. A
// map not known yet answers the unknown list of its lengths. Only the
// value's own marks reach the answer.
var KeysFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Keys",
	Description: "Returns a list of the keys of the given map, or the attribute names of the given object, in lexicographic order.",
	Params:      []tenon.Param{collection("inputMap", "The map or object.")},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		c, _ := tenon.Unmark(args[0])
		if !mayBe(c, tenon.KindMap, tenon.KindObject) {
			return tenon.Constraint{}, wrongKind("Keys", 0, args[0], "a map or an object")
		}
		if names, ok := objectNames(c); ok {
			types := make([]tenon.Type, len(names))
			for i := range types {
				types[i] = tenon.StringType()
			}
			return tenon.Exactly(tenon.TupleType(types...)), nil
		}
		if !c.IsPending() || c.Constraint().Kind() == tenon.ConstraintMapOf {
			return tenon.Exactly(tenon.ListType(tenon.StringType())), nil
		}
		return tenon.Any(), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		c, _ := tenon.Unmark(args[0])
		if names, ok := objectNames(c); ok {
			return marked(tenon.Tuple(stringValues(names)...), args[0]), nil
		}
		if c.HasMembers() {
			return marked(tenon.List(tenon.StringType(), stringValues(c.MapKeys())...), args[0]), nil
		}
		if rc.Kind() == tenon.ConstraintExactly {
			lo, hi, bounded := lengthOf(c)
			return marked(unknownList(tenon.StringType(), lo, hi, bounded), args[0]), nil
		}
		return marked(unknownOf(rc), args[0]), nil
	},
})

// objectNames returns the attribute names of the unmarked value c where it
// is an object, or a pending one holding its attributes.
func objectNames(c tenon.Value) ([]string, bool) {
	if !isObject(c) {
		return nil, false
	}
	return attributeNames(c)
}

// ValuesFunc is the values of a map, or the attributes of an object, in the
// order KeysFunc gives their keys: a list for a map, a tuple for an object.
// A map not known yet answers the unknown list of its lengths, and an object
// not known yet the unknown tuple of its attribute types. Only the value's
// own marks reach the answer; the values keep their own.
var ValuesFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Values",
	Description: "Returns a list of the values of the given map, or the attributes of the given object, in the order of their keys.",
	Params:      []tenon.Param{collection("mapping", "The map or object.")},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		c, _ := tenon.Unmark(args[0])
		if !mayBe(c, tenon.KindMap, tenon.KindObject) {
			return tenon.Constraint{}, wrongKind("Values", 0, args[0], "a map or an object")
		}
		switch {
		case c.IsPending() && isObject(c):
			var cs []tenon.Constraint
			for _, v := range c.Attributes() {
				u, _ := tenon.Unmark(v)
				cs = append(cs, typeOf(u))
			}
			return tenon.TupleOf(cs...), nil
		case c.IsPending() && c.Constraint().Kind() == tenon.ConstraintMapOf:
			return tenon.ListOf(c.Constraint().Element()), nil
		case c.IsPending():
			return tenon.Any(), nil
		case c.Type().Kind() == tenon.KindObject:
			return tenon.Exactly(tenon.TupleType(memberTypes(c.Type())...)), nil
		}
		return tenon.Exactly(tenon.ListType(c.Type().ElementType())), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		c, _ := tenon.Unmark(args[0])
		if !c.HasMembers() {
			if k, ok := kindOf(c); ok && k == tenon.KindMap {
				lo, hi, bounded := lengthOf(c)
				return marked(unknownList(c.Type().ElementType(), lo, hi, bounded), args[0]), nil
			}
			return marked(unknownOf(rc), args[0]), nil
		}
		var vals []tenon.Value
		if isObject(c) {
			for _, v := range c.Attributes() {
				vals = append(vals, v)
			}
			return marked(tenon.Tuple(vals...), args[0]), nil
		}
		for _, v := range c.MapEntries() {
			vals = append(vals, v)
		}
		return marked(tenon.List(c.Type().ElementType(), vals...), args[0]), nil
	},
})

// zipKeys reads the keys argument of Zipmap: the known keys in order, each
// unmarked, the marks of each key and of the list, and whether every key is
// known. A known null key fails, located at it.
func zipKeys(keys tenon.Value) (names []string, marks []tenon.Mark, all bool, failure tenon.Value) {
	l, _ := tenon.Unmark(keys)
	marks = propagating(keys)
	if !l.HasMembers() {
		return nil, marks, false, tenon.Value{}
	}
	all = true
	for i, k := range l.Elements() {
		u, _ := tenon.Unmark(k)
		marks = append(marks, propagating(k)...)
		switch {
		case !u.IsKnown():
			all = false
		case u.IsNull():
			return nil, marks, false, tenon.ErrorVal(tenon.Diagnostic{
				Code:    tenon.CodeOperationNullOperand,
				Message: "Zipmap: key " + strconv.Itoa(i) + " is null, and names no entry",
				Path:    argument(0).Index(tenon.NumberFromInt(int64(i))),
			})
		default:
			names = append(names, u.AsString())
		}
	}
	return names, marks, all, tenon.Value{}
}

// ZipmapFunc builds a map from a list of keys and a list of values, or an
// object from a list of keys and a tuple of values, the key at each position
// naming the value at it, a later key naming the same entry as an earlier
// one winning. A null key fails at it, and lists of different lengths fail,
// now where the lengths already differ whatever the members not known yet
// turn out to be. Not known yet, a map's answer is the unknown map between
// the distinct keys known and the number of keys. The marks of both lists
// and of every key reach the answer; the values keep their own.
var ZipmapFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Zipmap",
	Description: "Constructs a map from a list of keys and a corresponding list of values.",
	Params: []tenon.Param{
		{Name: "keys", Description: "The keys.", Constraint: tenon.ListOf(tenon.Exactly(tenon.StringType())), AllowUnknown: true, AllowMarked: true},
		collection("values", "The values: a list, or a tuple to build an object."),
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		vals, _ := tenon.Unmark(args[1])
		if !mayBe(vals, tenon.KindList, tenon.KindTuple) {
			return tenon.Constraint{}, wrongKind("Zipmap", 1, args[1], "a list or a tuple of values")
		}
		names, _, all, failure := zipKeys(args[0])
		if !failure.IsZero() {
			return tenon.Constraint{}, tenon.NewError(failure)
		}
		if k, ok := kindOf(vals); ok && k == tenon.KindList {
			return tenon.Exactly(tenon.MapType(vals.Type().ElementType())), nil
		}
		if vals.IsPending() {
			if c := vals.Constraint(); c.Kind() == tenon.ConstraintListOf {
				return tenon.MapOf(c.Element()), nil
			}
		}
		ts, settled := sequenceTypes(vals)
		if !all || !settled || vals.IsPending() {
			return tenon.Any(), nil
		}
		if len(ts) != len(names) {
			return tenon.Constraint{}, tenon.NewError(lengthMismatch(len(names), len(ts)))
		}
		attrs := map[string]tenon.Type{}
		for i, name := range names {
			attrs[tenon.String(name).AsString()] = ts[i]
		}
		return tenon.Exactly(tenon.ObjectType(attrs)), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		names, marks, all, failure := zipKeys(args[0])
		if !failure.IsZero() {
			return tenon.WithMarks(failure, marks...), nil
		}
		marks = append(marks, propagating(args[1])...)
		vals, _ := tenon.Unmark(args[1])
		keys, _ := tenon.Unmark(args[0])
		klo, khi, kbounded := lengthOf(keys)
		vlo, vhi, vbounded := lengthOf(vals)
		if kbounded && vlo > khi || vbounded && klo > vhi {
			return tenon.WithMarks(lengthMismatch(int(klo), int(vlo)), marks...), nil
		}
		if !all || !vals.HasMembers() {
			if rc.Kind() == tenon.ConstraintExactly && rc.Type().Kind() == tenon.KindMap {
				distinctKeys := map[string]bool{}
				for _, n := range names {
					distinctKeys[n] = true
				}
				least := int64(len(distinctKeys))
				if least == 0 && klo > 0 {
					least = 1
				}
				ns := []tenon.Narrowing{tenon.NotNull(), tenon.LengthMin(least)}
				if kbounded {
					ns = append(ns, tenon.LengthMax(khi))
				}
				return tenon.WithMarks(tenon.Narrow(tenon.Unknown(rc.Type()), ns...), marks...), nil
			}
			return tenon.WithMarks(unknownOf(rc), marks...), nil
		}
		if len(names) != vals.Len() {
			return tenon.WithMarks(lengthMismatch(len(names), vals.Len()), marks...), nil
		}
		entries := map[string]tenon.Value{}
		for i, name := range names {
			entries[tenon.String(name).AsString()] = vals.Index(i)
		}
		if k, _ := kindOf(vals); k == tenon.KindList {
			return tenon.WithMarks(tenon.Map(vals.Type().ElementType(), entries), marks...), nil
		}
		return tenon.WithMarks(tenon.Object(entries), marks...), nil
	},
})

// lengthMismatch returns the failure of Zipmap's lists of different lengths.
func lengthMismatch(keys, values int) tenon.Value {
	return invalid(1, "Zipmap has "+strconv.Itoa(keys)+" keys and "+strconv.Itoa(values)+" values, which must be as many")
}

// LookupFunc is the member of a map or an object at a key, or, where it has
// none, the default converted to the member's type; without a default, a
// missing key fails with tenon.CodeFunctionInvalidArgument at it. The
// default is optional and may be null, as the consumers' own lookup has it,
// and it is read only where the key is missing, so a default not known yet
// leaves a found member known. A map holding members answers a known key
// whatever its other members are. Not known yet, the answer is the unknown
// of the result, which may be null. The marks of the map and the key reach
// the answer with the member's own, and the default's only where it is the
// answer.
var LookupFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Lookup",
	Description: "Returns the value of the given key in a map or an object, or the default where it has none.",
	Params: []tenon.Param{
		collection("inputMap", "The map or object."),
		{Name: "key", Description: "The key.", Constraint: tenon.Exactly(tenon.StringType()), AllowUnknown: true, AllowMarked: true},
	},
	VarParam: &tenon.Param{Name: "default", Description: "The value where the key is missing; optional.", Constraint: tenon.Any(), AllowNull: true, AllowUnknown: true, AllowPending: true, AllowMarked: true},
	ResultOf: func(args []tenon.Value, p tenon.Policy) (tenon.Constraint, error) {
		if len(args) > 3 {
			return tenon.Constraint{}, tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{
				Code:    tenon.CodeFunctionArity,
				Message: "Lookup takes 2 or 3 arguments, and " + strconv.Itoa(len(args)) + " were given",
			}))
		}
		c, _ := tenon.Unmark(args[0])
		key, _ := tenon.Unmark(args[1])
		if !mayBe(c, tenon.KindMap, tenon.KindObject) {
			return tenon.Constraint{}, wrongKind("Lookup", 0, args[0], "a map or an object")
		}
		var def tenon.Value
		if len(args) == 3 {
			def, _ = tenon.Unmark(args[2])
		}
		if c.IsPending() && !isObject(c) {
			if k := c.Constraint(); k.Kind() == tenon.ConstraintMapOf {
				return k.Element(), nil
			}
			return tenon.Any(), nil
		}
		if !isObject(c) {
			el := tenon.Exactly(c.Type().ElementType())
			if !def.IsZero() && !def.IsPending() {
				if d := tenon.Convert(def, el, p); d.IsError() {
					return tenon.Constraint{}, tenon.NewError(at(2, d))
				}
			}
			return el, nil
		}
		names, _ := attributeNames(c)
		types := map[string]tenon.Constraint{}
		for _, name := range names {
			if c.IsPending() {
				v, _ := c.LookupAttribute(name)
				u, _ := tenon.Unmark(v)
				types[name] = typeOf(u)
			} else {
				types[name] = tenon.Exactly(c.Type().AttributeType(name))
			}
		}
		if key.IsKnown() {
			if t, ok := types[key.AsString()]; ok {
				return t, nil
			}
			if def.IsZero() {
				return tenon.Constraint{}, tenon.NewError(invalid(1, "Lookup: the object has no attribute "+key.String()+", and no default was given"))
			}
			return typeOf(def), nil
		}
		var cs []tenon.Constraint
		for _, name := range names {
			if !slices.ContainsFunc(cs, types[name].Equal) {
				cs = append(cs, types[name])
			}
		}
		if !def.IsZero() && !slices.ContainsFunc(cs, typeOf(def).Equal) {
			cs = append(cs, typeOf(def))
		}
		switch len(cs) {
		case 0:
			return tenon.Any(), nil
		case 1:
			return cs[0], nil
		}
		return tenon.OneOf(cs...), nil
	},
	Impl: func(args []tenon.Value, rc tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
		c, _ := tenon.Unmark(args[0])
		key, _ := tenon.Unmark(args[1])
		marks := propagating(args[0], args[1])
		answer := func(v tenon.Value) tenon.Value { return tenon.WithMarks(v, marks...) }
		if !key.IsKnown() {
			return answer(unknownOf(rc)), nil
		}
		name := key.AsString()
		var found tenon.Value
		var ok, settled bool
		switch {
		case isObject(c) && c.HasMembers():
			found, ok = c.LookupAttribute(name)
			settled = true
		case isObject(c):
			// An object not known yet has the attributes its type names.
			if c.Type().HasAttribute(name) {
				return answer(unknownOf(rc)), nil
			}
			settled = true
		case c.HasMembers():
			found, ok = c.LookupMapElement(name)
			settled = true
		}
		switch {
		case ok:
			return answer(found), nil
		case !settled:
			return answer(unknownOf(rc)), nil
		case len(args) < 3:
			return answer(invalid(1, "Lookup: no member is named "+key.String()+", and no default was given")), nil
		}
		d := at(2, tenon.Convert(args[2], rc, p))
		return tenon.WithMarks(d, marks...), nil
	},
})
