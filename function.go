package tenon

import (
	"errors"
	"slices"
	"strconv"
	"strings"
)

// Param is one parameter of a function: the constraint an argument there must
// convert to, and what the implementation is willing to see in that position.
// By default the implementation sees only arguments that are known, not null,
// unmarked and resolved; each admission widens that without changing what any
// state means at the call, which the boundary decides the same way for every
// function.
type Param struct {
	// Name names the parameter in messages. It is optional.
	Name string
	// Description describes the parameter for documentation. Call ignores it.
	Description string
	// Constraint is what an argument in this position must convert to, under
	// the policy the call is given. Any() admits every type.
	Constraint Constraint
	// AllowNull passes a null argument to the implementation, which then has
	// the meaning for it. Without it a null argument fails with
	// CodeOperationNullOperand, as an operation with no answer for null fails
	// (UN-009).
	AllowNull bool
	// AllowUnknown passes an argument that is not wholly known to the
	// implementation. Without it such an argument makes the result the
	// unknown of the function's result, without the implementation running
	// (UN-007).
	AllowUnknown bool
	// AllowPending passes a pending argument to an implementation that reads
	// constraints itself. Without it a pending argument answers the call as
	// every operation answers one (UN-023).
	AllowPending bool
	// AllowMarked passes a marked argument as it is, and propagating its
	// marks to the result is then the implementation's to do. Without it the
	// argument is unmarked deeply before anything reads it, and the result
	// carries the marks that propagate (MK-003). Marks never change the
	// call's value result either way (MK-005).
	AllowMarked bool
}

// FunctionSpec describes a function for NewFunction. Exactly the fields that
// are documented as required must be set; NewFunction panics on a
// specification that cannot make a function.
type FunctionSpec struct {
	// Name names the function in messages and panics. It is optional; an
	// unnamed function is named "the function".
	Name string
	// Description describes the function for documentation. Call ignores it.
	Description string
	// Params are the positional parameters, in order.
	Params []Param
	// VarParam, if set, stands for any number of further arguments after the
	// positional ones, each on its terms.
	VarParam *Param
	// Result is the constraint the call's result satisfies. Exactly one of
	// Result and ResultOf must be set.
	Result Constraint
	// ResultOf derives the call's result constraint from the converted
	// arguments, in the states they stand: a derivation given an unknown or
	// a pending argument says what it can from the rest, as an expression
	// language type-checking before it runs needs it to. A refusal is
	// returned as an error, which becomes diagnostics as Impl's failures
	// do. Exactly one of Result and ResultOf must be set.
	ResultOf func(args []Value) (Constraint, error)
	// Volatile declares that the result is not a function of the arguments,
	// as a timestamp or a fresh identifier is not. A volatile call's result
	// is the unknown of its result constraint even where every argument is
	// known, without the implementation running: the declared exception to
	// the rule that known operands give a known result.
	Volatile bool
	// Impl is the function's behavior, given the converted arguments and the
	// result constraint the call promised. It is required. A failure is
	// returned as an error, which becomes diagnostics (a *Error contributes
	// its own as they are, any other error its text under
	// CodeFunctionFailed), or equally as an error value. What it returns
	// must satisfy the result constraint, and where every argument was known
	// it must be known, given or an error: a given value leaves nothing open
	// but types, as the pending null ParseJSON reads from a JSON null with
	// Any does, and a tuple or object holding only such values and known
	// ones. Breaking either is a usage panic naming the function, since the
	// defect is the function author's.
	//
	// Impl and ResultOf see an argument unmarked unless its parameter admits
	// marks, so they cannot know that a redacting mark withheld its content.
	// Where the call removed one, their failures keep their codes and have
	// their messages withheld, located at the call; a function that would
	// quote its argument in a failure admits marks and withholds what they
	// require itself.
	Impl func(args []Value, result Constraint) (Value, error)
}

// Function is a caller-defined operation: a value built once by NewFunction
// and called with Call. The zero Function is not a function.
type Function struct {
	spec *fnSpec
}

// fnSpec is the immutable inside of a Function.
type fnSpec struct {
	name        string
	description string
	params      []Param
	varParam    *Param
	result      Constraint
	resultOf    func(args []Value) (Constraint, error)
	volatile    bool
	impl        func(args []Value, result Constraint) (Value, error)
}

