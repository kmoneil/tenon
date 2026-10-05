package tenon_test

import (
	"slices"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/conformance/matrix"
)

// registered returns the package's registered operations as the matrix sees
// them.
func registered() []matrix.Operation {
	var ops []matrix.Operation
	for _, r := range tenon.RegisteredOperations() {
		op := matrix.Operation{Name: r.Name + r.Params, Agree: r.Agree, Fixed: r.Fixed, Call: r.Call}
		for i, c := range r.Constraints {
			op.Operands = append(op.Operands, matrix.Operand{Constraint: c, Nulls: r.Nulls[i], Within: r.Within[i]})
		}
		ops = append(ops, op)
	}
	return ops
}

// functionOperations returns caller-defined functions as the matrix sees
// them, so every state crosses a real call (FN rules) in every argument
// position: one function on the default terms, and one whose first
// parameter admits null.
func functionOperations() []matrix.Operation {
	num := tenon.Exactly(tenon.NumberType())
	add := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "MatrixAdd",
		Params: []tenon.Param{{Name: "a", Constraint: num}, {Name: "b", Constraint: num}},
		Result: num,
		Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
			return tenon.Add(args[0], args[1]), nil
		},
	})
	orZero := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "MatrixAddOrZero",
		Params: []tenon.Param{{Name: "a", Constraint: num, AllowNull: true}, {Name: "b", Constraint: num}},
		Result: num,
		Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
			if args[0].IsNull() {
				return args[1], nil
			}
			return tenon.Add(args[0], args[1]), nil
		},
	})
	within := func(nulls bool) matrix.Operand {
		return matrix.Operand{Constraint: num, Nulls: nulls, Within: true}
	}
	call := func(f tenon.Function) func(...tenon.Value) tenon.Value {
		return func(args ...tenon.Value) tenon.Value { return tenon.Call(f, args, tenon.Safe) }
	}
	return []matrix.Operation{
		{Name: "Call(MatrixAdd)", Operands: []matrix.Operand{within(false), within(false)}, Fixed: true, Collects: true, Call: call(add)},
		{Name: "Call(MatrixAddOrZero)", Operands: []matrix.Operand{within(true), within(false)}, Fixed: true, Collects: true, Call: call(orZero)},
	}
}

func TestConformance_MK005_OperandMatrix(t *testing.T) {
	conformance.Covers(t, "ER-005", "UN-008", "UN-009", "UN-023", "MK-003", "MK-005", "MK-010")
	violations, calls := matrix.Check(append(registered(), functionOperations()...))
	for i, v := range violations {
		if i == 20 {
			t.Errorf("and %d more", len(violations)-i)
			break
		}
		t.Error(v)
	}
	// A matrix that checked next to nothing would pass as cleanly.
	if calls < 10000 {
		t.Errorf("the matrix made only %d calls", calls)
	}
}

