package stdlib

import (
	"slices"
	"strconv"

	"github.com/kmoneil/tenon"
)

// unknownList returns the unknown list of the element type elem, not null,
// of a length from lo to hi, unbounded above where bounded is false.
func unknownList(elem tenon.Type, lo, hi int64, bounded bool) tenon.Value {
	ns := []tenon.Narrowing{tenon.NotNull(), tenon.LengthMin(lo)}
	if bounded {
		ns = append(ns, tenon.LengthMax(hi))
	}
	return tenon.Narrow(tenon.Unknown(tenon.ListType(elem)), ns...)
}

// marked returns v carrying the marks that propagate from each of vs.
func marked(v tenon.Value, vs ...tenon.Value) tenon.Value {
	return tenon.WithMarks(v, propagating(vs...)...)
}

// wholeAtLeastZero checks that the known number v is a whole number not
// less than zero, and returns the failure of argument i otherwise.
func wholeAtLeastZero(fn string, i int, what string, v tenon.Value) (tenon.Value, bool) {
	switch {
	case fractional(v):
		return invalid(i, fn+": the "+what+" "+v.String()+" is not a whole number"), false
	case less(v, zero):
		return invalid(i, fn+": the "+what+" "+v.String()+" is negative"), false
	}
	return tenon.Value{}, true
}

// SliceFunc is the elements of a list or a tuple from a start index up to,
// and not including, an end index: whole numbers with 0 <= start <= end <=
// length, each failure tenon.CodeFunctionInvalidArgument at its index, now
// where a list not known yet records lengths that rule it out. A set has no
// order to slice, and is refused. Not known yet, the answer is the unknown
// list of the length the indexes give, where both are known. Only the
// list's own marks and the indexes' reach the answer; its elements keep
// their own.
var SliceFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Slice",
	Description: "Extracts a subslice of the given list or tuple value.",
	Params: []tenon.Param{
		collection("list", "The list or tuple."),
		{Name: "start_index", Description: "The index of the first element taken.", Constraint: number, AllowUnknown: true},
		{Name: "end_index", Description: "The index just past the last element taken.", Constraint: number, AllowUnknown: true},
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		c, _ := tenon.Unmark(args[0])
		if mayBe(c, tenon.KindSet) && !mayBe(c, tenon.KindList, tenon.KindTuple) {
			return tenon.Constraint{}, wrongKind("Slice", 0, args[0], "a list or a tuple, a set having no order to slice by")
		}
		if !mayBe(c, tenon.KindList, tenon.KindTuple) {
			return tenon.Constraint{}, wrongKind("Slice", 0, args[0], "a list or a tuple")
		}
		if c.IsPending() {
			if el := c.Constraint(); el.Kind() == tenon.ConstraintListOf {
				return el, nil
			}
			return tenon.Any(), nil
		}
		t := c.Type()
		if t.Kind() == tenon.KindList {
			return tenon.Exactly(t), nil
		}
		start, end := args[1], args[2]
		if !start.IsKnown() || !end.IsKnown() {
			return tenon.Any(), nil
		}
		if failure, ok := sliceBounds(start, end, int64(t.TupleLength()), true); !ok {
			return tenon.Constraint{}, tenon.NewError(failure)
		}
		s, _ := start.AsInt64()
		e, _ := end.AsInt64()
		return tenon.Exactly(tenon.TupleType(t.TupleElementTypes()[s:e]...)), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		c, _ := tenon.Unmark(args[0])
		start, end := args[1], args[2]
		for i, v := range []tenon.Value{start, end} {
			if v.IsKnown() {
				if failure, ok := wholeAtLeastZero("Slice", i+1, []string{"start index", "end index"}[i], v); !ok {
					return marked(failure, args[0]), nil
				}
			}
		}
		_, hi, bounded := lengthOf(c)
		if start.IsKnown() && end.IsKnown() {
			if failure, ok := sliceBounds(start, end, hi, bounded); !ok {
				return marked(failure, args[0]), nil
			}
		} else if end.IsKnown() && bounded && less(tenon.NumberFromInt(hi), end) {
			return marked(invalid(2, "Slice: the end index "+end.String()+" is past the most elements the list holds, "+strconv.FormatInt(hi, 10)), args[0]), nil
		}
		if !c.HasMembers() || !start.IsKnown() || !end.IsKnown() {
			if k, ok := kindOf(c); ok && k == tenon.KindList {
				n := int64(-1)
				if start.IsKnown() && end.IsKnown() {
					s, _ := start.AsInt64()
					e, _ := end.AsInt64()
					n = e - s
				}
				if n >= 0 {
					return marked(unknownList(c.Type().ElementType(), n, n, true), args[0]), nil
				}
				return marked(unknownList(c.Type().ElementType(), 0, hi, bounded), args[0]), nil
			}
			return marked(unknownOf(rc), args[0]), nil
		}
		s, _ := start.AsInt64()
		e, _ := end.AsInt64()
		members := c.Elements()[s:e]
		if k, _ := kindOf(c); k == tenon.KindList {
			return marked(tenon.List(c.Type().ElementType(), members...), args[0]), nil
		}
		return marked(tenon.Tuple(members...), args[0]), nil
	},
})