// NewFunction returns the function that spec describes. The specification is
// copied: changing spec, or what its slices hold, after the call changes
// nothing. NewFunction panics if the specification cannot make a function:
// no implementation, no result or two, or a parameter without a constraint.
func NewFunction(spec FunctionSpec) Function {
	if spec.Impl == nil {
		usagePanic("NewFunction: the specification of %s has no implementation", specName(spec.Name))
	}
	switch {
	case spec.Result.c == nil && spec.ResultOf == nil:
		usagePanic("NewFunction: the specification of %s has no result", specName(spec.Name))
	case spec.Result.c != nil && spec.ResultOf != nil:
		usagePanic("NewFunction: the specification of %s has both a result constraint and a derivation", specName(spec.Name))
	}
	for i, prm := range spec.Params {
		if prm.Constraint.c == nil {
			usagePanic("NewFunction: %s of %s has no constraint",
				paramName(i, &spec.Params[i]), specName(spec.Name))
		}
	}
	f := &fnSpec{
		name:        spec.Name,
		description: spec.Description,
		params:      slices.Clone(spec.Params),
		result:      spec.Result,
		resultOf:    spec.ResultOf,
		volatile:    spec.Volatile,
		impl:        spec.Impl,
	}
	if spec.VarParam != nil {
		if spec.VarParam.Constraint.c == nil {
			usagePanic("NewFunction: the variadic parameter of %s has no constraint", specName(spec.Name))
		}
		v := *spec.VarParam
		f.varParam = &v
	}
	return Function{spec: f}
}

// specName names a function being specified, before there is a Function to
// ask.
func specName(name string) string {
	if name == "" {
		return "the function"
	}
	return name
}

// Equal reports whether f and g are one function: the value one NewFunction
// call returned. Nothing else can say what two implementations do, so two
// functions built from equal specifications are not equal.
func (f Function) Equal(g Function) bool { return f.spec == g.spec }

// data returns the function's inside, and panics on the zero Function.
func (f Function) data() *fnSpec {
	if f.spec == nil {
		usagePanic("use of the zero Function")
	}
	return f.spec
}

// Name returns the name the function was specified with, which may be empty.
func (f Function) Name() string { return f.data().name }

// Description returns the description the function was specified with.
func (f Function) Description() string { return f.data().description }

// Params returns the positional parameters, as a copy.
func (f Function) Params() []Param { return slices.Clone(f.data().params) }

// VarParam returns a copy of the variadic parameter, or nil if the function
// takes none.
func (f Function) VarParam() *Param {
	s := f.data()
	if s.varParam == nil {
		return nil
	}
	v := *s.varParam
	return &v
}

// Result returns the constraint the function's result satisfies, and false
// where the function derives it from the arguments instead, which
// ResultConstraint asks per call.
func (f Function) Result() (Constraint, bool) {
	s := f.data()
	return s.result, s.resultOf == nil
}

// Volatile reports whether the function declares its result not to be a
// function of its arguments.
func (f Function) Volatile() bool { return f.data().volatile }

// AsVolatile returns f with volatility declared, its specification otherwise
// unchanged: a function whose implementation is not pure, wrapped by cty's
// Unpredictable on the other side of a migration, is declared this way
// where a specification of one's own cannot be. The result is a new
// function, equal to itself alone.
func (f Function) AsVolatile() Function {
	s := *f.data()
	s.volatile = true
	return Function{spec: &s}
}

// name names the function in messages.
func (s *fnSpec) name_() string { return specName(s.name) }

// param returns the parameter that argument i is bound to.
func (s *fnSpec) param(i int) *Param {
	if i < len(s.params) {
		return &s.params[i]
	}
	return s.varParam
}