// TestOperandMatrixCatchesBrokenOperations runs the matrix over operations
// broken in the ways it exists to catch, and requires each to be caught under
// the rule it breaks.
func TestOperandMatrixCatchesBrokenOperations(t *testing.T) {
	num := tenon.NumberType()
	numbers := []matrix.Operand{{Constraint: tenon.Exactly(num)}, {Constraint: tenon.Exactly(num)}}
	add := func(args []tenon.Value) tenon.Value { return tenon.Add(args[0], args[1]) }
	unmarkAll := func(args []tenon.Value) []tenon.Value {
		out := make([]tenon.Value, len(args))
		for i, a := range args {
			out[i], _ = tenon.UnmarkDeep(a)
		}
		return out
	}
	for _, tt := range []struct {
		rule string
		op   matrix.Operation
	}{
		{"MK-003", matrix.Operation{Name: "drops marks", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			return add(unmarkAll(args))
		}}},
		{"ER-005", matrix.Operation{Name: "keeps the first error only", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			if args[0].IsError() {
				return args[0]
			}
			return add(args)
		}}},
		{"UN-008", matrix.Operation{Name: "answers known operands with an unknown", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			if r := add(args); r.IsKnown() {
				return tenon.WithMarks(tenon.Unknown(num), marksOf(r)...)
			}
			return add(args)
		}}},
		{"UN-023", matrix.Operation{Name: "answers pending operands with pending", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			if r := add(args); !r.IsError() && (args[0].IsPending() || args[1].IsPending()) {
				return tenon.WithMarks(tenon.Pending(tenon.Exactly(num)), marksOf(r)...)
			}
			return add(args)
		}}},
		{"UN-023", matrix.Operation{Name: "forgets the type a pending operand names", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			forgot := slices.Clone(args)
			for i, a := range args {
				if a.IsPending() && a.Constraint().Kind() == tenon.ConstraintExactly {
					forgot[i] = tenon.WithMarks(tenon.Pending(tenon.Any()), marksOf(a)...)
				}
			}
			return add(forgot)
		}}},
		{"UN-023", matrix.Operation{Name: "decides only a pending operand that names one type", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			forgot := slices.Clone(args)
			for i, a := range args {
				if !a.IsPending() {
					continue
				}
				if k := a.Constraint().Kind(); k != tenon.ConstraintExactly && k != tenon.ConstraintAny {
					forgot[i] = tenon.WithMarks(tenon.Pending(tenon.Any()), marksOf(a)...)
				}
			}
			return add(forgot)
		}}},
		{"UN-009", matrix.Operation{Name: "answers null with zero", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			for i, a := range args {
				if !a.IsError() && !a.IsPending() && a.IsKnown() && tenon.IsNull(a).AsBool() {
					u := unmarkAll(args)
					u[i] = tenon.NumberFromInt(0)
					return tenon.WithMarks(add(u), marksOf(add(args))...)
				}
			}
			return add(args)
		}}},
		{"MK-005", matrix.Operation{Name: "answers marked operands differently", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			if r := add(args); len(marksOf(r)) > 0 && r.IsKnown() {
				return tenon.WithMarks(tenon.NumberFromInt(42), marksOf(r)...)
			}
			return add(args)
		}}},
		{"MK-003", matrix.Operation{
			Name:     "drops marks held within what it reads",
			Operands: []matrix.Operand{{Constraint: tenon.Any(), Nulls: true, Within: true}, {Constraint: tenon.Any(), Nulls: true, Within: true}},
			Fixed:    true,
			Call: func(args ...tenon.Value) tenon.Value {
				for i, a := range args {
					u, _ := tenon.UnmarkDeep(a)
					args[i] = tenon.WithMarks(u, marksOf(a)...)
				}
				return tenon.Equals(args[0], args[1])
			},
		}},
		// Isolate marks stay behind, but not a redacting one (MK-002), which
		// only an operand carrying one alone tells apart.
		{"MK-003", matrix.Operation{Name: "carries a redacting Isolate mark no further", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			var propagated []tenon.Mark
			for _, a := range args {
				for _, m := range marksOf(a) {
					if m.Propagation() == tenon.Propagate {
						propagated = append(propagated, m)
					}
				}
			}
			return tenon.WithMarks(add(unmarkAll(args)), propagated...)
		}}},
		// A position whose constraint no value the matrix draws on satisfies
		// meets no unknown, known or null operand, which its calls in the
		// other states would not show.
		{"coverage", matrix.Operation{
			Name:     "takes what no generated value is",
			Operands: []matrix.Operand{{Constraint: tenon.Exactly(tenon.ObjectType(map[string]tenon.Type{"never": num}))}},
			Fixed:    true,
			Call:     func(...tenon.Value) tenon.Value { return tenon.Bool(true) },
		}},
		{"no panic", matrix.Operation{Name: "panics on an unknown", Operands: numbers, Fixed: true, Call: func(args ...tenon.Value) tenon.Value {
			if args[0].IsResolved() && !args[0].IsKnown() {
				return tenon.Add(args[0], tenon.Bool(true)) // a usage panic
			}
			return add(args)
		}}},
		{"ER-005", matrix.Operation{Name: "forgets where its failures sit", Operands: numbers, Fixed: true, Collects: true, Call: func(args ...tenon.Value) tenon.Value {
			// A collecting operation must locate an error operand's
			// diagnostics beneath its index, as go-cty's variadic
			// argument errors are not.
			for _, a := range args {
				if a.IsError() {
					return a
				}
			}
			return add(args)
		}}},
	} {
		violations, _ := matrix.Check([]matrix.Operation{tt.op})
		if !slices.ContainsFunc(violations, func(v matrix.Violation) bool { return v.Rule == tt.rule }) {
			t.Errorf("%s breaks %s, but the matrix found %d violations, none of %s", tt.op.Name, tt.rule, len(violations), tt.rule)
		}
	}
}

// marksOf returns the marks v carries.
func marksOf(v tenon.Value) []tenon.Mark {
	_, marks := tenon.Unmark(v)
	return marks
}

// TestEveryOperationIsRegistered holds the package to the matrix: every
// operation is declared through register, and every exported function that
// makes a value is either a registered operation or one of the constructors
// and refinements the matrix does not cover, named here so that a new one has
// to be placed.
func TestEveryOperationIsRegistered(t *testing.T) {
	for _, lit := range operationLiterals(t) {
		if !lit.registered || lit.keys["registered"] {
			t.Errorf("%s: an operation is declared outside register", lit.where)
		}
	}
	var ops []string
	for _, op := range tenon.RegisteredOperations() {
		ops = append(ops, op.Name)
	}
	others := []string{
		"Bool", "Call", "CapsuleType.Value", "Deserialize", "ErrorVal", "List", "Map", "Narrow", "Null", "NumberFromBigInt",
		"NumberFromBigRat", "NumberFromInt", "NumberFromText", "Object", "ParseJSON", "Pending", "Resolve", "Set", "String",
		"Tuple", "Unknown", "Unmark", "UnmarkDeep", "WithMarks",
	}
	for _, name := range exportedFuncs(t, "Value") {
		isOp, isOther := slices.Contains(ops, name), slices.Contains(others, name)
		if isOp == isOther {
			t.Errorf("%s makes a value and is %s", name, map[bool]string{true: "both an operation and not one", false: "neither a registered operation nor a named exception"}[isOp])
		}
	}
	for _, name := range ops {
		if !slices.Contains(exportedFuncs(t, "Value"), name) {
			t.Errorf("the registered operation %s has no exported function of its name", name)
		}
	}
}