// sliceBounds checks known indexes against a length of at most n (none where
// bounded is false): the end not past it, the start not past the end.
func sliceBounds(start, end tenon.Value, n int64, bounded bool) (tenon.Value, bool) {
	for i, v := range []tenon.Value{start, end} {
		if failure, ok := wholeAtLeastZero("Slice", i+1, []string{"start index", "end index"}[i], v); !ok {
			return failure, false
		}
	}
	switch {
	case bounded && less(tenon.NumberFromInt(n), end):
		return invalid(2, "Slice: the end index "+end.String()+" is past the length, "+strconv.FormatInt(n, 10)), false
	case less(end, start):
		return invalid(1, "Slice: the start index "+start.String()+" is past the end index "+end.String()), false
	}
	return tenon.Value{}, true
}

// ReverseListFunc is the elements of a list, a tuple or a set in reverse
// order, a set's in its canonical order (EQ-044) reversed. A list or a set
// answers a list, a tuple a tuple. A set holding members not known yet, whose
// length is a range, and a list not known yet, answer the unknown list of the
// lengths they allow. Only the value's own marks reach the answer; its
// elements keep their own.
var ReverseListFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "ReverseList",
	Description: "Returns the given list with its elements in reverse order.",
	Params:      []tenon.Param{collection("list", "The list, tuple or set.")},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		c, _ := tenon.Unmark(args[0])
		if !mayBe(c, tenon.KindList, tenon.KindTuple, tenon.KindSet) {
			return tenon.Constraint{}, wrongKind("ReverseList", 0, args[0], "a list, a tuple or a set")
		}
		if c.IsPending() {
			switch k := c.Constraint(); k.Kind() {
			case tenon.ConstraintListOf, tenon.ConstraintSetOf:
				return tenon.ListOf(k.Element()), nil
			case tenon.ConstraintTupleOf:
				ms := slices.Clone(k.Members())
				slices.Reverse(ms)
				return tenon.TupleOf(ms...), nil
			}
			return tenon.Any(), nil
		}
		t := c.Type()
		if t.Kind() == tenon.KindTuple {
			ts := t.TupleElementTypes()
			slices.Reverse(ts)
			return tenon.Exactly(tenon.TupleType(ts...)), nil
		}
		return tenon.Exactly(tenon.ListType(t.ElementType())), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		c, _ := tenon.Unmark(args[0])
		k, resolved := kindOf(c)
		if !c.HasMembers() {
			if resolved && (k == tenon.KindList || k == tenon.KindSet) {
				lo, hi, bounded := lengthOf(c)
				return marked(unknownList(c.Type().ElementType(), lo, hi, bounded), args[0]), nil
			}
			return marked(unknownOf(rc), args[0]), nil
		}
		if k == tenon.KindSet {
			if n := tenon.Length(c); !n.IsKnown() {
				lo, hi, bounded := lengthOf(c)
				return marked(unknownList(c.Type().ElementType(), lo, hi, bounded), args[0]), nil
			}
		}
		members := c.Elements()
		slices.Reverse(members)
		if resolved && (k == tenon.KindList || k == tenon.KindSet) {
			return marked(tenon.List(c.Type().ElementType(), members...), args[0]), nil
		}
		return marked(tenon.Tuple(members...), args[0]), nil
	},
})

