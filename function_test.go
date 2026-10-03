package tenon_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// fnAdd returns a function of two numbers for the call-boundary tests. Its
// implementation records whether it ran.
func fnAdd(name string, ran *bool) tenon.Function {
	return tenon.NewFunction(tenon.FunctionSpec{
		Name: name,
		Params: []tenon.Param{
			{Name: "a", Constraint: tenon.Exactly(tenon.NumberType())},
			{Name: "b", Constraint: tenon.Exactly(tenon.NumberType())},
		},
		Result: tenon.Exactly(tenon.NumberType()),
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			if ran != nil {
				*ran = true
			}
			return tenon.Add(args[0], args[1]), nil
		},
	})
}

// fnEcho returns a single-parameter function that hands its argument to probe
// and answers with what probe returns.
func fnEcho(prm tenon.Param, result tenon.Constraint, probe func(tenon.Value) (tenon.Value, error)) tenon.Function {
	return tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Echo",
		Params: []tenon.Param{prm},
		Result: result,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return probe(args[0])
		},
	})
}

func TestConformance_FN001_TheSpecificationMakesTheFunction(t *testing.T) {
	conformance.Covers(t, "FN-001")
	num := tenon.Exactly(tenon.NumberType())
	impl := func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return args[0], nil }

	// A specification that cannot make a function is a usage error naming
	// the defect.
	mustPanicUsage(t, "the specification of Broken has no implementation", func() {
		tenon.NewFunction(tenon.FunctionSpec{Name: "Broken", Result: num})
	})
	mustPanicUsage(t, "the specification of Broken has no result", func() {
		tenon.NewFunction(tenon.FunctionSpec{Name: "Broken", Impl: impl})
	})
	mustPanicUsage(t, "parameter 2 (b) of Broken has no constraint", func() {
		tenon.NewFunction(tenon.FunctionSpec{
			Name:   "Broken",
			Params: []tenon.Param{{Name: "a", Constraint: num}, {Name: "b"}},
			Result: num,
			Impl:   impl,
		})
	})
	mustPanicUsage(t, "the variadic parameter of Broken has no constraint", func() {
		tenon.NewFunction(tenon.FunctionSpec{
			Name:     "Broken",
			VarParam: &tenon.Param{},
			Result:   num,
			Impl:     impl,
		})
	})
	mustPanicUsage(t, "the specification of the function has no implementation", func() {
		tenon.NewFunction(tenon.FunctionSpec{Result: num})
	})

	// The function is built from a copy: changing the specification, or what
	// its slices hold, after NewFunction changes nothing.
	params := []tenon.Param{{Name: "a", Constraint: num}}
	varParam := tenon.Param{Name: "rest", Constraint: num}
	spec := tenon.FunctionSpec{Name: "Keep", Params: params, VarParam: &varParam, Result: num, Impl: impl}
	f := tenon.NewFunction(spec)
	params[0].Name = "changed"
	varParam.Name = "changed"
	if got := f.Params()[0].Name; got != "a" {
		t.Errorf("the function's parameter is named %q after the specification changed, want %q", got, "a")
	}
	if got := f.VarParam().Name; got != "rest" {
		t.Errorf("the function's variadic parameter is named %q after the specification changed, want %q", got, "rest")
	}

	// A variadic function may have no positional parameters at all.
	v := tenon.NewFunction(tenon.FunctionSpec{
		Name:     "Gather",
		VarParam: &tenon.Param{Name: "vals", Constraint: num},
		Result:   num,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.NumberFromInt(int64(len(args))), nil
		},
	})
	if got := tenon.Call(v, nil, tenon.Safe); !got.Equal(tenon.NumberFromInt(0)) {
		t.Errorf("calling a variadic function with no arguments gave %v", got)
	}
	if got := tenon.Call(v, []tenon.Value{tenon.NumberFromInt(1), tenon.NumberFromInt(2)}, tenon.Safe); !got.Equal(tenon.NumberFromInt(2)) {
		t.Errorf("calling a variadic function with two arguments gave %v", got)
	}
}

func TestConformance_FN002_Introspection(t *testing.T) {
	conformance.Covers(t, "FN-002")
	num := tenon.Exactly(tenon.NumberType())
	f := tenon.NewFunction(tenon.FunctionSpec{
		Name:        "Join",
		Description: "joins",
		Params:      []tenon.Param{{Name: "sep", Description: "the separator", Constraint: num}},
		VarParam:    &tenon.Param{Name: "parts", Constraint: num},
		Result:      num,
		Impl:        func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return args[0], nil },
	})
	if f.Name() != "Join" || f.Description() != "joins" {
		t.Errorf("Name %q and Description %q, want Join and joins", f.Name(), f.Description())
	}
	ps := f.Params()
	if len(ps) != 1 || ps[0].Name != "sep" || ps[0].Description != "the separator" {
		t.Errorf("Params() = %+v", ps)
	}
	if vp := f.VarParam(); vp == nil || vp.Name != "parts" {
		t.Errorf("VarParam() = %+v", vp)
	}
	if rc, static := f.Result(); !static || !rc.Equal(num) {
		t.Errorf("Result() = %v, %t, want %v and static", rc, static, num)
	}

	// What introspection returns is a copy.
	ps[0].Name = "changed"
	if got := f.Params()[0].Name; got != "sep" {
		t.Errorf("mutating Params()'s result changed the function: %q", got)
	}
	f.VarParam().Name = "changed"
	if got := f.VarParam().Name; got != "parts" {
		t.Errorf("mutating VarParam()'s result changed the function: %q", got)
	}

	// The zero Function has nothing to introspect or call.
	mustPanicUsage(t, "use of the zero Function", func() { tenon.Function{}.Params() })
	mustPanicUsage(t, "use of the zero Function", func() {
		tenon.Call(tenon.Function{}, nil, tenon.Safe)
	})
}

