// Package matrix checks what must hold of every operation over the cross
// product that conformance asks for: each operand in each state it can be in,
// an error value, a pending value, an unknown value and a known one, and each
// operand unmarked, carrying marks, or holding a value that carries one.
//
// It is a package beside conformance rather than part of it because it builds
// values, so it imports tenon, and tenon's own internal tests import
// conformance.
package matrix

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance/values"
)

// Operation is what the matrix needs to know of an operation.
type Operation struct {
	Name     string
	Operands []Operand
	Agree    bool // the operands must have one type between them
	Fixed    bool // the result type does not depend on the operands' types
	Call     func(args ...tenon.Value) tenon.Value
}

// Operand is what an operation accepts in one position.
type Operand struct {
	Constraint tenon.Constraint
	Nulls      bool // the operation has an answer for null here
	Within     bool // the operation reads the values within the operand
}

// Violation is one invariant broken by one call.
type Violation struct {
	Operation, Rule, Detail string
}

func (v Violation) String() string { return v.Operation + ": " + v.Rule + ": " + v.Detail }

// label is a mark the matrix attaches. It never redacts, so a message reads
// the same whether or not what it renders is marked.
type label struct {
	id     string
	policy tenon.Propagation
}

func (m label) MarkID() string                 { return m.id }
func (m label) Propagation() tenon.Propagation { return m.policy }
func (label) Redacting() bool                  { return false }

// markedness is how the operand in one position is marked.
type markedness int

const (
	unmarked markedness = iota
	carries             // the operand carries a Propagate and an Isolate mark
	holds               // a value within the operand carries a Propagate mark
)

// Check runs every operation over the matrix. It returns what was violated and
// how many calls were checked, so that a caller can tell a clean run from an
// empty one.
func Check(ops []Operation) (violations []Violation, calls int) {
	for _, op := range ops {
		v, n := checkOperation(op)
		violations = append(violations, v...)
		calls += n
	}
	return violations, calls
}

// checkOperation runs one operation over the matrix.
func checkOperation(op Operation) (violations []Violation, calls int) {
	report := func(rule, format string, args ...any) {
		violations = append(violations, Violation{op.Name, rule, fmt.Sprintf(format, args...)})
	}
	types := []*tenon.Type{nil}
	if op.Agree {
		types = sharedTypes(op)
	}
	for _, typ := range types {
		pools := make([][]tenon.Value, len(op.Operands))
		for i, o := range op.Operands {
			pools[i] = candidates(o, typ)
		}
		for _, args := range product(pools) {
			calls += checkArgs(op, args, report)
		}
	}
	return violations, calls
}