// sequenceTypes returns the types of the members of the unmarked sequence c,
// in order, and false where they are not all settled: a list's element type
// as many times as it holds members, or a tuple's.
func sequenceTypes(c tenon.Value) ([]tenon.Type, bool) {
	k, resolved := kindOf(c)
	if !resolved {
		return nil, false
	}
	switch k {
	case tenon.KindTuple:
		return c.Type().TupleElementTypes(), true
	case tenon.KindList:
		if !c.HasMembers() {
			return nil, false
		}
		out := make([]tenon.Type, c.Len())
		for i := range out {
			out[i] = c.Type().ElementType()
		}
		return out, true
	}
	return nil, false
}

// ConcatFunc joins lists and tuples, one or more, in order. Lists whose
// element types unify under the call's policy (§7.5) join as a list of the
// unified type, each element converted to it; anything else joins as a
// tuple, where every argument's length is settled. A set or a map is
// refused. Not known yet, a list's answer is the unknown list of the summed
// lengths the arguments allow. Each argument's own marks reach the answer;
// the elements keep their own.
var ConcatFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Concat",
	Description: "Concatenates together all of the given lists or tuples into a single sequence, preserving the input order.",
	Params:      []tenon.Param{collection("seqs", "The first list or tuple.")},
	VarParam:    &tenon.Param{Name: "seqs", Description: "The further lists or tuples.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true, AllowMarked: true},
	ResultOf: func(args []tenon.Value, p tenon.Policy) (tenon.Constraint, error) {
		var elems []tenon.Constraint
		lists := true
		for i, a := range args {
			c, _ := tenon.Unmark(a)
			if !mayBe(c, tenon.KindList, tenon.KindTuple) {
				return tenon.Constraint{}, wrongKind("Concat", i, a, "lists and tuples")
			}
			switch k, resolved := kindOf(c); {
			case resolved && k == tenon.KindList:
				elems = append(elems, tenon.Exactly(c.Type().ElementType()))
			case !resolved && c.Constraint().Kind() == tenon.ConstraintListOf:
				elems = append(elems, c.Constraint().Element())
			default:
				lists = false
			}
		}
		if lists {
			if el, err := tenon.Unify(elems, p); err == nil {
				if el.Kind() == tenon.ConstraintExactly {
					return tenon.Exactly(tenon.ListType(el.Type())), nil
				}
				return tenon.ListOf(el), nil
			}
		}
		var types []tenon.Type
		for _, a := range args {
			c, _ := tenon.Unmark(a)
			ts, ok := sequenceTypes(c)
			if !ok {
				return tenon.Any(), nil
			}
			types = append(types, ts...)
		}
		return tenon.Exactly(tenon.TupleType(types...)), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
		listResult := rc.Kind() == tenon.ConstraintListOf || rc.Kind() == tenon.ConstraintExactly && rc.Type().Kind() == tenon.KindList
		var members []tenon.Value
		var lo, hi int64
		known, bounded := true, true
		for i, a := range args {
			c, _ := tenon.Unmark(a)
			if listResult {
				c = at(i, tenon.Convert(c, rc, p))
				if c.IsError() {
					return marked(c, args...), nil
				}
			}
			if !c.HasMembers() {
				known = false
				l, h, ok := lengthOf(c)
				lo, hi, bounded = lo+l, hi+h, bounded && ok
				continue
			}
			n := int64(c.Len())
			lo, hi = lo+n, hi+n
			members = append(members, c.Elements()...)
		}
		switch {
		case !known && listResult && rc.Kind() == tenon.ConstraintExactly:
			return marked(unknownList(rc.Type().ElementType(), lo, hi, bounded), args...), nil
		case !known:
			return marked(unknownOf(rc), args...), nil
		case listResult && rc.Kind() == tenon.ConstraintExactly:
			return marked(tenon.List(rc.Type().ElementType(), members...), args...), nil
		case listResult:
			return marked(tenon.Convert(tenon.Tuple(members...), rc, p), args...), nil
		}
		return marked(tenon.Tuple(members...), args...), nil
	},
})