func TestConformance_FN003_CallsFailAsValues(t *testing.T) {
	conformance.Covers(t, "FN-003")
	f := fnAdd("Add", nil)
	one := tenon.NumberFromInt(1)
	boom := tenon.ErrorVal(tenon.Diagnostic{Code: "app.boom", Message: "boom"})
	for _, tt := range []struct {
		name string
		args []tenon.Value
	}{
		{"wrong arity", []tenon.Value{one}},
		{"error argument", []tenon.Value{boom, one}},
		{"null argument", []tenon.Value{tenon.Null(tenon.NumberType()), one}},
		{"mistyped argument", []tenon.Value{tenon.Bool(true), one}},
		{"unconvertible under Safe", []tenon.Value{tenon.String("8080"), one}},
	} {
		got := tenon.Call(f, tt.args, tenon.Safe)
		if !got.IsError() {
			t.Errorf("%s: Call gave %v, want an error value", tt.name, got)
		}
	}

	// Misusing Call itself stays a usage error: the policy is the caller's
	// program, not the arguments' data.
	mustPanicUsage(t, "Call called with", func() {
		tenon.Call(f, []tenon.Value{one, one}, tenon.Policy(0))
	})
}

func TestConformance_FN010_Arity(t *testing.T) {
	conformance.Covers(t, "FN-010")
	f := fnAdd("Add", nil)
	one := tenon.NumberFromInt(1)
	for _, tt := range []struct {
		name string
		f    tenon.Function
		args []tenon.Value
		msg  string
	}{
		{"too few", f, []tenon.Value{one}, "Add takes 2 arguments, and 1 was given"},
		{"too many", f, []tenon.Value{one, one, one}, "Add takes 2 arguments, and 3 were given"},
		{"none", f, nil, "Add takes 2 arguments, and none was given"},
	} {
		got := tenon.Call(tt.f, tt.args, tenon.Safe)
		want := []tenon.Diagnostic{{Code: tenon.CodeFunctionArity, Message: tt.msg}}
		if !got.IsError() || !equalDiagnostics(got.Diagnostics(), want) {
			t.Errorf("%s: Call gave %v, want the diagnostic %+v", tt.name, got, want[0])
		}
	}

	// A variadic function bounds its arity from below only.
	v := tenon.NewFunction(tenon.FunctionSpec{
		Name:     "Gather",
		Params:   []tenon.Param{{Name: "first", Constraint: tenon.Exactly(tenon.NumberType())}},
		VarParam: &tenon.Param{Name: "rest", Constraint: tenon.Exactly(tenon.NumberType())},
		Result:   tenon.Exactly(tenon.NumberType()),
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.NumberFromInt(int64(len(args))), nil
		},
	})
	got := tenon.Call(v, nil, tenon.Safe)
	want := []tenon.Diagnostic{{Code: tenon.CodeFunctionArity, Message: "Gather takes at least 1 argument, and none was given"}}
	if !got.IsError() || !equalDiagnostics(got.Diagnostics(), want) {
		t.Errorf("variadic: Call gave %v, want the diagnostic %+v", got, want[0])
	}
	if got := tenon.Call(v, []tenon.Value{one, one, one}, tenon.Safe); !got.Equal(tenon.NumberFromInt(3)) {
		t.Errorf("variadic: Call gave %v, want 3", got)
	}

	// The arity failure stands alone: nothing further is asked of arguments
	// that cannot be bound to parameters.
	boom := tenon.ErrorVal(tenon.Diagnostic{Code: "app.boom", Message: "boom"})
	got = tenon.Call(f, []tenon.Value{boom}, tenon.Safe)
	want = []tenon.Diagnostic{{Code: tenon.CodeFunctionArity, Message: "Add takes 2 arguments, and 1 was given"}}
	if !got.IsError() || !equalDiagnostics(got.Diagnostics(), want) {
		t.Errorf("arity with an error argument: Call gave %v, want the arity diagnostic alone", got)
	}

	// An unnamed function is named "the function".
	anon := tenon.NewFunction(tenon.FunctionSpec{
		Result: tenon.Exactly(tenon.NumberType()),
		Impl:   func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return one, nil },
	})
	got = tenon.Call(anon, []tenon.Value{one}, tenon.Safe)
	want = []tenon.Diagnostic{{Code: tenon.CodeFunctionArity, Message: "the function takes no arguments, and 1 was given"}}
	if !got.IsError() || !equalDiagnostics(got.Diagnostics(), want) {
		t.Errorf("unnamed: Call gave %v, want the diagnostic %+v", got, want[0])
	}
}

func TestConformance_FN011_ArgumentsConvert(t *testing.T) {
	conformance.Covers(t, "FN-011")
	var ran bool
	f := fnAdd("Add", &ran)
	one := tenon.NumberFromInt(1)

	// The policy is the call's: a string of digits is a number argument
	// under Unsafe, and a failure under Safe, as Convert has it.
	if got := tenon.Call(f, []tenon.Value{tenon.String("8080"), one}, tenon.Unsafe); !got.Equal(tenon.NumberFromInt(8081)) {
		t.Errorf("under Unsafe the call gave %v, want 8081", got)
	}
	ran = false
	got := tenon.Call(f, []tenon.Value{tenon.String("8080"), one}, tenon.Safe)
	if !got.IsError() || ran {
		t.Fatalf("under Safe the call gave %v (implementation ran: %t), want an error value without running", got, ran)
	}
	diags := got.Diagnostics()
	if len(diags) != 1 || diags[0].Code != tenon.CodeConvertUnsafe || !diags[0].Path.Equal(tenon.Path{}.Index(tenon.NumberFromInt(0))) {
		t.Errorf("under Safe the diagnostics are %+v, want one %s located at [0]", diags, tenon.CodeConvertUnsafe)
	}

	// Every failing argument reports, in argument order.
	got = tenon.Call(f, []tenon.Value{tenon.Bool(true), tenon.String("x")}, tenon.Safe)
	diags = got.Diagnostics()
	if len(diags) != 2 ||
		!diags[0].Path.Equal(tenon.Path{}.Index(tenon.NumberFromInt(0))) ||
		!diags[1].Path.Equal(tenon.Path{}.Index(tenon.NumberFromInt(1))) {
		t.Errorf("two failing arguments gave %+v, want one diagnostic at [0] and one at [1]", diags)
	}
}