// Call calls the function with the given arguments and returns its result.
// Calling is an operation: every failure the arguments cause is an error
// value, never a panic and never a host-language error, and the failures of
// every argument report together, each located by the argument's zero-based
// index (a path such as [1]).
//
// Each argument is converted to its parameter's constraint under the policy
// before the function sees it, so "8080" is a number argument under Unsafe
// and a failure under Safe, as Convert has it. An argument the parameter
// does not admit is answered before the implementation runs: an error
// argument fails the call with its own diagnostics (ER-005, never
// short-circuited), a null argument fails with CodeOperationNullOperand, a
// pending argument answers as every operation answers one, and an argument
// not wholly known makes the result the unknown of the function's result.
// Marked arguments are unmarked for an implementation that does not admit
// marks, and every answer carries the marks that propagate (MK-003). A
// function that derives its result does so first, from the converted
// arguments in the states they stand, and a volatile function answers with
// the unknown of its result even where every argument is known.
//
// Call panics on the zero Function, on a policy that is neither Safe nor
// Unsafe, and where the implementation breaks its contract: a result that
// does not satisfy the function's result constraint, or one that is neither
// known, given nor an error although every argument was known.
func Call(f Function, args []Value, p Policy) Value {
	s := f.data()
	if p != Safe && p != Unsafe {
		usagePanic("Call called with %s, which is neither Safe nor Unsafe", p)
	}
	if len(args) < len(s.params) || (s.varParam == nil && len(args) > len(s.params)) {
		// The arity failure stands alone: the arguments cannot be bound to
		// parameters, so nothing further is asked of them (FN-010).
		return errorValue(s.arity(len(args)))
	}

	pr := s.prepare(args, p)
	if e, ok := pr.ce.value(); ok {
		return pr.finish(e, true)
	}
	rc, failed, ok := s.derive(pr)
	if !ok {
		return pr.finish(failed, true)
	}
	if s.volatile {
		// The result is not a function of the arguments, so known arguments
		// settle nothing: the declaration is the exception UN-008 names.
		return pr.finish(resultPlaceholder(rc), true)
	}
	if pr.pending || pr.unknown {
		return pr.finish(resultPlaceholder(rc), true)
	}

	r, err := s.impl(pr.visible, rc)
	if err != nil {
		return pr.finish(errorValue(s.withheld(pr, implFailure(err))...), false)
	}
	if r.n == nil {
		usagePanic("%s: the implementation returned the zero Value and no error", s.name_())
	}
	switch r.n.state {
	case stateError:
		if redactingOf(pr.g.marks) != nil {
			// The marks the implementation put on its error stay with it.
			r = carryMarks(r, errorValue(s.withheld(pr, r.n.diagnostics())...))
		}
	case statePending:
		// A pending value known to be null, or holding only members that
		// are known or given, is given (UN-008): only its type is open, as
		// a JSON null read with Any is, so known arguments may give it.
		if pr.known && !r.n.given() {
			usagePanic("%s: every argument was known, but the implementation returned %s",
				s.name_(), pr.returned(r.n))
		}
		if _, ok := sharedType(r.n.constraint(), rc); !ok {
			usagePanic("%s: the implementation returned %s, which cannot satisfy its result %s",
				s.name_(), pr.returned(r.n), rc)
		}
	default:
		if pr.known && !r.n.isKnown() {
			usagePanic("%s: every argument was known, but the implementation returned %s",
				s.name_(), pr.returned(r.n))
		}
		if !Satisfies(rc, r.Type()) {
			usagePanic("%s: the implementation returned %s, which does not satisfy its result %s",
				s.name_(), pr.returned(r.n), rc)
		}
	}
	return pr.finish(r, false)
}

// given reports whether nothing about n is open but types (UN-008): n is
// known, or n is pending and known to be null, or n is pending and holds
// members (UN-025) each of which is given. A JSON document read with Any
// gives such a value wherever it says null.
func (n *node) given() bool {
	if n.isKnown() {
		return true
	}
	if n.state != statePending {
		return false
	}
	if n.null == nullOnly {
		return true
	}
	held, ok := n.held()
	if !ok {
		return false
	}
	for _, m := range held.vals {
		if !m.n.given() {
			return false
		}
	}
	return true
}

// returned describes what an implementation returned for a contract panic's
// message. The implementation saw its arguments unmarked, so where the
// boundary removed a redacting mark from one, what it returned may show what
// that mark withholds, its type naming the attributes of a redacted object
// among it; it is described as the answer would have been marked, by the
// redacting marks alone (MK-011).
func (pr *prepared) returned(n *node) string {
	if ms := redactingOf(pr.g.marks); ms != nil {
		return redactedBy(ms)
	}
	return n.describe()
}