// ChunklistFunc splits a list into consecutive lists of a size, the last
// shorter where the length is not a multiple of it; a size of zero gives one
// list holding the whole, as go-cty has it. The size is a whole number of
// any magnitude, else tenon.CodeFunctionInvalidArgument at it. A tuple is
// read as the list its element types unify to under the call's policy, and
// the empty tuple, a language's [], answers the empty tuple. Not known yet,
// the answer is the unknown list of the chunk counts the lengths and size
// allow. The list's own marks reach the answer; its elements keep their own.
var ChunklistFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Chunklist",
	Description: "Splits a single list into multiple lists where each has at most the given number of elements.",
	Params: []tenon.Param{
		collection("list", "The list to split."),
		{Name: "size", Description: "The most elements in each chunk.", Constraint: number, AllowUnknown: true},
	},
	ResultOf: func(args []tenon.Value, p tenon.Policy) (tenon.Constraint, error) {
		l, empty, err := asList("Chunklist", args[0], p)
		switch {
		case err != nil:
			return tenon.Constraint{}, err
		case empty:
			return tenon.Exactly(tenon.TupleType()), nil
		case l.IsPending():
			return tenon.ListOf(l.Constraint()), nil
		}
		return tenon.Exactly(tenon.ListType(l.Type())), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
		size := args[1]
		if size.IsKnown() {
			if failure, ok := wholeAtLeastZero("Chunklist", 1, "size", size); !ok {
				return marked(failure, args[0]), nil
			}
		}
		if rc.Kind() == tenon.ConstraintExactly && rc.Type().Kind() == tenon.KindTuple {
			return marked(tenon.Tuple(), args[0]), nil
		}
		c, _, _ := asList("Chunklist", args[0], p)
		if rc.Kind() != tenon.ConstraintExactly {
			return marked(unknownOf(rc), args[0]), nil
		}
		elem := rc.Type().ElementType().ElementType()
		lo, hi, bounded := lengthOf(c)
		switch {
		case !c.HasMembers() && size.IsKnown():
			return marked(chunksUnknown(rc.Type().ElementType(), lo, hi, bounded, size), args[0]), nil
		case !c.HasMembers():
			return marked(unknownList(rc.Type().ElementType(), min(lo, 1), hi, bounded), args[0]), nil
		case !size.IsKnown():
			n := int64(c.Len())
			return marked(unknownList(rc.Type().ElementType(), min(n, 1), n, true), args[0]), nil
		}
		members := c.Elements()
		var chunks []tenon.Value
		s, fits := size.AsInt64()
		if !fits || s == 0 || s > int64(len(members)) {
			s = int64(len(members))
		}
		for i := int64(0); i < int64(len(members)); i += s {
			chunks = append(chunks, tenon.List(elem, members[i:min(i+s, int64(len(members)))]...))
		}
		return marked(tenon.List(tenon.ListType(elem), chunks...), args[0]), nil
	},
})

// chunksUnknown returns the unknown list of chunks of a list of lo to hi
// elements in chunks of the known size.
func chunksUnknown(chunk tenon.Type, lo, hi int64, bounded bool, size tenon.Value) tenon.Value {
	s, fits := size.AsInt64()
	if !fits || s == 0 {
		// One chunk, or none of an empty list.
		return unknownList(chunk, min(lo, 1), 1, true)
	}
	return unknownList(chunk, (lo+s-1)/s, (hi+s-1)/s, bounded)
}

// asList returns the list argument v of a function taking a list, unmarked:
// a tuple is converted to the list its element types unify to under the
// policy p, so that its type is settled; empty reports the empty tuple,
// which has no element type, and a refusal is located at argument 0.
func asList(fn string, v tenon.Value, p tenon.Policy) (l tenon.Value, empty bool, err error) {
	c, _ := tenon.Unmark(v)
	if !mayBe(c, tenon.KindList, tenon.KindTuple) {
		return tenon.Value{}, false, wrongKind(fn, 0, v, "a list or a tuple")
	}
	return listAt(fn, 0, v, p)
}

