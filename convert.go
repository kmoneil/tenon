package tenon

import "strconv"

// Policy says which conversions a conversion may apply. Whether a string may
// stand where a number is expected is a question about a language, not about
// values, so tenon never converts on its own: a caller converting a value
// chooses a policy, and may choose differently for each conversion.
//
// The zero Policy is not a policy, and Convert panics on it.
type Policy uint8

const (
	// Safe applies only the conversions that are safe: those that change how
	// a value is held without changing what kind of thing it is, as a tuple
	// of strings becomes a list of strings.
	Safe Policy = iota + 1
	// Unsafe applies unsafe conversions as well: those that change what kind
	// of thing a value is, as a number becomes a string, and those that can
	// fail or lose something, as a list becomes a set and loses its order.
	Unsafe
)

// String returns "safe" or "unsafe".
func (p Policy) String() string {
	switch p {
	case Safe:
		return "safe"
	case Unsafe:
		return "unsafe"
	}
	return "Policy(" + strconv.Itoa(int(p)) + ")"
}

// Convert returns v converted to a type that satisfies c under the policy p,
// or an error value saying why it does not convert.
//
// An object converted to an ObjectWith constraint gains each absent optional
// attribute as null, where the field's constraint settles the attribute's
// type, and a value whose type satisfies c and holds every such attribute
// already converts to itself. Otherwise the conversion applies the one
// conversion that the value's type and c call for,
// and never a chain of them: a number converts to a string and a string to a
// bool, but a number does not convert to a bool. A conversion that p does not
// allow fails with code CodeConvertUnsafe. A constraint that admits exactly
// one type converts as Exactly of that type does, however it is written.
//
// The type of the result follows from the type of v and from c, not from what
// v holds, except where a map becomes an object, whose attributes are the
// map's keys. A conversion that fails for what v holds, as the string "x"
// converted to a number does, gives an error value, never a value of another
// type. A failure within a container is reported where it happens: a
// diagnostic for each member that fails, located by its path within v, with
// the member's own code.
//
// A null value converts to the null of the result type, and an unknown value
// to the unknown of it. A container converts member by member, so members that
// are not known stay unknown within the result. A pending value converts to
// an unknown value where its type would settle the result type, and to a
// pending value where it would not, keeping its own constraint when c is Any;
// an unknown map converted to an object whose attributes its keys would
// settle converts to a pending value too.
//
// The result carries the Propagate marks of v. A member converted within v
// carries its own Propagate marks, a member carried across unchanged keeps
// every mark it has, and a member placed into a set, whose members carry no
// marks, gives all of its marks, at every depth, to the set instead.
//
// Convert returns an error value if v is one. It panics if c is the zero
// Constraint or p is not Safe or Unsafe.
func Convert(v Value, c Constraint, p Policy) Value {
	c.data()
	if p != Safe && p != Unsafe {
		usagePanic("Convert called with %s, which is neither Safe nor Unsafe", p)
	}
	return convertOp.with(conversion{target: c, policy: p}).apply(v)
}

// conversion is the choice of parameters that a conversion is bound to.
type conversion struct {
	target Constraint
	policy Policy
}

func (cv conversion) String() string {
	return "(" + cv.target.String() + ", " + cv.policy.String() + ")"
}

var convertOp = register(&op{
	name: "Convert",
	// Any value converts or fails as data, null included. A conversion to a
	// set reads the members it places in the set and gives the set their
	// marks, which only the conversion knows, so it says so to the matrix
	// rather than asking the framework to read within; any other result
	// leaves the marks of the members on the members.
	operands: []operand{{constraint: Any(), nulls: true}},
	bind: func(o *op, param opParam) {
		cv := param.(conversion)
		o.operands[0].marksWithin = makesSet(cv.target)
		o.result = func([]Type) Constraint {
			if t, ok := resultType(cv.target); ok {
				return Exactly(t)
			}
			return cv.target
		}
		o.known = func(args []Value) Value {
			return convertTop(args[0], cv)
		}
		// Every conversion of an operand that is not known has an answer the
		// rules give, so the framework's own answer is never asked for.
		o.decided = func(args []Value) (Value, bool) {
			return convertTop(args[0], cv), true
		}
	},
	samples: conversionSamples(),
})