func TestConformance_FN012_FailuresCollect(t *testing.T) {
	conformance.Covers(t, "FN-012")
	num := tenon.Exactly(tenon.NumberType())
	var ran bool
	f := tenon.NewFunction(tenon.FunctionSpec{
		Name: "Three",
		Params: []tenon.Param{
			{Name: "a", Constraint: num}, {Name: "b", Constraint: num}, {Name: "c", Constraint: num},
		},
		Result: num,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			ran = true
			return args[0], nil
		},
	})
	boom := tenon.ErrorVal(tenon.Diagnostic{Code: "app.boom", Message: "boom"})

	// An error argument, a failed conversion and a refused null report
	// together, in argument order, each located by its argument.
	got := tenon.Call(f, []tenon.Value{boom, tenon.Bool(true), tenon.Null(tenon.NumberType())}, tenon.Safe)
	if !got.IsError() || ran {
		t.Fatalf("Call gave %v (implementation ran: %t), want an error value without running", got, ran)
	}
	diags := got.Diagnostics()
	if len(diags) != 3 {
		t.Fatalf("Call gave %d diagnostics %+v, want 3", len(diags), diags)
	}
	at := func(i int) tenon.Path { return tenon.Path{}.Index(tenon.NumberFromInt(int64(i))) }
	if diags[0].Code != "app.boom" || !diags[0].Path.Equal(at(0)) {
		t.Errorf("the error argument's diagnostic is %+v, want app.boom at [0]", diags[0])
	}
	if !diags[1].Path.Equal(at(1)) {
		t.Errorf("the conversion's diagnostic is %+v, want it at [1]", diags[1])
	}
	if diags[2].Code != tenon.CodeOperationNullOperand || !diags[2].Path.Equal(at(2)) {
		t.Errorf("the null's diagnostic is %+v, want %s at [2]", diags[2], tenon.CodeOperationNullOperand)
	}

	// One error value given twice is two located failures, not one: the
	// locations differ, so neither is an exact duplicate of the other.
	got = tenon.Call(f, []tenon.Value{boom, boom, tenon.NumberFromInt(1)}, tenon.Safe)
	diags = got.Diagnostics()
	if len(diags) != 2 || !diags[0].Path.Equal(at(0)) || !diags[1].Path.Equal(at(1)) {
		t.Errorf("one error at two arguments gave %+v, want app.boom at [0] and at [1]", diags)
	}
}

func TestConformance_FN013_NullArguments(t *testing.T) {
	conformance.Covers(t, "FN-013")
	f := fnAdd("Add", nil)
	one := tenon.NumberFromInt(1)
	at0 := tenon.Path{}.Index(tenon.NumberFromInt(0))

	got := tenon.Call(f, []tenon.Value{tenon.Null(tenon.NumberType()), one}, tenon.Safe)
	want := []tenon.Diagnostic{{
		Code:    tenon.CodeOperationNullOperand,
		Message: "argument 1 (a) of Add is null, which Add cannot use",
		Path:    at0,
	}}
	if !got.IsError() || !equalDiagnostics(got.Diagnostics(), want) {
		t.Errorf("a refused null gave %v, want the diagnostic %+v", got, want[0])
	}

	// A pending argument already known to be null is refused the same way.
	pendingNull := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	echo := fnEcho(tenon.Param{Name: "v", Constraint: tenon.Any()}, tenon.Exactly(tenon.BoolType()),
		func(v tenon.Value) (tenon.Value, error) { return tenon.Bool(true), nil })
	got = tenon.Call(echo, []tenon.Value{pendingNull}, tenon.Safe)
	if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeOperationNullOperand {
		t.Errorf("a pending null gave %v, want %s", got, tenon.CodeOperationNullOperand)
	}

	// What a redacting mark withholds includes nullness, so the message
	// names the placeholder, though the code still says why.
	secret := stamp{id: "secret", redact: true}
	got = tenon.Call(f, []tenon.Value{tenon.WithMarks(tenon.Null(tenon.NumberType()), secret), one}, tenon.Safe)
	wantMsg := `argument 1 (a) of Add is redacted("secret"), which Add cannot use`
	if !got.IsError() {
		t.Fatalf("a redacted null gave %v, want an error value", got)
	}
	if d := got.Diagnostics()[0]; d.Code != tenon.CodeOperationNullOperand || d.Message != wantMsg {
		t.Errorf("a redacted null's diagnostic is %+v, want message %q", d, wantMsg)
	}

	// AllowNull passes the null to the implementation, which has the
	// meaning for it.
	var saw tenon.Value
	lenient := fnEcho(tenon.Param{Name: "v", Constraint: tenon.Exactly(tenon.NumberType()), AllowNull: true},
		tenon.Exactly(tenon.BoolType()),
		func(v tenon.Value) (tenon.Value, error) { saw = v; return tenon.Bool(v.IsNull()), nil })
	if got := tenon.Call(lenient, []tenon.Value{tenon.Null(tenon.NumberType())}, tenon.Safe); !got.Equal(tenon.Bool(true)) {
		t.Errorf("AllowNull: the call gave %v, want true", got)
	}
	if !saw.IsNull() {
		t.Errorf("AllowNull: the implementation saw %v, want the null", saw)
	}
}