// checkArgs checks one choice of operands in every markedness, returning how
// many calls it made.
func checkArgs(op Operation, args []tenon.Value, report func(rule, format string, args ...any)) int {
	r, failure := call(op, args)
	calls := 1
	if failure != "" {
		report("no panic", "%s(%s) panicked: %s", op.Name, render(args), failure)
		return calls
	}
	if again, _ := call(op, args); !tenon.Identical(again, r) {
		report("determinism", "%s(%s) gave %v and then %v", op.Name, render(args), r, again)
	}
	calls++
	var errs []tenon.Value
	anyPending, allKnown := false, true
	for _, a := range args {
		switch {
		case a.IsError():
			errs = append(errs, a)
		case a.IsPending():
			anyPending = true
		}
		allKnown = allKnown && a.IsKnown()
	}
	if len(errs) > 0 {
		// ER-005: an error operand makes an error value of the concatenated
		// diagnostics, in operand order, each once.
		var want []tenon.Diagnostic
		for _, e := range errs {
			for _, d := range e.Diagnostics() {
				if !slices.ContainsFunc(want, d.Equal) {
					want = append(want, d)
				}
			}
		}
		if !r.IsError() || !slices.EqualFunc(r.Diagnostics(), want, tenon.Diagnostic.Equal) {
			report("ER-005", "%s(%s) gave %v, want the error operands' diagnostics", op.Name, render(args), r)
		}
	} else {
		if allKnown && !r.IsKnown() && !r.IsError() {
			report("UN-008", "%s(%s) gave %v, neither known nor an error", op.Name, render(args), r)
		}
		if anyPending && op.Fixed && r.IsPending() {
			report("UN-023", "%s(%s) gave a pending value although its result type is fixed", op.Name, render(args))
		}
		for i, a := range args {
			if !op.Operands[i].Nulls && knownNull(a) && !hasCode(r, tenon.CodeOperationNullOperand) {
				report("UN-009", "%s(%s) gave %v for a null operand %d", op.Name, render(args), r, i+1)
			}
			if rejected(a, op.Operands[i]) && !hasCode(r, tenon.CodeOperationWrongType) {
				report("UN-023", "%s(%s) gave %v, but operand %d can only be of a type the operation rejects", op.Name, render(args), r, i+1)
			}
		}
	}
	for _, marks := range markings(args) {
		margs, want := mark(op, args, marks)
		mr, failure := call(op, margs)
		calls++
		if failure != "" {
			report("no panic", "%s(%s) panicked: %s", op.Name, render(margs), failure)
			continue
		}
		rule := "MK-003"
		if mr.IsError() {
			rule = "MK-010"
		}
		if got := markIDs(mr); !slices.Equal(got, want) {
			report(rule, "%s(%s) carries %v, want %v", op.Name, render(margs), got, want)
		}
		if stripped, _ := tenon.UnmarkDeep(mr); !tenon.Identical(stripped, r) {
			report("MK-005", "%s(%s) gave %v, but %v unmarked", op.Name, render(margs), stripped, r)
		}
	}
	return calls
}

// call calls the operation, reporting a panic rather than propagating it.
func call(op Operation, args []tenon.Value) (r tenon.Value, failure string) {
	defer func() {
		if p := recover(); p != nil {
			failure = fmt.Sprint(p)
		}
	}()
	return op.Call(args...), ""
}

// candidates returns operands for one position: a few in each state that the
// position accepts, of type typ where typ is given. Operands are chosen for
// variety: at most two of each type the generator holds, and a single null,
// so that containers with members to mark are among them.
func candidates(o Operand, typ *tenon.Type) []tenon.Value {
	var errs, known, nulls, unknown []tenon.Value
	seen := map[string]int{}
	for _, v := range values.All() {
		if _, marks := tenon.UnmarkDeep(v); marks != nil {
			continue
		}
		switch {
		case v.IsError():
			if len(errs) < 2 {
				errs = append(errs, v)
			}
		case v.IsPending():
		case !tenon.Satisfies(o.Constraint, v.Type()) || typ != nil && v.Type() != *typ:
		case knownNull(v):
			if len(nulls) < 1 {
				nulls = append(nulls, v)
			}
		case v.IsKnown():
			if key := "k" + v.Type().String(); seen[key] < 2 {
				seen[key]++
				known = append(known, v)
			}
		default:
			if key := "u" + v.Type().String(); seen[key] < 2 {
				seen[key]++
				unknown = append(unknown, v)
			}
		}
	}
	known = append(known, nulls...)
	any := tenon.Pending(tenon.Any())
	pending := []tenon.Value{any, tenon.Narrow(any, tenon.Null()), tenon.Narrow(any, tenon.NotNull())}
	for _, t := range candidateTypes() {
		if tenon.Satisfies(o.Constraint, t) && (typ == nil || t == *typ) {
			pending = append(pending, tenon.Pending(tenon.Exactly(t)))
			break
		}
	}
	for _, t := range candidateTypes() {
		if !tenon.Satisfies(o.Constraint, t) {
			pending = append(pending, tenon.Pending(tenon.Exactly(t)))
			break
		}
	}
	return slices.Concat(errs, pending, unknown, known)
}

// candidateTypes returns types to make pending operands of.
func candidateTypes() []tenon.Type {
	str := tenon.StringType()
	return []tenon.Type{
		tenon.BoolType(), tenon.NumberType(), str, tenon.List(str), tenon.Set(str),
		tenon.Map(tenon.NumberType()), tenon.Tuple(), tenon.Object(nil),
	}
}

