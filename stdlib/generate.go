package stdlib

import (
	"strconv"

	"github.com/kmoneil/tenon"
)

// rangeBound is the most elements Range makes (LB-031), go-cty's own.
const rangeBound = 1024

// productBound is the most tuples SetProduct makes (LB-031).
const productBound = 1 << 20

// tooLarge returns the failure of argument i, driving a result past the
// bound of what it names (LB-031).
func tooLarge(i int, message string) tenon.Value {
	return tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeFunctionTooLarge, Message: message, Path: argument(i)})
}

// certain reports whether the Bool b is known true.
func certain(b tenon.Value) bool { return b.IsKnown() && b.AsBool() }

// rangeArgs is what Range's arguments say: the start, the limit and the
// step, the step's argument -1 where it is implied, and the limit's
// argument.
type rangeArgs struct {
	start, limit, step tenon.Value
	stepAt, limitAt    int
}

// rangeOf reads Range's one, two or three arguments: the limit alone,
// from zero; the start and the limit; or the start, the limit and the
// step. An implied step is 1, or -1 where the limit is less than the
// start, and not known where that is not.
func rangeOf(args []tenon.Value) rangeArgs {
	r := rangeArgs{start: zero, stepAt: -1}
	switch len(args) {
	case 1:
		r.limit = args[0]
	default:
		r.start, r.limit, r.limitAt = args[0], args[1], 1
	}
	if len(args) == 3 {
		r.step, r.stepAt = args[2], 2
		return r
	}
	switch down := tenon.LessThan(r.limit, r.start); {
	case !down.IsKnown():
		r.step = tenon.Unknown(tenon.NumberType())
	case down.AsBool():
		r.step = tenon.NumberFromInt(-1)
	default:
		r.step = tenon.NumberFromInt(1)
	}
	return r
}

// failure returns the failure the arguments settle, whatever is not known
// yet: an explicit step of zero, at the step; a step whose sign leads away
// from the limit, at the limit; and more elements than rangeBound, at the
// limit, decided before any is made.
func (r rangeArgs) failure() (tenon.Value, bool) {
	if r.stepAt >= 0 && certain(tenon.Equals(r.step, zero)) {
		return invalid(r.stepAt, "Range: the step is zero, which never reaches the limit"), true
	}
	up, down := tenon.LessThan(zero, r.step), tenon.LessThan(r.step, zero)
	if r.stepAt >= 0 {
		switch {
		case certain(up) && certain(tenon.LessThan(r.limit, r.start)):
			return invalid(r.limitAt, "Range: the limit is less than the start, and the step is positive"), true
		case certain(down) && certain(tenon.LessThan(r.start, r.limit)):
			return invalid(r.limitAt, "Range: the limit is greater than the start, and the step is negative"), true
		}
	}
	// More than rangeBound elements is a span longer than rangeBound steps,
	// asked without dividing, so a step not terminating as a quotient does
	// not round the count.
	span := tenon.Sub(r.limit, r.start)
	if span.IsError() {
		return span, true
	}
	upSteps, downSteps := tenon.NumberFromInt(rangeBound), tenon.NumberFromInt(-rangeBound)
	if r.stepAt < 0 {
		// The implied step is one either way, whichever way that is.
		up, down = tenon.LessThan(r.start, r.limit), tenon.LessThan(r.limit, r.start)
	} else {
		steps := tenon.Mul(upSteps, r.step)
		if steps.IsError() {
			return steps, true
		}
		upSteps, downSteps = steps, steps
	}
	if certain(up) && certain(tenon.LessThan(upSteps, span)) || certain(down) && certain(tenon.LessThan(span, downSteps)) {
		return tooLarge(r.limitAt, "Range: from "+r.start.String()+" to "+r.limit.String()+" by "+r.step.String()+
			" is more than "+strconv.Itoa(rangeBound)+" elements, the most it makes"), true
	}
	return tenon.Value{}, false
}