// listAt returns argument i, v, a list or a tuple, unmarked, as asList
// does, a refusal located at it.
func listAt(fn string, i int, v tenon.Value, p tenon.Policy) (l tenon.Value, empty bool, err error) {
	c, _ := tenon.Unmark(v)
	if c.IsPending() {
		if k := c.Constraint(); k.Kind() == tenon.ConstraintListOf {
			return tenon.Pending(k), false, nil
		}
		return tenon.Pending(tenon.ListOf(tenon.Any())), false, nil
	}
	t := c.Type()
	if t.Kind() == tenon.KindList {
		return c, false, nil
	}
	ts := t.TupleElementTypes()
	if len(ts) == 0 {
		return tenon.Value{}, true, nil
	}
	cs := make([]tenon.Constraint, len(ts))
	for i, et := range ts {
		cs[i] = tenon.Exactly(et)
	}
	u, uerr := tenon.Unify(cs, p)
	if uerr != nil {
		return tenon.Value{}, false, wrongKind(fn, i, v, "a list, or a tuple whose element types unify")
	}
	converted := tenon.Convert(c, tenon.ListOf(u), p)
	if converted.IsError() {
		return tenon.Value{}, false, tenon.NewError(at(i, converted))
	}
	return converted, false, nil
}

// flattened gathers what Flatten makes of a value: the leaves, the marks of
// every container flattened, and whether every nested sequence's length is
// settled.
type flattened struct {
	leaves  []tenon.Value
	marks   []tenon.Mark
	settled bool
}

// add flattens v into f: a list, a set or a tuple that is not null and holds
// its members is flattened, a set's members taken in its canonical order; a
// sequence whose members are not there to read, not known yet or pending,
// or a set whose length is a range, unsettles the answer; anything else is
// a leaf, a null sequence among them.
func (f *flattened) add(v tenon.Value) {
	u, _ := tenon.Unmark(v)
	if u.IsPending() {
		if u.HasMembers() && u.Constraint().Kind() == tenon.ConstraintTupleOf {
			f.marks = append(f.marks, propagating(v)...)
			for _, m := range u.Elements() {
				f.add(m)
			}
			return
		}
		if mayBe(u, tenon.KindList, tenon.KindSet, tenon.KindTuple) && !u.IsNull() {
			f.settled = false
		}
		f.leaves = append(f.leaves, v)
		return
	}
	switch k := u.Type().Kind(); {
	case k != tenon.KindList && k != tenon.KindSet && k != tenon.KindTuple:
		f.leaves = append(f.leaves, v)
	case u.IsNull():
		f.leaves = append(f.leaves, v)
	case !u.HasMembers() || k == tenon.KindSet && !tenon.Length(u).IsKnown():
		f.marks = append(f.marks, propagating(v)...)
		f.settled = false
	default:
		f.marks = append(f.marks, propagating(v)...)
		for _, m := range u.Elements() {
			f.add(m)
		}
	}
}

// flatten returns what Flatten makes of v.
func flatten(v tenon.Value) flattened {
	f := flattened{settled: true}
	u, _ := tenon.Unmark(v)
	f.marks = append(f.marks, propagating(v)...)
	if !u.HasMembers() || (!u.IsPending() && u.Type().Kind() == tenon.KindSet && !tenon.Length(u).IsKnown()) {
		f.settled = false
		return f
	}
	for _, m := range u.Elements() {
		f.add(m)
	}
	return f
}

// FlattenFunc replaces each list, set and tuple within a list, a set or a
// tuple by its elements, at any depth, and answers the tuple of what is left,
// in order, a set's elements in its canonical order (EQ-044). A null list,
// set or tuple is left as it is, as go-cty leaves it, and so is a map or an
// object. Where a nested sequence's members are not known yet, or a set's
// length is a range, the answer is pending, as no constraint says a tuple
// of some length. Each flattened container's own marks reach the answer;
// the leaves keep theirs.
var FlattenFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Flatten",
	Description: "Transforms a list, set, or tuple value into a tuple by replacing any given elements that are themselves sequences with a flattened tuple of all of the nested elements concatenated together.",
	Params:      []tenon.Param{collection("list", "The list, set or tuple.")},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if u, _ := tenon.Unmark(args[0]); !mayBe(u, tenon.KindList, tenon.KindSet, tenon.KindTuple) {
			return tenon.Constraint{}, wrongKind("Flatten", 0, args[0], "a list, a set or a tuple")
		}
		f := flatten(args[0])
		if !f.settled {
			return tenon.Any(), nil
		}
		types := make([]tenon.Type, len(f.leaves))
		for i, l := range f.leaves {
			u, _ := tenon.Unmark(l)
			if u.IsPending() {
				return tenon.Any(), nil
			}
			types[i] = u.Type()
		}
		return tenon.Exactly(tenon.TupleType(types...)), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		f := flatten(args[0])
		if !f.settled {
			return tenon.WithMarks(unknownOf(rc), f.marks...), nil
		}
		return tenon.WithMarks(tenon.Tuple(f.leaves...), f.marks...), nil
	},
})

