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
// A value whose type satisfies c already converts to itself. Otherwise the
// conversion applies the one conversion that the value's type and c call for,
// and never a chain of them: a number converts to a string and a string to a
// bool, but a number does not convert to a bool. A conversion that p does not
// allow fails with code CodeConvertUnsafe.
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
// pending value where it would not; so does an unknown map converted to an
// object whose attributes its keys would settle.
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
	// Any value converts or fails as data, null included. The operation reads
	// within its operand only where the result is a set, which takes the marks
	// of every member placed into it; any other result leaves the marks of the
	// members on the members.
	operands: []operand{{constraint: Any(), nulls: true}},
	bind: func(o *op, param opParam) {
		cv := param.(conversion)
		o.operands[0].within = makesSet(cv.target)
		o.result = func([]Type) Constraint {
			if t, ok := soleType(cv.target); ok {
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
// Convert with: one of each kind of target, under each policy, and targets
// whose result type is fixed as well as ones where it is not.
func conversionSamples() []opParam {
	str, num := Type{stringType}, Type{numberType}
	return []opParam{
		conversion{Exactly(str), Unsafe},
		conversion{Exactly(num), Safe},
		conversion{ListOf(Any()), Safe},
		conversion{SetOf(Any()), Unsafe},
		conversion{MapOf(Exactly(num)), Safe},
		conversion{TupleOf(Any()), Unsafe},
		conversion{ObjectWith(map[string]Field{"a": Optional(Exactly(num))}, false), Unsafe},
		conversion{ObjectWith(map[string]Field{"k": Required(Exactly(num))}, true), Unsafe},
		conversion{OneOf(Exactly(num), ListOf(Exactly(str))), Safe},
		conversion{Any(), Safe},
	}
}

// makesSet reports whether every conversion to c that succeeds gives a set.
func makesSet(c Constraint) bool {
	if t, ok := soleType(c); ok {
		return t.t.kind == KindSet
	}
	return c.c.kind == ConstraintSetOf
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
		elems := make([]Type, len(d.members))
		for i, m := range d.members {
			t, ok := soleType(m)
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
			if !f.Required && admitsNone(f.Constraint) {
				// No attribute can be here, so the field leaves one choice.
				continue
			}
			t, ok := soleType(f.Constraint)
			if !ok || !f.Required {
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