// conversionSamples returns the conversions that the operand matrix checks
// Convert with: one of each kind of target, under each policy, targets whose
// result type is fixed as well as ones where it is not, and conversions that
// parse a string, whose failures render it in their messages.
//
// The matrix expects the marks within an operand to reach the result for
// every operand or for none, as makesSet says, so no sample converts to a
// OneOf that gives a set for some values and not for others, nor to a set
// under Safe, which a list fails to convert to before it reads a member.
func conversionSamples() []opParam {
	str, num := Type{stringType}, Type{numberType}
	return []opParam{
		conversion{Exactly(str), Unsafe},
		conversion{Exactly(num), Safe},
		conversion{Exactly(num), Unsafe},
		conversion{Exactly(Type{boolType}), Unsafe},
		conversion{ListOf(Any()), Safe},
		conversion{SetOf(Any()), Unsafe},
		conversion{OneOf(SetOf(Any()), SetOf(Exactly(num))), Unsafe},
		conversion{MapOf(Exactly(num)), Safe},
		conversion{TupleOf(Any()), Unsafe},
		conversion{ObjectWith(map[string]Field{"a": Optional(Exactly(num))}, false), Unsafe},
		conversion{ObjectWith(map[string]Field{"k": Required(Exactly(num))}, true), Unsafe},
		conversion{ObjectWith(map[string]Field{"k": Optional(Exactly(str))}, true), Safe},
		conversion{OneOf(Exactly(num), ListOf(Exactly(str))), Safe},
		conversion{Any(), Safe},
	}
}

// makesSet reports whether c admits a type, and every conversion to c that
// succeeds gives a set. A OneOf does when every member that admits a type
// does.
func makesSet(c Constraint) bool {
	if admitsNone(c) {
		return false
	}
	if t, ok := resultType(c); ok {
		return t.t.kind == KindSet
	}
	switch c.c.kind {
	case ConstraintSetOf:
		return true
	case ConstraintOneOf:
		for _, m := range c.c.members {
			if !admitsNone(m) && !makesSet(m) {
				return false
			}
		}
		return true
	}
	return false
}

// soleType returns the type that c admits, and whether it admits exactly one.
func soleType(c Constraint) (Type, bool) {
	d := c.c
	switch d.kind {
	case ConstraintExactly:
		return d.typ, true
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		elem, ok := soleType(d.elem)
		if !ok {
			return Type{}, false
		}
		switch d.kind {
		case ConstraintListOf:
			return List(elem), true
		case ConstraintSetOf:
			return Set(elem), true
		}
		return Map(elem), true
	case ConstraintTupleOf:
		// Conversion asks this of every constraint it meets, so a tuple that
		// admits more than one type allocates nothing to say so.
		var elems []Type
		for i, m := range d.members {
			t, ok := soleType(m)
			if !ok {
				return Type{}, false
			}
			if elems == nil {
				elems = make([]Type, len(d.members))
			}
			elems[i] = t
		}
		return Tuple(elems...), true
	case ConstraintObjectWith:
		if !d.closed {
			return Type{}, false
		}
		// An optional field that some type fills leaves two types, one with
		// the attribute and one without, which is decided before anything is
		// built. One that no type fills leaves one choice: no attribute.
		for _, f := range d.fields {
			if !f.Required && !admitsNone(f.Constraint) {
				return Type{}, false
			}
		}
		attrs := make(map[string]Type, len(d.fields))
		for _, f := range d.fields {
			if !f.Required {
				continue
			}
			t, ok := soleType(f.Constraint)
			if !ok {
				return Type{}, false
			}
			attrs[f.name] = t
		}
		return Object(attrs), true
	case ConstraintOneOf:
		var sole Type
		for _, m := range d.members {
			if admitsNone(m) {
				continue
			}
			t, ok := soleType(m)
			if !ok || sole.t != nil && t != sole {
				return Type{}, false
			}
			sole = t
		}
		return sole, sole.t != nil
	}
	return Type{}, false
}