func TestConformance_FN014_PendingArguments(t *testing.T) {
	conformance.Covers(t, "FN-014")
	pending := tenon.Pending(tenon.Any())

	// Where the result constraint settles one type, a pending argument
	// gives the resolved unknown of it (UN-023).
	var ran bool
	f := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Test",
		Params: []tenon.Param{{Name: "v", Constraint: tenon.Any()}},
		Result: tenon.Exactly(tenon.BoolType()),
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			ran = true
			return tenon.Bool(true), nil
		},
	})
	got := tenon.Call(f, []tenon.Value{pending}, tenon.Safe)
	if ran || got.IsPending() || got.IsKnown() || got.IsError() || !got.Type().Equal(tenon.BoolType()) {
		t.Errorf("a pending argument gave %v (implementation ran: %t), want the unknown Bool", got, ran)
	}

	// Where it does not, the answer is pending with the result constraint.
	open := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Open",
		Params: []tenon.Param{{Name: "v", Constraint: tenon.Any()}},
		Result: tenon.ListOf(tenon.Any()),
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.List(tenon.BoolType()), nil
		},
	})
	got = tenon.Call(open, []tenon.Value{pending}, tenon.Safe)
	if !got.IsPending() || !got.Constraint().Equal(tenon.ListOf(tenon.Any())) {
		t.Errorf("a pending argument gave %v, want pending with %v", got, tenon.ListOf(tenon.Any()))
	}

	// AllowPending passes the pending value to an implementation that reads
	// constraints itself.
	var saw tenon.Value
	reader := fnEcho(tenon.Param{Name: "v", Constraint: tenon.Any(), AllowPending: true},
		tenon.Exactly(tenon.BoolType()),
		func(v tenon.Value) (tenon.Value, error) { saw = v; return tenon.Bool(v.IsPending()), nil })
	if got := tenon.Call(reader, []tenon.Value{pending}, tenon.Safe); !got.Equal(tenon.Bool(true)) {
		t.Errorf("AllowPending: the call gave %v, want true", got)
	}
	if !saw.IsPending() {
		t.Errorf("AllowPending: the implementation saw %v, want the pending value", saw)
	}
}

func TestConformance_FN015_UnknownArguments(t *testing.T) {
	conformance.Covers(t, "FN-015")
	var ran bool
	f := fnAdd("Add", &ran)
	one := tenon.NumberFromInt(1)

	// An unknown argument makes the result the unknown of the function's
	// result, and the implementation does not run. A function's result may
	// be null, so the unknown is not narrowed away from it.
	got := tenon.Call(f, []tenon.Value{tenon.Unknown(tenon.NumberType()), one}, tenon.Safe)
	if ran || got.IsKnown() || got.IsError() || got.IsPending() || !got.Type().Equal(tenon.NumberType()) {
		t.Fatalf("an unknown argument gave %v (implementation ran: %t), want the unknown Number", got, ran)
	}
	if !got.Range().AllowsNull() {
		t.Errorf("the unknown answer excludes null, which the implementation might have returned")
	}

	// Wholly known is the bar: a list holding an unknown member is not
	// wholly known, so it answers the same way.
	lists := fnEcho(tenon.Param{Name: "v", Constraint: tenon.ListOf(tenon.Exactly(tenon.BoolType()))},
		tenon.Exactly(tenon.NumberType()),
		func(v tenon.Value) (tenon.Value, error) { return tenon.NumberFromInt(int64(v.Len())), nil })
	got = tenon.Call(lists, []tenon.Value{tenon.List(tenon.BoolType(), tenon.Unknown(tenon.BoolType()))}, tenon.Safe)
	if got.IsKnown() || got.IsError() {
		t.Errorf("a list holding an unknown gave %v, want the unknown Number", got)
	}

	// AllowUnknown passes it, and the implementation answers from the range.
	var saw tenon.Value
	ranged := fnEcho(tenon.Param{Name: "v", Constraint: tenon.Exactly(tenon.NumberType()), AllowUnknown: true},
		tenon.Exactly(tenon.BoolType()),
		func(v tenon.Value) (tenon.Value, error) { saw = v; return tenon.IsNull(v), nil })
	got = tenon.Call(ranged, []tenon.Value{tenon.Unknown(tenon.NumberType())}, tenon.Safe)
	if saw.IsKnown() || saw.IsError() {
		t.Errorf("AllowUnknown: the implementation saw %v, want the unknown argument", saw)
	}
	if got.IsError() {
		t.Errorf("AllowUnknown: the call gave %v", got)
	}
}