// sharedTypes returns the types that every operand of an operation that takes
// operands of one type accepts.
func sharedTypes(op Operation) []*tenon.Type {
	var out []*tenon.Type
	for _, t := range candidateTypes() {
		ok := true
		for _, o := range op.Operands {
			ok = ok && tenon.Satisfies(o.Constraint, t)
		}
		if ok {
			out = append(out, &t)
		}
	}
	return out
}

// product returns every choice of one operand from each pool.
func product(pools [][]tenon.Value) [][]tenon.Value {
	out := [][]tenon.Value{nil}
	for _, pool := range pools {
		var next [][]tenon.Value
		for _, prefix := range out {
			for _, v := range pool {
				next = append(next, append(slices.Clone(prefix), v))
			}
		}
		out = next
	}
	return out
}

// markings returns every markedness of the operands, the unmarked one aside:
// each operand unmarked or carrying marks, or holding a marked value where it
// is a known list with an element to mark.
func markings(args []tenon.Value) [][]markedness {
	out := [][]markedness{nil}
	for _, a := range args {
		choices := []markedness{unmarked, carries}
		if a.IsKnown() && !knownNull(a) && a.Type().Kind() == tenon.KindList && a.Len() > 0 {
			choices = append(choices, holds)
		}
		var next [][]markedness
		for _, prefix := range out {
			for _, c := range choices {
				next = append(next, append(slices.Clone(prefix), c))
			}
		}
		out = next
	}
	return out[1:] // the first is every operand unmarked
}

// mark returns the operands marked as marks says, and the identifiers of the
// marks the result must carry: the Propagate marks the operands carry, and
// those held within an operand the operation reads within, sorted.
func mark(op Operation, args []tenon.Value, marks []markedness) ([]tenon.Value, []string) {
	out := slices.Clone(args)
	var want []string
	for i, m := range marks {
		switch m {
		case carries:
			out[i] = tenon.WithMarks(args[i], label{fmt.Sprintf("carried-%d", i), tenon.Propagate}, label{fmt.Sprintf("isolated-%d", i), tenon.Isolate})
			want = append(want, fmt.Sprintf("carried-%d", i))
		case holds:
			elems := args[i].Elements()
			elems[0] = tenon.WithMarks(elems[0], label{fmt.Sprintf("held-%d", i), tenon.Propagate})
			out[i] = tenon.ListVal(args[i].Type().ElementType(), elems...)
			if op.Operands[i].Within {
				want = append(want, fmt.Sprintf("held-%d", i))
			}
		}
	}
	slices.Sort(want)
	return out, want
}

// markIDs returns the identifiers of the marks v carries, sorted.
func markIDs(v tenon.Value) []string {
	_, marks := tenon.Unmark(v)
	var ids []string
	for _, m := range marks {
		ids = append(ids, m.MarkID())
	}
	slices.Sort(ids)
	return ids
}

// knownNull reports whether v is known to be null, pending or not.
func knownNull(v tenon.Value) bool {
	if v.IsError() {
		return false
	}
	n := tenon.IsNull(v)
	return n.IsKnown() && n.AsBool()
}

// rejected reports whether v is pending with a constraint that names one type,
// and o does not accept that type, so that an operation can never apply to v
// whatever it turns out to be.
func rejected(v tenon.Value, o Operand) bool {
	if !v.IsPending() {
		return false
	}
	c := v.Constraint()
	return c.Kind() == tenon.ConstraintExactly && !tenon.Satisfies(o.Constraint, c.Type())
}

// hasCode reports whether v is an error value with a diagnostic of code c.
func hasCode(v tenon.Value, c tenon.Code) bool {
	return v.IsError() && slices.ContainsFunc(v.Diagnostics(), func(d tenon.Diagnostic) bool { return d.Code == c })
}

// render describes operands for a message.
func render(args []tenon.Value) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.String()
	}
	return strings.Join(parts, ", ")
}