// RangeFunc is the list of numbers from a start, by a step, up to and not
// including a limit: Range(limit) from zero, Range(start, limit), and
// Range(start, limit, step), the step 1, or -1 where the limit is less than
// the start, where it is not given. Each element is start + i*step,
// exactly, so a step such as 0.1 does not drift, and the count is exact. A
// step of zero fails at the step, and a step leading away from the limit at
// the limit, both with tenon.CodeFunctionInvalidArgument; more than 1024
// elements fails at the limit with tenon.CodeFunctionTooLarge before any is
// made. Each failure is decided as soon as the known arguments settle it.
// Not known yet, the answer is the unknown list of at most 1024 numbers.
var RangeFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Range",
	Description: "Returns a list of numbers spread evenly over a particular range.",
	VarParam:    &tenon.Param{Name: "params", Description: "The limit; the start and the limit; or the start, the limit and the step.", Constraint: number, AllowUnknown: true},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if len(args) < 1 || len(args) > 3 {
			return tenon.Constraint{}, tenon.NewError(tenon.ErrorVal(tenon.Diagnostic{
				Code:    tenon.CodeFunctionArity,
				Message: "Range takes 1, 2 or 3 arguments, and " + strconv.Itoa(len(args)) + " were given",
			}))
		}
		return tenon.Exactly(tenon.ListType(tenon.NumberType())), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		r := rangeOf(args)
		if f, ok := r.failure(); ok {
			return f, nil
		}
		if !r.start.IsKnown() || !r.limit.IsKnown() || !r.step.IsKnown() {
			return unknownList(tenon.NumberType(), 0, rangeBound, true), nil
		}
		up := less(zero, r.step)
		var out []tenon.Value
		for i := int64(0); ; i++ {
			v := tenon.Add(r.start, tenon.Mul(tenon.NumberFromInt(i), r.step))
			if up && !less(v, r.limit) || !up && !less(r.limit, v) {
				break
			}
			out = append(out, v)
		}
		return tenon.List(tenon.NumberType(), out...), nil
	},
})

// productArg is what one argument of SetProduct, unmarked, says: its
// members, where it holds them, read as a list's or a set's, a tuple's
// converted to the list its element types unify to; the constraint of its
// members' type; whether it is a set, and whether that is settled, as a
// pending argument's may not be; and whether it is the empty tuple, which
// has no element type.
type productArg struct {
	c       tenon.Value
	members []tenon.Value
	held    bool
	elem    tenon.Constraint
	set     bool
	settled bool
	empty   bool
}

// productArgOf reads argument i of SetProduct under the policy p.
func productArgOf(i int, v tenon.Value, p tenon.Policy) (productArg, error) {
	c, _ := tenon.Unmark(v)
	if !mayBe(c, tenon.KindSet, tenon.KindList, tenon.KindTuple) {
		return productArg{}, wrongKind("SetProduct", i, v, "sets, lists and tuples")
	}
	a := productArg{c: c, settled: true}
	if c.IsPending() {
		switch k := c.Constraint(); k.Kind() {
		case tenon.ConstraintSetOf:
			a.elem, a.set = k.Element(), true
		case tenon.ConstraintListOf:
			a.elem = k.Element()
		default:
			a.elem, a.settled = tenon.Any(), false
		}
		return a, nil
	}
	if c.Type().Kind() != tenon.KindSet {
		l, empty, err := listAt("SetProduct", i, v, p)
		if err != nil || empty {
			return productArg{c: c, empty: empty, settled: true}, err
		}
		c = l
	}
	a.c, a.set, a.elem = c, c.Type().Kind() == tenon.KindSet, tenon.Exactly(c.Type().ElementType())
	if a.held = c.HasMembers(); a.held {
		a.members = c.Elements()
	}
	return a, nil
}

// productResult returns the constraint of SetProduct's answer: a set of
// tuples where an argument is a set, a list of them otherwise, each tuple
// holding a member of each argument in turn; the empty tuple where an
// argument is the empty tuple, the product being empty with no element
// type to name.
func productResult(as []productArg) tenon.Constraint {
	var types []tenon.Type
	var cs []tenon.Constraint
	set, settled, exact := false, true, true
	for _, a := range as {
		if a.empty {
			return tenon.Exactly(tenon.TupleType())
		}
		set = set || a.set
		settled = settled && a.settled
		cs = append(cs, a.elem)
		if a.elem.Kind() == tenon.ConstraintExactly {
			types = append(types, a.elem.Type())
		} else {
			exact = false
		}
	}
	switch {
	case !settled:
		return tenon.OneOf(tenon.ListOf(tenon.TupleOf(cs...)), tenon.SetOf(tenon.TupleOf(cs...)))
	case set && exact:
		return tenon.Exactly(tenon.SetType(tenon.TupleType(types...)))
	case set:
		return tenon.SetOf(tenon.TupleOf(cs...))
	case exact:
		return tenon.Exactly(tenon.ListType(tenon.TupleType(types...)))
	}
	return tenon.ListOf(tenon.TupleOf(cs...))
}