func TestConformance_FN016_MarkedArguments(t *testing.T) {
	conformance.Covers(t, "FN-016")
	origin := stamp{id: "origin"}
	aside := stamp{id: "aside", policy: tenon.Isolate}
	one := tenon.NumberFromInt(1)

	// The implementation sees the argument unmarked, and the result carries
	// the marks that propagate; an Isolate mark stays behind.
	var saw tenon.Value
	echo := fnEcho(tenon.Param{Name: "v", Constraint: tenon.Exactly(tenon.NumberType())},
		tenon.Exactly(tenon.NumberType()),
		func(v tenon.Value) (tenon.Value, error) { saw = v; return v, nil })
	got := tenon.Call(echo, []tenon.Value{tenon.WithMarks(one, origin, aside)}, tenon.Safe)
	if tenon.HasMark(saw, origin) || tenon.HasMark(saw, aside) {
		t.Errorf("the implementation saw marks on %v", saw)
	}
	if !tenon.HasMark(got, origin) || tenon.HasMark(got, aside) {
		t.Errorf("the result %v carries the wrong marks: want origin and not aside", got)
	}

	// A mark within the argument propagates too: the implementation reads
	// within what it is given.
	lists := fnEcho(tenon.Param{Name: "v", Constraint: tenon.ListOf(tenon.Exactly(tenon.NumberType()))},
		tenon.Exactly(tenon.NumberType()),
		func(v tenon.Value) (tenon.Value, error) { return tenon.NumberFromInt(int64(v.Len())), nil })
	got = tenon.Call(lists, []tenon.Value{tenon.List(tenon.NumberType(), tenon.WithMarks(one, origin))}, tenon.Safe)
	if !tenon.HasMark(got, origin) {
		t.Errorf("a mark within the argument did not reach the result %v", got)
	}

	// Every answer carries them: the unknown short-circuit and the error.
	f := fnAdd("Add", nil)
	got = tenon.Call(f, []tenon.Value{tenon.WithMarks(tenon.Unknown(tenon.NumberType()), origin), one}, tenon.Safe)
	if got.IsKnown() || !tenon.HasMark(got, origin) {
		t.Errorf("the unknown answer %v does not carry the argument's mark", got)
	}
	got = tenon.Call(f, []tenon.Value{tenon.WithMarks(one, origin), tenon.Null(tenon.NumberType())}, tenon.Safe)
	if !got.IsError() || !tenon.HasMark(got, origin) {
		t.Errorf("the error answer %v does not carry the other argument's mark", got)
	}

	// AllowMarked passes the marked value, and propagating is then the
	// implementation's to do.
	var sawMarked bool
	trusted := fnEcho(tenon.Param{Name: "v", Constraint: tenon.Exactly(tenon.NumberType()), AllowMarked: true},
		tenon.Exactly(tenon.NumberType()),
		func(v tenon.Value) (tenon.Value, error) {
			sawMarked = tenon.HasMark(v, origin)
			unmarked, _ := tenon.Unmark(v)
			return unmarked, nil
		})
	got = tenon.Call(trusted, []tenon.Value{tenon.WithMarks(one, origin)}, tenon.Safe)
	if !sawMarked {
		t.Errorf("AllowMarked: the implementation did not see the mark")
	}
	if tenon.HasMark(got, origin) {
		t.Errorf("AllowMarked: the system propagated %v itself, which is the implementation's to do", got)
	}

	// An answer the implementation does not make still carries an admitted
	// argument's propagating marks (MK-003 binds whoever answers).
	two := tenon.NewFunction(tenon.FunctionSpec{
		Name: "Two",
		Params: []tenon.Param{
			{Name: "a", Constraint: tenon.Exactly(tenon.NumberType()), AllowMarked: true},
			{Name: "b", Constraint: tenon.Exactly(tenon.NumberType())},
		},
		Result: tenon.Exactly(tenon.NumberType()),
		Impl:   func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return args[1], nil },
	})
	got = tenon.Call(two, []tenon.Value{tenon.WithMarks(one, origin), tenon.Unknown(tenon.NumberType())}, tenon.Safe)
	if got.IsKnown() || !tenon.HasMark(got, origin) {
		t.Errorf("the unknown answer %v loses the admitted argument's mark", got)
	}
}

func TestConformance_FN017_TheDecidingOrder(t *testing.T) {
	conformance.Covers(t, "FN-017")
	var ran bool
	num := tenon.Exactly(tenon.NumberType())
	f := tenon.NewFunction(tenon.FunctionSpec{
		Name: "Three",
		Params: []tenon.Param{
			{Name: "a", Constraint: num}, {Name: "b", Constraint: num}, {Name: "c", Constraint: num},
		},
		Result: num,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			ran = true
			return args[0], nil
		},
	})
	one := tenon.NumberFromInt(1)
	boom := tenon.ErrorVal(tenon.Diagnostic{Code: "app.boom", Message: "boom"})
	unknown := tenon.Unknown(tenon.NumberType())
	pending := tenon.Pending(tenon.Any())

	// The failures decide over the pending and the unknown answers, however
	// the arguments are ordered.
	got := tenon.Call(f, []tenon.Value{unknown, pending, boom}, tenon.Safe)
	if !got.IsError() || ran {
		t.Errorf("an error among pending and unknown arguments gave %v (implementation ran: %t)", got, ran)
	}

	// The pending and the unknown answers decide over the implementation.
	got = tenon.Call(f, []tenon.Value{unknown, pending, one}, tenon.Safe)
	if got.IsError() || got.IsKnown() || ran {
		t.Errorf("pending and unknown arguments gave %v (implementation ran: %t), want the unknown Number", got, ran)
	}

	// The implementation runs only where no earlier answer decided.
	if got := tenon.Call(f, []tenon.Value{one, one, one}, tenon.Safe); !ran || !got.Equal(one) {
		t.Errorf("known arguments gave %v (implementation ran: %t)", got, ran)
	}
}

func TestConformance_FN021_TheResultContract(t *testing.T) {
	conformance.Covers(t, "FN-021")
	num := tenon.Exactly(tenon.NumberType())
	one := tenon.NumberFromInt(1)

	lying := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Lying",
		Params: []tenon.Param{{Name: "v", Constraint: num}},
		Result: num,
		Impl:   func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return tenon.Bool(true), nil },
	})
	mustPanicUsage(t, "Lying: the implementation returned a value of type bool, which does not satisfy its result exactly(number)", func() {
		tenon.Call(lying, []tenon.Value{one}, tenon.Safe)
	})

	sneaking := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Sneaking",
		Params: []tenon.Param{{Name: "v", Constraint: num}},
		Result: num,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.Unknown(tenon.NumberType()), nil
		},
	})
	mustPanicUsage(t, "Sneaking: every argument was known, but the implementation returned", func() {
		tenon.Call(sneaking, []tenon.Value{one}, tenon.Safe)
	})
	// With an argument the parameter admits unknown, an unknown result is
	// what UN-007 asks for.
	lenient := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Lenient",
		Params: []tenon.Param{{Name: "v", Constraint: num, AllowUnknown: true}},
		Result: num,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.Unknown(tenon.NumberType()), nil
		},
	})
	if got := tenon.Call(lenient, []tenon.Value{tenon.Unknown(tenon.NumberType())}, tenon.Safe); got.IsKnown() || got.IsError() {
		t.Errorf("an unknown result from an unknown argument gave %v", got)
	}

	empty := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Empty",
		Params: []tenon.Param{{Name: "v", Constraint: num}},
		Result: num,
		Impl:   func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return tenon.Value{}, nil },
	})
	mustPanicUsage(t, "Empty: the implementation returned the zero Value and no error", func() {
		tenon.Call(empty, []tenon.Value{one}, tenon.Safe)
	})
}

