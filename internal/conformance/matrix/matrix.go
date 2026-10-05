// Package matrix checks what must hold of every operation over the cross
// product that conformance asks for: each operand in each state it can be in,
// an error value, a pending value, an unknown value, a known one and null, and
// each operand unmarked, carrying marks, or holding a value that carries one;
// and each operand in turn carrying a deep mark and a redacting one.
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
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// Operation is what the matrix needs to know of an operation.
type Operation struct {
	Name     string
	Operands []Operand
	Agree    bool // the operands must have one type between them
	Fixed    bool // the result type does not depend on the operands' types
	// Collects says the operation converts its operands and collects every
	// failing one, each diagnostic located by its zero-based operand index,
	// as a function call does (FN-011, FN-012, FN-030); an operation's own
	// failures are unlocated, and a rejected operand fails it with the
	// conversion's codes rather than the operation's.
	Collects bool
	Call     func(args ...tenon.Value) tenon.Value
	// Shallow reports that a call reads none of the values within its
	// operands, its answer decided by what they are at their top, as an
	// equality with a null is: the marks held within them then stay off the
	// result (MK-003). Nil where the operation always reads within.
	Shallow func(args []tenon.Value) bool
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

// label is a mark the matrix attaches. None redacts but the one the redacted
// marking attaches, so that elsewhere a message reads the same whether or not
// what it renders is marked.
type label struct {
	id           string
	policy       tenon.Propagation
	deep, redact bool
}

func (m label) MarkID() string                 { return m.id }
func (m label) Propagation() tenon.Propagation { return m.policy }
func (m label) Redacting() bool                { return m.redact }
func (m label) Deep() bool                     { return m.deep }

// markedness is how the operand in one position is marked.
type markedness int

const (
	unmarked markedness = iota
	carries             // the operand carries a Propagate and an Isolate mark
	holds               // a value within the operand carries a Propagate mark
	redacted            // the operand carries a deep mark and a redacting Isolate mark
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
	met := make([]map[state]bool, len(op.Operands))
	for _, typ := range types {
		pools := make([][]tenon.Value, len(op.Operands))
		for i, o := range op.Operands {
			pools[i] = candidates(o, typ)
			for _, v := range pools[i] {
				if met[i] == nil {
					met[i] = map[state]bool{}
				}
				met[i][stateOf(v)] = true
			}
		}
		for _, args := range product(pools) {
			calls += checkArgs(op, args, report)
		}
	}
	// A position that meets no operand in some state is not checked in it,
	// and the calls the other positions make would hide that it is not.
	for i := range op.Operands {
		for _, s := range states {
			if !met[i][s] {
				report("coverage", "operand %d is never %s", i+1, s)
			}
		}
	}
	return violations, calls
}

// state is a state an operand can be in, as the matrix tells them apart.
type state string

// states are the states the matrix gives every operand position.
var states = []state{"an error", "pending", "unknown", "known", "null"}

// stateOf returns the state v is in.
func stateOf(v tenon.Value) state {
	switch {
	case v.IsError():
		return "an error"
	case v.IsPending():
		return "pending"
	case knownNull(v):
		return "null"
	case v.IsKnown():
		return "known"
	}
	return "unknown"
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
		// diagnostics, in operand order, each once. A collecting operation
		// locates each beneath its operand, and other operands' refusals may
		// join them, so its check asks for the located diagnostics in order
		// among the result's rather than for the whole list.
		if op.Collects {
			var want []tenon.Diagnostic
			for i, a := range args {
				if !a.IsError() {
					continue
				}
				for _, d := range a.Diagnostics() {
					ld := located(d, i)
					if !slices.ContainsFunc(want, ld.Equal) {
						want = append(want, ld)
					}
				}
			}
			if !r.IsError() || !subsequence(r.Diagnostics(), want) {
				report("ER-005", "%s(%s) gave %v, want every error operand's diagnostics, each located", op.Name, render(args), r)
			}
		} else {
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
			if rejected(a, op.Operands[i]) {
				if op.Collects {
					// A collecting operation rejects by converting, and
					// UN-023 makes recognising an impossible operand
					// mandatory only where its constraint names exactly
					// one type; a kind-level rejection the conversion may
					// defer surfaces when the operand resolves.
					if a.Constraint().Kind() == tenon.ConstraintExactly && !r.IsError() {
						report("UN-023", "%s(%s) gave %v, but operand %d can only be of a type the operation rejects", op.Name, render(args), r, i+1)
					}
				} else if !hasCode(r, tenon.CodeOperationWrongType) {
					report("UN-023", "%s(%s) gave %v, but operand %d can only be of a type the operation rejects", op.Name, render(args), r, i+1)
				}
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
		if stripped, _ := tenon.UnmarkDeep(mr); !sameAnswer(stripped, r, slices.Contains(marks, redacted)) {
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
	pending := []tenon.Value{any, tenon.Narrow(any, tenon.NullOnly()), tenon.Narrow(any, tenon.NotNull()),
		// A pending tuple and object holding members, one of them pending.
		tenon.Tuple(any, tenon.NumberFromInt(1)), tenon.Object(map[string]tenon.Value{"a": any})}
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
	// And constraints that name a kind of type rather than a type: one of a
	// kind the position could accept, and one of a kind it has no type of.
	for _, c := range candidateKinds() {
		if k, _ := kindNamed(c); admitsKind(o.Constraint, k) && (typ == nil || tenon.Satisfies(c, *typ)) {
			pending = append(pending, tenon.Pending(c))
			break
		}
	}
	for _, c := range candidateKinds() {
		if k, _ := kindNamed(c); !admitsKind(o.Constraint, k) {
			pending = append(pending, tenon.Pending(c))
			break
		}
	}
	return slices.Concat(errs, pending, unknown, known)
}

// candidateTypes returns types to make pending operands of.
func candidateTypes() []tenon.Type {
	str := tenon.StringType()
	return []tenon.Type{
		tenon.BoolType(), tenon.NumberType(), str, tenon.ListType(str), tenon.SetType(str),
		tenon.MapType(tenon.NumberType()), tenon.TupleType(), tenon.ObjectType(nil),
	}
}

// candidateKinds returns constraints that name a kind of type, to make
// pending operands of.
func candidateKinds() []tenon.Constraint {
	return []tenon.Constraint{
		tenon.ListOf(tenon.Any()), tenon.SetOf(tenon.Any()), tenon.MapOf(tenon.Any()),
		tenon.TupleOf(tenon.Any()), tenon.ObjectWith(nil, false),
	}
}

// kindNamed returns the kind of type that c admits, and whether c names a
// kind: a list, set, map, tuple or object constraint does, whatever its parts
// say.
func kindNamed(c tenon.Constraint) (tenon.Kind, bool) {
	switch c.Kind() {
	case tenon.ConstraintListOf:
		return tenon.KindList, true
	case tenon.ConstraintSetOf:
		return tenon.KindSet, true
	case tenon.ConstraintMapOf:
		return tenon.KindMap, true
	case tenon.ConstraintTupleOf:
		return tenon.KindTuple, true
	case tenon.ConstraintObjectWith:
		return tenon.KindObject, true
	}
	return 0, false
}

// admitsKind reports whether c could admit a type of kind k, as far as the
// types and kinds it names say. It is false only where each of them is of
// another kind, so that no type of kind k satisfies c.
func admitsKind(c tenon.Constraint, k tenon.Kind) bool {
	switch c.Kind() {
	case tenon.ConstraintAny:
		return true
	case tenon.ConstraintExactly:
		return c.Type().Kind() == k
	case tenon.ConstraintOneOf:
		for _, m := range c.Members() {
			if admitsKind(m, k) {
				return true
			}
		}
		return false
	}
	named, _ := kindNamed(c)
	return named == k
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

// sameAnswer reports whether an operation marked gave what it gave unmarked,
// its marks taken off: the same value, or, where a redacting mark may have
// changed what a diagnostic says and where it is located, an error value of
// the same codes (MK-005).
func sameAnswer(marked, unmarked tenon.Value, redacting bool) bool {
	if !redacting || !unmarked.IsError() {
		return tenon.Identical(marked, unmarked)
	}
	codes := func(v tenon.Value) []tenon.Code {
		var out []tenon.Code
		for _, d := range v.Diagnostics() {
			out = append(out, d.Code)
		}
		slices.Sort(out)
		return slices.Compact(out)
	}
	return marked.IsError() && slices.Equal(codes(marked), codes(unmarked))
}

// markings returns every markedness of the operands, the unmarked one aside:
// each operand unmarked or carrying marks, or holding a marked value where it
// is a known list with an element to mark; and each operand in turn carrying
// a deep mark and a redacting Isolate mark, the others unmarked, which a
// redacting mark carries as a Propagate mark does (MK-002).
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
	out = out[1:] // the first is every operand unmarked
	for i := range args {
		alone := make([]markedness, len(args))
		alone[i] = redacted
		out = append(out, alone)
	}
	return out
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
			out[i] = tenon.WithMarks(args[i], label{id: fmt.Sprintf("carried-%d", i), policy: tenon.Propagate}, label{id: fmt.Sprintf("isolated-%d", i), policy: tenon.Isolate})
			want = append(want, fmt.Sprintf("carried-%d", i))
		case holds:
			elems := args[i].Elements()
			elems[0] = tenon.WithMarks(elems[0], label{id: fmt.Sprintf("held-%d", i), policy: tenon.Propagate})
			out[i] = tenon.List(args[i].Type().ElementType(), elems...)
			if op.Operands[i].Within && (op.Shallow == nil || !op.Shallow(args)) {
				want = append(want, fmt.Sprintf("held-%d", i))
			}
		case redacted:
			deep := label{id: fmt.Sprintf("deep-%d", i), policy: tenon.Propagate, deep: true}
			secret := label{id: fmt.Sprintf("redacted-%d", i), policy: tenon.Isolate, redact: true}
			out[i] = tenon.WithMarks(args[i], deep, secret)
			want = append(want, deep.id, secret.id)
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

// rejected reports whether v is pending with a constraint that no type o
// accepts satisfies, so that an operation can never apply to v whatever it
// turns out to be. The matrix decides it for the constraints it builds
// pending operands of: one naming a type o does not accept, and one naming a
// kind o has no type of.
func rejected(v tenon.Value, o Operand) bool {
	if !v.IsPending() {
		return false
	}
	c := v.Constraint()
	if c.Kind() == tenon.ConstraintExactly {
		return !tenon.Satisfies(o.Constraint, c.Type())
	}
	k, ok := kindNamed(c)
	return ok && !admitsKind(o.Constraint, k)
}

// located returns d located beneath operand i, as a collecting operation
// locates it (FN-030).
func located(d tenon.Diagnostic, i int) tenon.Diagnostic {
	p := tenon.Path{}.Index(tenon.NumberFromInt(int64(i)))
	for _, s := range d.Path.Steps() {
		switch s.Kind() {
		case tenon.StepAttribute:
			p = p.Attribute(s.Name())
		case tenon.StepIndex:
			p = p.Index(s.Key())
		}
	}
	d.Path = p
	return d
}

// subsequence reports whether want's diagnostics appear among got's, in
// order.
func subsequence(got, want []tenon.Diagnostic) bool {
	j := 0
	for _, d := range got {
		if j < len(want) && want[j].Equal(d) {
			j++
		}
	}
	return j == len(want)
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