// productLengths returns the lengths of the product of the arguments: the
// least and greatest the answer may have, the products of the arguments',
// the greatest no more than productBound, and none where an argument's
// lengths allow no member, whatever the others are. The product fails past
// productBound, located at the argument whose length takes it past: where
// the least lengths take it past, and where every argument holds its
// members and the members they hold do, each counted, since that is the
// work of making it.
func productLengths(as []productArg) (lo, hi int64, empty bool, failure tenon.Value) {
	for _, a := range as {
		if _, h, ok := lengthOf(a.c); ok && h == 0 {
			return 0, 0, true, tenon.Value{}
		}
	}
	lo, hi = 1, 1
	work := int64(1)
	for i, a := range as {
		l, h, ok := lengthOf(a.c)
		n := l
		if a.held {
			n = int64(len(a.members))
		}
		lo, work = capped(lo, l, productBound+1), capped(work, n, productBound+1)
		if work > productBound {
			return 0, 0, false, tooLarge(i, "SetProduct: the product of the arguments' lengths passes "+strconv.Itoa(productBound)+" tuples, the most it makes")
		}
		if ok {
			hi = capped(hi, h, productBound)
		} else {
			hi = productBound
		}
	}
	return lo, hi, false, tenon.Value{}
}

// capped returns a times b, both at least zero, or most where that is less.
func capped(a, b, most int64) int64 {
	if b != 0 && a > most/b {
		return most
	}
	return min(a*b, most)
}

// SetProductFunc is the Cartesian product of two or more sets, lists or
// tuples: every tuple holding a member of each argument in turn, the last
// argument's varying fastest. It is a set of tuples where an argument is a
// set, members merging as a set's do, and a list of them otherwise; a
// tuple argument is read as the list its element types unify to under the
// call's policy, and the empty tuple makes the answer the empty tuple. More
// than 1,048,576 tuples fails with tenon.CodeFunctionTooLarge at the
// argument whose length takes the product past, before any is made, and so
// does an argument not known yet whose least length does. Not known yet,
// the answer is the unknown collection of the lengths the arguments allow.
// Every argument's own marks reach the answer, the members keeping theirs;
// a set's members carry none, so in a set of tuples they reach the set.
var SetProductFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "SetProduct",
	Description: "Calculates the Cartesian product of two or more sets.",
	Params:      []tenon.Param{collection("first", "The first set, list or tuple."), collection("second", "The second.")},
	VarParam:    &tenon.Param{Name: "sets", Description: "The further sets, lists or tuples.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true, AllowMarked: true},
	ResultOf: func(args []tenon.Value, p tenon.Policy) (tenon.Constraint, error) {
		as := make([]productArg, len(args))
		for i, v := range args {
			a, err := productArgOf(i, v, p)
			if err != nil {
				return tenon.Constraint{}, err
			}
			as[i] = a
		}
		return productResult(as), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
		marks := propagating(args...)
		as := make([]productArg, len(args))
		for i, v := range args {
			as[i], _ = productArgOf(i, v, p)
		}
		if rc.Kind() == tenon.ConstraintExactly && rc.Type().Kind() == tenon.KindTuple {
			return tenon.WithMarks(tenon.Tuple(), marks...), nil
		}
		lo, hi, empty, failure := productLengths(as)
		switch {
		case !failure.IsZero():
			return tenon.WithMarks(failure, marks...), nil
		case rc.Kind() != tenon.ConstraintExactly:
			return tenon.WithMarks(unknownOf(rc), marks...), nil
		}
		t := rc.Type()
		held := true
		for _, a := range as {
			held = held && a.held
		}
		switch {
		case empty && t.Kind() == tenon.KindList:
			return tenon.WithMarks(tenon.List(t.ElementType()), marks...), nil
		case empty:
			return tenon.WithMarks(tenon.Set(t.ElementType()), marks...), nil
		case !held:
			return tenon.WithMarks(tenon.Narrow(tenon.Unknown(t), tenon.NotNull(), tenon.LengthMin(lo), tenon.LengthMax(hi)), marks...), nil
		}
		tuples := product(as)
		if t.Kind() == tenon.KindList {
			return tenon.WithMarks(tenon.List(t.ElementType(), tuples...), marks...), nil
		}
		// A set's members carry no marks: those within a tuple reach the set.
		for i, tu := range tuples {
			var within []tenon.Mark
			tuples[i], within = tenon.UnmarkDeep(tu)
			marks = append(marks, within...)
		}
		return tenon.WithMarks(tenon.Set(t.ElementType(), tuples...), marks...), nil
	},
})

// product returns every tuple holding a member of each argument in turn,
// the last argument's varying fastest; every argument holds members.
func product(as []productArg) []tenon.Value {
	var out []tenon.Value
	at := make([]int, len(as))
	for {
		tuple := make([]tenon.Value, len(as))
		for i, a := range as {
			tuple[i] = a.members[at[i]]
		}
		out = append(out, tenon.Tuple(tuple...))
		i := len(as) - 1
		for ; i >= 0; i-- {
			if at[i]++; at[i] < len(as[i].members) {
				break
			}
			at[i] = 0
		}
		if i < 0 {
			return out
		}
	}
}