func TestConformance_FN023_ImplementationFailures(t *testing.T) {
	conformance.Covers(t, "FN-023")
	num := tenon.Exactly(tenon.NumberType())
	one := tenon.NumberFromInt(1)
	failing := func(err error) tenon.Function {
		return tenon.NewFunction(tenon.FunctionSpec{
			Name:   "Failing",
			Params: []tenon.Param{{Name: "v", Constraint: num}},
			Result: num,
			Impl:   func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return tenon.Value{}, err },
		})
	}

	// A plain error contributes its text under function.failed.
	got := tenon.Call(failing(errors.New("the file is gone")), []tenon.Value{one}, tenon.Safe)
	want := []tenon.Diagnostic{{Code: tenon.CodeFunctionFailed, Message: "the file is gone"}}
	if !got.IsError() || !equalDiagnostics(got.Diagnostics(), want) {
		t.Errorf("a failing implementation gave %v, want the diagnostic %+v", got, want[0])
	}

	// An error with no text gets a placeholder rather than an empty message.
	got = tenon.Call(failing(errors.New("")), []tenon.Value{one}, tenon.Safe)
	if !got.IsError() || got.Diagnostics()[0].Message != "the implementation returned an error with no text" {
		t.Errorf("an empty error gave %v", got)
	}

	// A failure that carries diagnostics contributes them as they are.
	diag := tenon.Diagnostic{Code: "app.gone", Message: "gone", Path: tenon.Path{}.Attribute("f")}
	got = tenon.Call(failing(tenon.NewError(tenon.ErrorVal(diag))), []tenon.Value{one}, tenon.Safe)
	if !got.IsError() || !equalDiagnostics(got.Diagnostics(), []tenon.Diagnostic{diag}) {
		t.Errorf("a *Error gave %v, want its diagnostic as it is", got)
	}

	// An error value returned as the result passes through as any result
	// does.
	erroring := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Erroring",
		Params: []tenon.Param{{Name: "v", Constraint: num}},
		Result: num,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.ErrorVal(tenon.Diagnostic{Code: "app.no", Message: "no"}), nil
		},
	})
	got = tenon.Call(erroring, []tenon.Value{one}, tenon.Safe)
	if !got.IsError() || got.Diagnostics()[0].Code != "app.no" {
		t.Errorf("an error value result gave %v", got)
	}

	// An implementation that panics is the author's defect: the panic
	// reaches the caller as it is, not as a value. Here the defect is a
	// misuse of Add, whose usage panic Call must not catch.
	panicking := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Panicking",
		Params: []tenon.Param{{Name: "v", Constraint: num}},
		Result: num,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.Add(args[0], tenon.Bool(true)), nil
		},
	})
	mustPanicUsage(t, "Add: the second operand is a value of type bool", func() {
		tenon.Call(panicking, []tenon.Value{one}, tenon.Safe)
	})
}

func TestConformance_FN030_DiagnosticsLocateArguments(t *testing.T) {
	conformance.Covers(t, "FN-030")
	str := tenon.Exactly(tenon.StringType())
	at := func(i int) tenon.Path { return tenon.Path{}.Index(tenon.NumberFromInt(int64(i))) }

	// Arguments count from zero, across positional and variadic alike: a
	// failure in the third argument is located at [2], the argument that
	// failed. (go-cty attributes this failure to the wrong index.)
	join := tenon.NewFunction(tenon.FunctionSpec{
		Name:     "Join",
		Params:   []tenon.Param{{Name: "sep", Constraint: str}},
		VarParam: &tenon.Param{Name: "parts", Constraint: str},
		Result:   str,
		Impl:     func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return args[0], nil },
	})
	got := tenon.Call(join, []tenon.Value{tenon.String(","), tenon.String("a"), tenon.Bool(true)}, tenon.Safe)
	diags := got.Diagnostics()
	if len(diags) != 1 || !diags[0].Path.Equal(at(2)) {
		t.Fatalf("a failing third argument gave %+v, want one diagnostic at [2]", diags)
	}

	// The message names the parameter the argument is bound to.
	got = tenon.Call(join, []tenon.Value{tenon.Null(tenon.StringType()), tenon.String("a")}, tenon.Safe)
	want := "argument 1 (sep) of Join is null, which Join cannot use"
	if d := got.Diagnostics()[0]; d.Message != want {
		t.Errorf("the message is %q, want %q", d.Message, want)
	}
	got = tenon.Call(join, []tenon.Value{tenon.String(","), tenon.Null(tenon.StringType())}, tenon.Safe)
	want = "argument 2 (parts) of Join is null, which Join cannot use"
	if d := got.Diagnostics()[0]; d.Message != want {
		t.Errorf("the message is %q, want %q", d.Message, want)
	}

	// An error argument's own locations survive beneath the argument's.
	located := tenon.ErrorVal(tenon.Diagnostic{Code: "app.deep", Message: "deep", Path: tenon.Path{}.Attribute("x")})
	got = tenon.Call(join, []tenon.Value{tenon.String(","), located}, tenon.Safe)
	wantPath := at(1).Attribute("x")
	if d := got.Diagnostics()[0]; !d.Path.Equal(wantPath) {
		t.Errorf("the located diagnostic is at %v, want %v", d.Path, wantPath)
	}
}