// withheld returns the diagnostics of an implementation's or a derivation's
// failure as the call reports them. Those hooks saw their arguments unmarked
// (FN-016), so where the boundary removed a redacting mark from one, they
// could not know what it withholds, and a failure quoting the argument would
// show it: each diagnostic then keeps its code, and its message and path give
// way to one naming the function and the redacting marks, located at the
// call (FN-023, MK-011).
func (s *fnSpec) withheld(pr *prepared, ds []Diagnostic) []Diagnostic {
	ms := redactingOf(pr.g.marks)
	if ms == nil {
		return ds
	}
	message := s.name_() + " failed on " + redactedText(ms) + ", for a reason its redacting marks withhold"
	out := make([]Diagnostic, 0, len(ds))
	for _, d := range ds {
		w := Diagnostic{Code: d.Code, Message: message}
		if !slices.ContainsFunc(out, w.Equal) {
			out = append(out, w)
		}
	}
	return out
}

// ResultConstraint returns the constraint a call of f with these arguments
// would promise its result, without running the implementation, so a host
// can type-check a call before it evaluates: an unknown argument stands for
// one not yet evaluated. The arguments cross the same boundary a call's do,
// and where they fail it, the failure is returned as the call would have
// returned it: wrong arity, a failed conversion, a refused null, an error
// argument, or the derivation's own refusal. ResultConstraint panics as
// Call panics: on the zero Function and on a policy that is neither Safe
// nor Unsafe.
func ResultConstraint(f Function, args []Value, p Policy) (Constraint, *Error) {
	s := f.data()
	if p != Safe && p != Unsafe {
		usagePanic("ResultConstraint called with %s, which is neither Safe nor Unsafe", p)
	}
	if len(args) < len(s.params) || (s.varParam == nil && len(args) > len(s.params)) {
		return Constraint{}, NewError(errorValue(s.arity(len(args))))
	}
	pr := s.prepare(args, p)
	if e, ok := pr.ce.value(); ok {
		return Constraint{}, NewError(pr.finish(e, true))
	}
	rc, failed, ok := s.derive(pr)
	if !ok {
		return Constraint{}, NewError(pr.finish(failed, true))
	}
	return rc, nil
}

// prepared is a call's arguments across the boundary: converted, their
// failures collected, their marks gathered, their states counted.
type prepared struct {
	visible []Value
	ce      containerErrors // the data failures, each located (FN-012)
	g       propagating     // the marks every answer carries (FN-016)
	ga      propagating     // marks of admitted arguments, for answers the implementation does not make
	pending bool            // a pending argument answers the call (FN-014)
	unknown bool            // an argument not wholly known answers the call (FN-015)
	known   bool
}

// prepare takes every argument across the boundary. The arity holds.
func (s *fnSpec) prepare(args []Value, p Policy) *prepared {
	pr := &prepared{visible: make([]Value, len(args)), known: true}
	for i := range args {
		prm := s.param(i)
		n := args[i].data()
		if n.state == stateError {
			pr.ce.add(argStep(i), args[i])
			continue
		}
		// Conversion sees the argument marked, so its diagnostics withhold
		// what a redacting mark requires and its result carries the marks
		// that propagate; what does not propagate stays behind here.
		c := Convert(args[i], prm.Constraint, p)
		if c.n.state == stateError {
			pr.ce.add(argStep(i), c)
			continue
		}
		if !prm.AllowNull && (c.n.state == stateNull || c.n.state == statePending && c.n.null == nullOnly) {
			// The null was consumed in refusing it, so its Propagate marks
			// reach the answer (MK-003), which its diagnostic already made
			// an error value.
			pr.g.gather(c.n, true)
			pr.ce.addDiagnostic(s.nullArgument(i, c.n))
			continue
		}
		if prm.AllowMarked {
			// Propagating is the implementation's to do, but an answer the
			// implementation does not make still carries what MK-003 carries.
			pr.ga.gather(c.n, true)
			pr.visible[i] = c
		} else {
			pr.g.gather(c.n, true)
			pr.visible[i], _ = UnmarkDeep(c)
		}
		switch {
		case c.n.state == statePending:
			pr.known = false
			pr.pending = pr.pending || !prm.AllowPending
		case !c.n.isKnown():
			pr.known = false
			pr.unknown = pr.unknown || !prm.AllowUnknown
		}
	}
	return pr
}

// finish puts on r the marks the answer carries: those gathered from the
// arguments, and, for an answer the implementation did not make, those of
// the admitted arguments too.
func (pr *prepared) finish(r Value, admitted bool) Value {
	if len(pr.g.marks) != 0 {
		r = WithMarks(r, pr.g.marks...)
	}
	if admitted && len(pr.ga.marks) != 0 {
		r = WithMarks(r, pr.ga.marks...)
	}
	return r
}