// resultType returns the type that c gives, and whether it gives one: the type
// every conversion to c that succeeds has (CV-027). It gives one wherever it
// admits exactly one, and also where a closed object constraint has optional
// fields whose types are settled, since a conversion adds those as null.
func resultType(c Constraint) (Type, bool) {
	d := c.c
	switch d.kind {
	case ConstraintExactly:
		return d.typ, true
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		elem, ok := resultType(d.elem)
		switch {
		case !ok:
			return Type{}, false
		case d.kind == ConstraintListOf:
			return List(elem), true
		case d.kind == ConstraintSetOf:
			return Set(elem), true
		}
		return Map(elem), true
	case ConstraintTupleOf:
		elems := make([]Type, len(d.members))
		for i, m := range d.members {
			t, ok := resultType(m)
			if !ok {
				return Type{}, false
			}
			elems[i] = t
		}
		return Tuple(elems...), true
	case ConstraintObjectWith:
		if !d.closed {
			return Type{}, false
		}
		attrs := make(map[string]Type, len(d.fields))
		for _, f := range d.fields {
			if admitsNone(f.Constraint) {
				if f.Required {
					return Type{}, false
				}
				continue
			}
			t, ok := resultType(f.Constraint)
			if !ok {
				return Type{}, false
			}
			attrs[f.name] = t
		}
		return Object(attrs), true
	case ConstraintOneOf:
		var given Type
		for _, m := range d.members {
			if admitsNone(m) {
				continue
			}
			t, ok := resultType(m)
			if !ok || given.t != nil && t != given {
				return Type{}, false
			}
			given = t
		}
		return given, given.t != nil
	}
	return Type{}, false
}

// fits reports whether a value of type t converts to c as itself: t satisfies
// c and already holds, at every depth, each attribute a conversion would add.
func fits(c Constraint, t Type) bool {
	return Satisfies(c, t) && complete(c, t)
}

// complete reports whether t, which satisfies c, holds at every depth each
// optional attribute whose field's constraint gives a type.
func complete(c Constraint, t Type) bool {
	d, td := c.c, t.t
	switch d.kind {
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return complete(d.elem, td.elem)
	case ConstraintTupleOf:
		for i, m := range d.members {
			if !complete(m, td.elems[i]) {
				return false
			}
		}
	case ConstraintObjectWith:
		for _, f := range d.fields {
			at, ok := td.attribute(f.name)
			if !ok {
				if _, gives := resultType(f.Constraint); gives {
					return false
				}
				continue
			}
			if !complete(f.Constraint, at) {
				return false
			}
		}
	case ConstraintOneOf:
		for _, m := range d.members {
			if Satisfies(m, t) && complete(m, t) {
				return true
			}
		}
		return false
	}
	return true
}

// admitsNone reports whether no type satisfies c.
func admitsNone(c Constraint) bool {
	d := c.c
	switch d.kind {
	case ConstraintListOf, ConstraintSetOf, ConstraintMapOf:
		return admitsNone(d.elem)
	case ConstraintTupleOf:
		for _, m := range d.members {
			if admitsNone(m) {
				return true
			}
		}
	case ConstraintObjectWith:
		for _, f := range d.fields {
			if f.Required && admitsNone(f.Constraint) {
				return true
			}
		}
	case ConstraintOneOf:
		for _, m := range d.members {
			if !admitsNone(m) {
				return false
			}
		}
		return true
	}
	return false
}
