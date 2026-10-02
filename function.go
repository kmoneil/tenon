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
	// Result is the constraint the call's result satisfies. It is required.
	Result Constraint
	// Impl is the function's behavior, given the converted arguments and the
	// result constraint the call promised. It is required. A failure is
	// returned as an error, which becomes diagnostics (a *Error contributes
	// its own as they are, any other error its text under
	// CodeFunctionFailed), or equally as an error value. What it returns
	// must satisfy the result constraint, and must be known or an error
	// where every argument was known; breaking either is a usage panic
	// naming the function, since the defect is the function author's.
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
	impl        func(args []Value, result Constraint) (Value, error)
}

// NewFunction returns the function that spec describes. The specification is
// copied: changing spec, or what its slices hold, after the call changes
// nothing. NewFunction panics if the specification cannot make a function:
// no implementation, no result, or a parameter without a constraint.
func NewFunction(spec FunctionSpec) Function {
	if spec.Impl == nil {
		usagePanic("NewFunction: the specification of %s has no implementation", specName(spec.Name))
	}
	if spec.Result.c == nil {
		usagePanic("NewFunction: the specification of %s has no result constraint", specName(spec.Name))
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

// Result returns the constraint the function's result satisfies.
func (f Function) Result() Constraint { return f.data().result }

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
// marks, and every answer carries the marks that propagate (MK-003).
//
// Call panics on the zero Function, on a policy that is neither Safe nor
// Unsafe, and where the implementation breaks its contract: a result that
// does not satisfy the function's result constraint, or one that is not
// known and not an error although every argument was known.
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

	var (
		ce      containerErrors // the data failures, each located (FN-012)
		g       propagating     // the marks every answer carries (FN-016)
		ga      propagating     // marks of admitted arguments, for answers the implementation does not make
		visible = make([]Value, len(args))
		pending bool // a pending argument answers the call (FN-014)
		unknown bool // an argument not wholly known answers the call (FN-015)
		known   = true
	)
	for i := range args {
		prm := s.param(i)
		n := args[i].data()
		if n.state == stateError {
			ce.add(argStep(i), args[i])
			continue
		}
		// Conversion sees the argument marked, so its diagnostics withhold
		// what a redacting mark requires and its result carries the marks
		// that propagate; what does not propagate stays behind here.
		c := Convert(args[i], prm.Constraint, p)
		if c.n.state == stateError {
			ce.add(argStep(i), c)
			continue
		}
		if !prm.AllowNull && (c.n.state == stateNull || c.n.state == statePending && c.n.null == nullOnly) {
			ce.addDiagnostic(s.nullArgument(i, c.n))
			continue
		}
		if prm.AllowMarked {
			// Propagating is the implementation's to do, but an answer the
			// implementation does not make still carries what MK-003 carries.
			ga.gather(c.n, true)
			visible[i] = c
		} else {
			g.gather(c.n, true)
			visible[i], _ = UnmarkDeep(c)
		}
		switch {
		case c.n.state == statePending:
			known = false
			pending = pending || !prm.AllowPending
		case !c.n.isKnown():
			known = false
			unknown = unknown || !prm.AllowUnknown
		}
	}

	finish := func(r Value, admitted bool) Value {
		if len(g.marks) != 0 {
			r = WithMarks(r, g.marks...)
		}
		if admitted && len(ga.marks) != 0 {
			r = WithMarks(r, ga.marks...)
		}
		return r
	}

	if e, ok := ce.value(); ok {
		return finish(e, true)
	}
	if pending || unknown {
		return finish(resultPlaceholder(s.result), true)
	}

	r, err := s.impl(visible, s.result)
	if err != nil {
		return finish(errorValue(implFailure(err)...), false)
	}
	if r.n == nil {
		usagePanic("%s: the implementation returned the zero Value and no error", s.name_())
	}
	switch r.n.state {
	case stateError:
	case statePending:
		if known {
			usagePanic("%s: every argument was known, but the implementation returned %s",
				s.name_(), r.n.describe())
		}
		if _, ok := sharedType(r.n.constraint(), s.result); !ok {
			usagePanic("%s: the implementation returned %s, which cannot satisfy its result %s",
				s.name_(), r.n.describe(), s.result)
		}
	default:
		if known && !r.n.isKnown() {
			usagePanic("%s: every argument was known, but the implementation returned %s",
				s.name_(), r.n.describe())
		}
		if !Satisfies(s.result, r.Type()) {
			usagePanic("%s: the implementation returned %s, which does not satisfy its result %s",
				s.name_(), r.n.describe(), s.result)
		}
	}
	return finish(r, false)
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
