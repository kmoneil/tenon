package tenon

// RegisteredOperation describes a registered operation to the operand matrix,
// which lives outside the package.
type RegisteredOperation struct {
	Name        string
	Constraints []Constraint
	Nulls       []bool
	Within      []bool
	Agree       bool
	Fixed       bool
	Call        func(args ...Value) Value
}

// RegisteredOperations returns every registered operation, in the order they
// were declared.
func RegisteredOperations() []RegisteredOperation {
	out := make([]RegisteredOperation, len(operations))
	for i, o := range operations {
		r := RegisteredOperation{Name: o.name, Agree: o.agree, Call: o.apply}
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