// CompactFunc is a list of strings without its null and empty members, in
// order. A member not known yet stays where its range rules out null and the
// empty string, a non-empty prefix doing so; where a member may be dropped
// or not, the answer is the unknown list of the lengths that leaves.
var CompactFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Compact",
	Description: "Removes all empty string elements from the given list of strings.",
	Params:      []tenon.Param{{Name: "list", Description: "The list of strings.", Constraint: tenon.ListOf(tenon.Exactly(tenon.StringType())), AllowUnknown: true}},
	Result:      tenon.Exactly(tenon.ListType(tenon.StringType())),
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		l := args[0]
		if !l.HasMembers() {
			_, hi, bounded := lengthOf(l)
			return unknownList(tenon.StringType(), 0, hi, bounded), nil
		}
		var kept []tenon.Value
		var surely, maybe int64
		decided := true
		for _, m := range l.Elements() {
			switch {
			case m.IsKnown() && (m.IsNull() || m.AsString() == ""):
			case m.IsKnown() || !m.Range().AllowsNull() && m.Range().StringPrefix() != "":
				kept = append(kept, m)
				surely++
				maybe++
			default:
				decided = false
				maybe++
			}
		}
		if !decided {
			return unknownList(tenon.StringType(), surely, maybe, true), nil
		}
		return tenon.List(tenon.StringType(), kept...), nil
	},
})

// DistinctFunc is a list without the members equal to an earlier one,
// first occurrences kept in order, equality being Equals (§5). Known members
// are told apart by their hashes, so the work grows with the members rather
// than with their pairs; a member not known yet is compared with every
// other. Where a member's equality with another is not settled, the answer
// is the unknown list whose least length is the members provably distinct
// from all before them and whose greatest is the members not provably equal
// to one before them. A tuple is read as the list its element types unify
// to under the call's policy, and the empty tuple, a language's [], answers
// the empty tuple.
var DistinctFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Distinct",
	Description: "Removes any duplicate values from the given list, preserving the order of remaining elements.",
	Params:      []tenon.Param{{Name: "list", Description: "The list or tuple.", Constraint: tenon.Any(), AllowUnknown: true, AllowPending: true}},
	ResultOf: func(args []tenon.Value, p tenon.Policy) (tenon.Constraint, error) {
		l, empty, err := asList("Distinct", args[0], p)
		switch {
		case err != nil:
			return tenon.Constraint{}, err
		case empty:
			return tenon.Exactly(tenon.TupleType()), nil
		case l.IsPending():
			return l.Constraint(), nil
		}
		return tenon.Exactly(l.Type()), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
		if rc.Kind() == tenon.ConstraintExactly && rc.Type().Kind() == tenon.KindTuple {
			return tenon.Tuple(), nil
		}
		l, _, _ := asList("Distinct", args[0], p)
		if l.IsPending() || !l.HasMembers() {
			if l.IsPending() {
				return unknownOf(rc), nil
			}
			lo, hi, bounded := lengthOf(l)
			return unknownList(l.Type().ElementType(), min(lo, 1), hi, bounded), nil
		}
		return distinctOf(l), nil
	},
})

// distinctOf answers Distinct of the list l, which holds its members.
func distinctOf(l tenon.Value) tenon.Value {
	var kept, apart members
	var notDuplicate int64
	decided := true
	for _, m := range l.Elements() {
		equal, open := kept.meet(m)
		if equal {
			continue
		}
		notDuplicate++
		decided = decided && !open
		kept.add(m)
		if e, o := apart.meet(m); !e && !o {
			apart.add(m)
		}
	}
	if !decided {
		return unknownList(l.Type().ElementType(), apart.count(), notDuplicate, true)
	}
	return tenon.List(l.Type().ElementType(), kept.order...)
}

// members holds values in order, the known ones also by hash, so that a
// known value is compared only with the known values of its hash and with
// those not known yet. A null has no hash; the members of a list share one
// type, so all its nulls are equal, and one held null stands for them.
type members struct {
	order   []tenon.Value
	known   map[uint64][]tenon.Value
	unknown []tenon.Value
	null    bool
}