// TestFunctionCallIsAnExample keeps the example of defining and calling a
// function honest in miniature until T-2306 writes the documented one.
func TestFunctionCallIsAnExample(t *testing.T) {
	var ran bool
	f := fnAdd("Add", &ran)
	got := tenon.Call(f, []tenon.Value{tenon.NumberFromInt(2), tenon.NumberFromInt(3)}, tenon.Safe)
	if !got.Equal(tenon.NumberFromInt(5)) {
		t.Fatalf("Add(2, 3) = %v", got)
	}
	if s := fmt.Sprint(got); s != "5" {
		t.Errorf("the result displays as %q", s)
	}
}

func TestConformance_FN020_TheResultDerives(t *testing.T) {
	conformance.Covers(t, "FN-020")
	num := tenon.Exactly(tenon.NumberType())
	impl := func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return args[0], nil }

	// Exactly one of the constraint and the derivation.
	mustPanicUsage(t, "the specification of Both has both a result constraint and a derivation", func() {
		tenon.NewFunction(tenon.FunctionSpec{
			Name:     "Both",
			Result:   num,
			ResultOf: func([]tenon.Value) (tenon.Constraint, error) { return num, nil },
			Impl:     impl,
		})
	})

	// The derivation sees the converted arguments in the states they stand,
	// and what it returns is the constraint the call promises.
	var derived, ran int
	ident := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Ident",
		Params: []tenon.Param{{Name: "v", Constraint: tenon.Any()}},
		ResultOf: func(args []tenon.Value) (tenon.Constraint, error) {
			derived++
			if args[0].IsPending() {
				return tenon.Any(), nil
			}
			return tenon.Exactly(args[0].Type()), nil
		},
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			ran++
			return args[0], nil
		},
	})
	if got := tenon.Call(ident, []tenon.Value{tenon.String("x")}, tenon.Safe); !got.Equal(tenon.String("x")) {
		t.Errorf("Ident(\"x\") = %v", got)
	}
	got := tenon.Call(ident, []tenon.Value{tenon.Unknown(tenon.BoolType())}, tenon.Safe)
	if got.IsKnown() || got.IsError() || got.IsPending() || !got.Type().Equal(tenon.BoolType()) {
		t.Errorf("Ident(unknown bool) = %v, want the unknown Bool the derivation promised", got)
	}
	got = tenon.Call(ident, []tenon.Value{tenon.Pending(tenon.Any())}, tenon.Safe)
	if !got.IsPending() || !got.Constraint().Equal(tenon.Any()) {
		t.Errorf("Ident(pending) = %v, want pending with any", got)
	}
	if derived != 3 || ran != 1 {
		t.Errorf("the derivation ran %d times and the implementation %d, want 3 and 1", derived, ran)
	}

	// Its refusal is a data failure, as the implementation's failures are.
	refusing := tenon.NewFunction(tenon.FunctionSpec{
		Name:     "Refusing",
		Params:   []tenon.Param{{Name: "v", Constraint: tenon.Any()}},
		ResultOf: func([]tenon.Value) (tenon.Constraint, error) { return tenon.Constraint{}, errors.New("no shape fits") },
		Impl:     impl,
	})
	got = tenon.Call(refusing, []tenon.Value{tenon.Bool(true)}, tenon.Safe)
	want := []tenon.Diagnostic{{Code: tenon.CodeFunctionFailed, Message: "no shape fits"}}
	if !got.IsError() || !equalDiagnostics(got.Diagnostics(), want) {
		t.Errorf("a refusing derivation gave %v, want the diagnostic %+v", got, want[0])
	}

	// A derivation that answers nothing is the author's defect.
	empty := tenon.NewFunction(tenon.FunctionSpec{
		Name:     "Empty",
		Params:   []tenon.Param{{Name: "v", Constraint: tenon.Any()}},
		ResultOf: func([]tenon.Value) (tenon.Constraint, error) { return tenon.Constraint{}, nil },
		Impl:     impl,
	})
	mustPanicUsage(t, "Empty: the derivation returned the zero Constraint and no error", func() {
		tenon.Call(empty, []tenon.Value{tenon.Bool(true)}, tenon.Safe)
	})

	// A host asks the result constraint before it evaluates, unknowns
	// standing for what it lacks, and the implementation does not run.
	ran = 0
	rc, errv := tenon.ResultConstraint(ident, []tenon.Value{tenon.Unknown(tenon.BoolType())}, tenon.Safe)
	if errv != nil || !rc.Equal(tenon.Exactly(tenon.BoolType())) || ran != 0 {
		t.Errorf("ResultConstraint gave %v, %v (implementation ran %d times)", rc, errv, ran)
	}
	f := fnAdd("Add", nil)
	if rc, errv := tenon.ResultConstraint(f, []tenon.Value{tenon.Unknown(tenon.NumberType()), tenon.NumberFromInt(1)}, tenon.Safe); errv != nil || !rc.Equal(num) {
		t.Errorf("ResultConstraint of a static result gave %v, %v", rc, errv)
	}
	if _, errv := tenon.ResultConstraint(f, []tenon.Value{tenon.NumberFromInt(1)}, tenon.Safe); errv == nil || errv.Diagnostics()[0].Code != tenon.CodeFunctionArity {
		t.Errorf("ResultConstraint with the wrong arity gave %v, want the arity failure", errv)
	}
	if _, errv := tenon.ResultConstraint(f, []tenon.Value{tenon.Bool(true), tenon.NumberFromInt(1)}, tenon.Safe); errv == nil ||
		!errv.Diagnostics()[0].Path.Equal(tenon.Path{}.Index(tenon.NumberFromInt(0))) {
		t.Errorf("ResultConstraint with a failing argument gave %v, want its located failure", errv)
	}
}