// derive returns the call's result constraint: the specification's, or what
// its derivation says of the converted arguments, in the states they stand.
// A refusal comes back as the error value the call answers with, and a
// derivation that returns neither a constraint nor an error is the
// function author's defect.
func (s *fnSpec) derive(pr *prepared) (Constraint, Value, bool) {
	if s.resultOf == nil {
		return s.result, Value{}, true
	}
	rc, err := s.resultOf(pr.visible)
	if err != nil {
		return Constraint{}, errorValue(s.withheld(pr, implFailure(err))...), false
	}
	if rc.c == nil {
		usagePanic("%s: the derivation returned the zero Constraint and no error", s.name_())
	}
	return rc, Value{}, true
}

// resultPlaceholder is the answer of a call that its arguments keep from
// running: the unknown of the result where the constraint settles one type,
// and otherwise pending with the result constraint (UN-023). It is not
// narrowed away from null, since a function's result may be null where the
// operations of the value layer never are.
func resultPlaceholder(rc Constraint) Value {
	if rc.Kind() == ConstraintExactly {
		return Unknown(rc.Type())
	}
	return Pending(rc)
}

// argStep is the path step that locates argument i, counting from zero
// across positional and variadic arguments alike.
func argStep(i int) Step {
	return Step{kind: StepIndex, key: NumberFromInt(int64(i))}
}

// argPath is the path that locates argument i.
func argPath(i int) Path {
	return Path{}.extend(argStep(i))
}

// arity returns the diagnostic of a call whose arguments cannot be bound to
// the function's parameters.
func (s *fnSpec) arity(given int) Diagnostic {
	takes := "takes " + countArguments(len(s.params))
	if s.varParam != nil {
		takes = "takes at least " + countArguments(len(s.params))
	}
	were := "none was given"
	switch given {
	case 0:
	case 1:
		were = "1 was given"
	default:
		were = strconv.Itoa(given) + " were given"
	}
	return Diagnostic{
		Code:    CodeFunctionArity,
		Message: s.name_() + " " + takes + ", and " + were,
	}
}

// countArguments counts arguments for the arity message.
func countArguments(n int) string {
	switch n {
	case 0:
		return "no arguments"
	case 1:
		return "1 argument"
	}
	return strconv.Itoa(n) + " arguments"
}

// nullArgument returns the diagnostic of a null argument whose parameter
// does not admit null. Whether a value is null is among what a redacting
// mark withholds (MK-011), so the message names a redacted argument by the
// placeholder, though the code still says why.
func (s *fnSpec) nullArgument(i int, arg *node) Diagnostic {
	what := "null"
	if ms := arg.redactingMarks(); ms != nil {
		what = redactedText(ms)
	}
	return Diagnostic{
		Code:    CodeOperationNullOperand,
		Message: s.argumentName(i) + " of " + s.name_() + " is " + what + ", which " + s.name_() + " cannot use",
		Path:    argPath(i),
	}
}

// argumentName names argument i in a message, by the parameter's name where
// it has one.
func (s *fnSpec) argumentName(i int) string {
	name := "argument " + strconv.Itoa(i+1)
	if prm := s.param(i); prm.Name != "" {
		name += " (" + prm.Name + ")"
	}
	return name
}

// paramName names parameter i of a specification being validated.
func paramName(i int, prm *Param) string {
	if prm.Name != "" {
		return "parameter " + strconv.Itoa(i+1) + " (" + prm.Name + ")"
	}
	return "parameter " + strconv.Itoa(i+1)
}

// implFailure converts an implementation's error into diagnostics: a failure
// that carries diagnostics contributes them as they are, and any other error
// its text, made valid UTF-8 and given a placeholder where empty, under
// CodeFunctionFailed, located at the call. This is the convention every hook
// follows (SE-043 for the serialization hooks).
func implFailure(err error) []Diagnostic {
	var te *Error
	if errors.As(err, &te) && !te.v.IsZero() {
		return te.Diagnostics()
	}
	message := strings.ToValidUTF8(err.Error(), "\U0000FFFD")
	if message == "" {
		message = "the implementation returned an error with no text"
	}
	return []Diagnostic{{Code: CodeFunctionFailed, Message: message}}
}
