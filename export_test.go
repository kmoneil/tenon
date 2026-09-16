package tenon

// RegisteredOperation describes a registered operation to the operand matrix,
// which lives outside the package.
type RegisteredOperation struct {
	Name string
	// Params describes the parameters an operation that takes them was bound
	// to, and is empty for one that takes none.
	Params      string
	Constraints []Constraint
	Nulls       []bool
	Within      []bool
	Agree       bool
	Fixed       bool
	Call        func(args ...Value) Value
}

// RegisteredOperations returns every registered operation, in the order they
// were declared. An operation that takes parameters appears once for each
// choice of them that it names for the matrix.
func RegisteredOperations() []RegisteredOperation {
	var bound []*op
	for _, o := range operations {
		if o.bind == nil {
			bound = append(bound, o)
			continue
		}
		for _, p := range o.samples {
			bound = append(bound, o.with(p))
		}
	}
	out := make([]RegisteredOperation, len(bound))
	for i, o := range bound {
		r := RegisteredOperation{Name: o.name, Agree: o.agree, Call: o.apply}
		if o.param != nil {
			r.Params = o.param.String()
		}
		for _, operand := range o.operands {
			r.Constraints = append(r.Constraints, operand.constraint)
			r.Nulls = append(r.Nulls, operand.nulls)
			r.Within = append(r.Within, operand.within)
		}
		// A result function is asked about operands whose types nothing has
		// settled; one that names a type then names it whatever they are.
		r.Fixed = o.result(make([]Type, len(o.operands))).Kind() == ConstraintExactly
		out[i] = r
	}
	return out
}

// PendingConstraint returns the constraint a pending value carries, which the
// public API does not expose yet.
func PendingConstraint(v Value) Constraint {
	if v.data().state != statePending {
		usagePanic("PendingConstraint called on %s", v.n.describe())
	}
	return v.n.data.(Constraint)
}