func TestConformance_FN022_DeclaredVolatility(t *testing.T) {
	conformance.Covers(t, "FN-022", "UN-008")
	num := tenon.Exactly(tenon.NumberType())
	var ran bool
	fresh := tenon.NewFunction(tenon.FunctionSpec{
		Name:     "Fresh",
		Params:   []tenon.Param{{Name: "seed", Constraint: num}},
		Result:   num,
		Volatile: true,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			ran = true
			return tenon.NumberFromInt(4), nil
		},
	})
	if !fresh.Volatile() {
		t.Errorf("Volatile() = false for a volatile specification")
	}

	// Known arguments settle nothing: the result is the unknown of the
	// result constraint, the declared exception to known in, known out.
	got := tenon.Call(fresh, []tenon.Value{tenon.NumberFromInt(1)}, tenon.Safe)
	if ran || got.IsKnown() || got.IsError() || got.IsPending() || !got.Type().Equal(tenon.NumberType()) {
		t.Errorf("a volatile call gave %v (implementation ran: %t), want the unknown Number", got, ran)
	}

	// The boundary still comes first: failures fail, and marks carry.
	boom := tenon.ErrorVal(tenon.Diagnostic{Code: "app.boom", Message: "boom"})
	if got := tenon.Call(fresh, []tenon.Value{boom}, tenon.Safe); !got.IsError() {
		t.Errorf("a volatile call with an error argument gave %v", got)
	}
	origin := stamp{id: "origin"}
	got = tenon.Call(fresh, []tenon.Value{tenon.WithMarks(tenon.NumberFromInt(1), origin)}, tenon.Safe)
	if got.IsKnown() || !tenon.HasMark(got, origin) {
		t.Errorf("a volatile call's answer %v does not carry the argument's mark", got)
	}

	// Volatility and a derived result compose: the unknown is of what the
	// derivation says.
	volatileIdent := tenon.NewFunction(tenon.FunctionSpec{
		Name:     "FreshIdent",
		Params:   []tenon.Param{{Name: "v", Constraint: tenon.Any()}},
		Volatile: true,
		ResultOf: func(args []tenon.Value) (tenon.Constraint, error) { return tenon.Exactly(args[0].Type()), nil },
		Impl:     func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) { return args[0], nil },
	})
	got = tenon.Call(volatileIdent, []tenon.Value{tenon.Bool(true)}, tenon.Safe)
	if got.IsKnown() || got.IsError() || !got.Type().Equal(tenon.BoolType()) {
		t.Errorf("a volatile derived call gave %v, want the unknown Bool", got)
	}

	// ResultConstraint answers for a volatile function as for any other:
	// the promise is the constraint, volatility is about the value.
	if rc, errv := tenon.ResultConstraint(fresh, []tenon.Value{tenon.NumberFromInt(1)}, tenon.Safe); errv != nil || !rc.Equal(num) {
		t.Errorf("ResultConstraint of a volatile function gave %v, %v", rc, errv)
	}

	// AsVolatile declares it after the fact, on a new function, the one it
	// was asked of unchanged.
	quiet := fnAdd("Add", nil)
	loud := quiet.AsVolatile()
	if quiet.Volatile() || !loud.Volatile() || loud.Equal(quiet) {
		t.Errorf("AsVolatile gave Volatile()=%t over %t, Equal=%t", loud.Volatile(), quiet.Volatile(), loud.Equal(quiet))
	}
	if got := tenon.Call(loud, []tenon.Value{tenon.NumberFromInt(1), tenon.NumberFromInt(2)}, tenon.Safe); got.IsKnown() {
		t.Errorf("the declared function answered %v from known arguments", got)
	}
}

// BenchmarkCallArguments measures a call whose arguments carry the work, at
// a size and at four times it, so the growth job holds a call's cost to the
// size of its arguments: a wide list and a wide map, each converted to its
// parameter's constraint, unmarked, and read by the implementation.
func BenchmarkCallArguments(b *testing.B) {
	for _, size := range []int{1000, 4000} {
		elems := make([]tenon.Value, size)
		for i := range elems {
			elems[i] = tenon.Object(map[string]tenon.Value{
				"name":  tenon.String(fmt.Sprintf("r%05d", i)),
				"count": tenon.NumberFromInt(int64(i)),
			})
		}
		entries := make(map[string]tenon.Value, size)
		for i := range size {
			entries[fmt.Sprintf("k%05d", i)] = tenon.NumberFromInt(int64(i))
		}
		for _, shape := range []struct {
			name string
			arg  tenon.Value
		}{
			{"list", tenon.List(elems[0].Type(), elems...)},
			{"map", tenon.Map(tenon.NumberType(), entries)},
		} {
			count := tenon.NewFunction(tenon.FunctionSpec{
				Name:   "Count",
				Params: []tenon.Param{{Name: "of", Constraint: tenon.Exactly(shape.arg.Type())}},
				Result: tenon.Exactly(tenon.NumberType()),
				Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
					return tenon.Length(args[0]), nil
				},
			})
			args := []tenon.Value{shape.arg}
			if got := tenon.Call(count, args, tenon.Safe); !got.Equal(tenon.NumberFromInt(int64(size))) {
				b.Fatalf("%s at %d: Call gave %v", shape.name, size, got)
			}
			b.Run(fmt.Sprintf("%s/%d", shape.name, size), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					tenon.Call(count, args, tenon.Safe)
				}
			})
		}
	}
}