func (s *members) add(m tenon.Value) {
	s.order = append(s.order, m)
	switch {
	case !m.IsKnown():
		s.unknown = append(s.unknown, m)
	case m.IsNull():
		s.null = true
	default:
		if s.known == nil {
			s.known = map[uint64][]tenon.Value{}
		}
		h := tenon.Hash(m)
		s.known[h] = append(s.known[h], m)
	}
}

// count returns how many values s holds.
func (s *members) count() int64 { return int64(len(s.order)) }

// meet reports whether m is provably equal to a value s holds, and whether
// its equality with one is not settled.
func (s *members) meet(m tenon.Value) (equal, open bool) {
	compare := func(o tenon.Value) {
		switch e := tenon.Equals(m, o); {
		case !e.IsKnown():
			open = true
		case e.AsBool():
			equal = true
		}
	}
	switch {
	case !m.IsKnown():
		for _, o := range s.order {
			compare(o)
		}
		return equal, open
	case m.IsNull():
		equal = s.null
	default:
		for _, o := range s.known[tenon.Hash(m)] {
			compare(o)
		}
	}
	for _, o := range s.unknown {
		compare(o)
	}
	return equal, open
}

// CoalesceListFunc returns the first of its arguments, lists and tuples,
// that is neither null nor empty, as it is. Nulls, a language's untyped null
// among them, and empty sequences are passed over; one that may still be
// null or empty, not known yet, leaves the answer unknown; where every
// argument is null or empty the call fails with
// tenon.CodeFunctionInvalidArgument. The result is derived from the
// arguments that may be chosen: their type where they agree, and one of
// their types otherwise. The answer carries the marks of the arguments
// examined, and none of one after the choice.
var CoalesceListFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "CoalesceList",
	Description: "Returns the first non-null, non-empty list or tuple among the given arguments.",
	Params:      []tenon.Param{anything("vals", "The first list or tuple to consider.")},
	VarParam:    &tenon.Param{Name: "vals", Description: "The further lists or tuples, in order.", Constraint: tenon.Any(), AllowNull: true, AllowUnknown: true, AllowPending: true, AllowMarked: true},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		var cs []tenon.Constraint
		for i, a := range args {
			u, _ := tenon.Unmark(a)
			if !u.IsNull() && !mayBe(u, tenon.KindList, tenon.KindTuple) {
				return tenon.Constraint{}, wrongKind("CoalesceList", i, a, "lists and tuples")
			}
		}
		for _, a := range args {
			u, _ := tenon.Unmark(a)
			state := emptiness(u)
			if state == passedOver {
				continue
			}
			cs = append(cs, typeOf(u))
			if state == chosen {
				break
			}
		}
		switch len(cs) {
		case 0:
			return tenon.Any(), nil
		case 1:
			return cs[0], nil
		}
		if slices.ContainsFunc(cs[1:], func(c tenon.Constraint) bool { return !c.Equal(cs[0]) }) {
			return tenon.OneOf(cs...), nil
		}
		return cs[0], nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		var read []tenon.Value
		for _, a := range args {
			u, _ := tenon.Unmark(a)
			read = append(read, a)
			switch emptiness(u) {
			case chosen:
				return marked(a, read...), nil
			case open:
				return marked(unknownOf(rc), read...), nil
			}
		}
		return marked(tenon.ErrorVal(tenon.Diagnostic{
			Code:    tenon.CodeFunctionInvalidArgument,
			Message: "every argument of CoalesceList is null or empty, and it has no answer without one that is neither",
		}), read...), nil
	},
})

// The answers emptiness gives of a CoalesceList argument.
const (
	passedOver = iota // null, or empty
	chosen            // neither null nor empty
	open              // either, as it turns out
)

// emptiness says whether the unmarked sequence v is passed over, chosen or
// open: null or empty, neither, or not settled.
func emptiness(v tenon.Value) int {
	if v.IsNull() {
		return passedOver
	}
	if n := tenon.IsNull(v); !n.IsKnown() {
		return open
	}
	lo, hi, bounded := lengthOf(v)
	switch {
	case v.HasMembers() && v.Len() == 0, bounded && hi == 0:
		return passedOver
	case lo > 0 || v.HasMembers() && v.Len() > 0:
		return chosen
	}
	return open
}
