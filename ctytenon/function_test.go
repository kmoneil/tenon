package ctytenon_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/ctytenon"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// tenonAdd is the tenon function the ToCty tests wrap.
func tenonAdd() tenon.Function {
	num := tenon.Exactly(tenon.NumberType())
	return tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Add",
		Params: []tenon.Param{{Name: "a", Constraint: num}, {Name: "b", Constraint: num}},
		Result: num,
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.Add(args[0], args[1]), nil
		},
	})
}

func TestFunctionToCty(t *testing.T) {
	cf, err := ctytenon.Bridge{}.FunctionToCty(tenonAdd(), tenon.Safe)
	if err != nil {
		t.Fatal(err)
	}

	// A call crosses in, is answered by tenon's boundary, and crosses out.
	got, err := cf.Call([]cty.Value{cty.NumberIntVal(2), cty.NumberIntVal(3)})
	if err != nil || !got.RawEquals(cty.NumberIntVal(5)) {
		t.Errorf("Call(2, 3) = %v, %v", got, err)
	}

	// The parameters grant cty's every allowance, so tenon answers each
	// state: an unknown argument is the unknown result, a dynamic one the
	// same through pending, and a null one the located refusal cty would
	// have reported alone and unlocated.
	got, err = cf.Call([]cty.Value{cty.UnknownVal(cty.Number), cty.NumberIntVal(3)})
	if err != nil || got.IsKnown() || !got.Type().Equals(cty.Number) {
		t.Errorf("Call(unknown, 3) = %v, %v, want the unknown Number", got, err)
	}
	got, err = cf.Call([]cty.Value{cty.DynamicVal, cty.NumberIntVal(3)})
	if err != nil || got.IsKnown() || !got.Type().Equals(cty.Number) {
		t.Errorf("Call(DynamicVal, 3) = %v, %v, want the unknown Number", got, err)
	}
	_, err = cf.Call([]cty.Value{cty.NullVal(cty.Number), cty.NumberIntVal(3)})
	var te *tenon.Error
	if !errors.As(err, &te) {
		t.Fatalf("Call(null, 3) returned %v, want a *tenon.Error", err)
	}
	d := te.Diagnostics()
	if len(d) != 1 || d[0].Code != tenon.CodeOperationNullOperand || !d[0].Path.Equal(tenon.Path{}.Index(tenon.NumberFromInt(0))) {
		t.Errorf("Call(null, 3) failed with %+v, want %s at [0]", d, tenon.CodeOperationNullOperand)
	}

	// The return type crosses through the derivation path.
	if ty, err := cf.ReturnType([]cty.Type{cty.Number, cty.Number}); err != nil || !ty.Equals(cty.Number) {
		t.Errorf("ReturnType = %v, %v", ty, err)
	}

	// Marks cross in, tenon's rules carry them, and they cross back.
	mf, err := marking.FunctionToCty(tenonAdd(), tenon.Safe)
	if err != nil {
		t.Fatal(err)
	}
	got, err = mf.Call([]cty.Value{cty.NumberIntVal(2).Mark("sensitive"), cty.NumberIntVal(3)})
	if err != nil || !got.HasMark("sensitive") {
		t.Errorf("a sensitive argument gave %v, %v; the mark did not cross back", got, err)
	}
	unmarked, _ := got.Unmark()
	if !unmarked.RawEquals(cty.NumberIntVal(5)) {
		t.Errorf("the marked call answered %v, want 5", unmarked)
	}
	// A mark the Bridge does not map fails the crossing rather than being
	// dropped.
	if _, err := cf.Call([]cty.Value{cty.NumberIntVal(2).Mark("sensitive"), cty.NumberIntVal(3)}); err == nil {
		t.Errorf("an unmapped mark crossed silently")
	}

	// A defect of the function's author panics in tenon and reaches a cty
	// caller as cty's own convention: this boundary alone converts it.
	liar := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Liar",
		Params: []tenon.Param{{Name: "v", Constraint: tenon.Exactly(tenon.NumberType())}},
		Result: tenon.Exactly(tenon.NumberType()),
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.Bool(true), nil
		},
	})
	cl, err := ctytenon.Bridge{}.FunctionToCty(liar, tenon.Safe)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cl.Call([]cty.Value{cty.NumberIntVal(1)})
	var pe function.PanicError
	if !errors.As(err, &pe) || !strings.Contains(err.Error(), "tenon: usage: Liar") {
		t.Errorf("a lying implementation returned %v, want cty's PanicError carrying the usage panic", err)
	}

	// A constraint cty cannot say fails the crossing of the function.
	oneOf := tenon.NewFunction(tenon.FunctionSpec{
		Name:   "Either",
		Params: []tenon.Param{{Name: "v", Constraint: tenon.OneOf(tenon.Exactly(tenon.NumberType()), tenon.Exactly(tenon.StringType()))}},
		Result: tenon.Exactly(tenon.StringType()),
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			return tenon.String("x"), nil
		},
	})
	if _, err := (ctytenon.Bridge{}).FunctionToCty(oneOf, tenon.Safe); err == nil {
		t.Errorf("a OneOf parameter crossed, which no cty type says")
	}
}

func TestFunctionFromCty(t *testing.T) {
	upper, err := ctytenon.Bridge{}.FunctionFromCty(stdlib.UpperFunc)
	if err != nil {
		t.Fatal(err)
	}

	// A real cty function answers through tenon's boundary.
	if got := tenon.Call(upper, []tenon.Value{tenon.String("héllo")}, tenon.Safe); !got.Equal(tenon.String("HÉLLO")) {
		t.Errorf("Upper(héllo) = %v", got)
	}

	// The call converts under tenon's policy where cty's host would have
	// converted on its own: a number is a string argument under Unsafe and
	// a failure under Safe.
	if got := tenon.Call(upper, []tenon.Value{tenon.NumberFromInt(1)}, tenon.Unsafe); !got.Equal(tenon.String("1")) {
		t.Errorf("Upper(1) under Unsafe = %v", got)
	}
	if got := tenon.Call(upper, []tenon.Value{tenon.NumberFromInt(1)}, tenon.Safe); !got.IsError() {
		t.Errorf("Upper(1) under Safe = %v, want the conversion's failure", got)
	}

	// An unknown argument derives the result type through cty's own hook
	// and answers with its unknown; a null is tenon's located refusal,
	// where cty's own would name no argument.
	got := tenon.Call(upper, []tenon.Value{tenon.Unknown(tenon.StringType())}, tenon.Safe)
	if got.IsKnown() || got.IsError() || !got.Type().Equal(tenon.StringType()) {
		t.Errorf("Upper(unknown) = %v, want the unknown String", got)
	}
	got = tenon.Call(upper, []tenon.Value{tenon.Null(tenon.StringType())}, tenon.Safe)
	if !got.IsError() {
		t.Fatalf("Upper(null) = %v, want the refusal", got)
	}
	if d := got.Diagnostics(); d[0].Code != tenon.CodeOperationNullOperand || !d[0].Path.Equal(tenon.Path{}.Index(tenon.NumberFromInt(0))) {
		t.Errorf("Upper(null) failed with %+v, want %s at [0]", d, tenon.CodeOperationNullOperand)
	}

	// cty's allowances cross as the admissions they mean: Coalesce admits
	// nulls and unknowns variadically, and still does through tenon.
	coalesce, err := ctytenon.Bridge{}.FunctionFromCty(stdlib.CoalesceFunc)
	if err != nil {
		t.Fatal(err)
	}
	got = tenon.Call(coalesce, []tenon.Value{tenon.Null(tenon.StringType()), tenon.String("x")}, tenon.Safe)
	if !got.Equal(tenon.String("x")) {
		t.Errorf("Coalesce(null, x) = %v", got)
	}

	// A cty type hook that refuses fails the call as every implementation
	// failure fails it.
	picky := function.New(&function.Spec{
		Params: []function.Parameter{{Name: "v", Type: cty.String}},
		Type: func(args []cty.Value) (cty.Type, error) {
			if args[0].IsKnown() && args[0].AsString() == "no" {
				return cty.NilType, errors.New("not that one")
			}
			return cty.String, nil
		},
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) { return args[0], nil },
	})
	crossed, err := ctytenon.Bridge{}.FunctionFromCty(picky)
	if err != nil {
		t.Fatal(err)
	}
	got = tenon.Call(crossed, []tenon.Value{tenon.String("no")}, tenon.Safe)
	if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeFunctionFailed || got.Diagnostics()[0].Message != "not that one" {
		t.Errorf("a refusing type hook gave %v, want %s saying so", got, tenon.CodeFunctionFailed)
	}

	// A cty function that answers known arguments with an unknown result,
	// as Unpredictable's wrapping does, breaks the contract a tenon
	// function makes; the migration path is the function beneath the
	// wrapper, declared volatile.
	wrapped, err := ctytenon.Bridge{}.FunctionFromCty(function.Unpredictable(stdlib.UpperFunc))
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			r := recover()
			if msg, ok := r.(string); !ok || !strings.HasPrefix(msg, "tenon: usage: ") {
				t.Errorf("recovered %#v, want the contract's usage panic", r)
			}
		}()
		tenon.Call(wrapped, []tenon.Value{tenon.String("x")}, tenon.Safe)
	}()
	volatile := upper.AsVolatile()
	got = tenon.Call(volatile, []tenon.Value{tenon.String("x")}, tenon.Safe)
	if got.IsKnown() || got.IsError() || !got.Type().Equal(tenon.StringType()) {
		t.Errorf("the volatile crossing gave %v, want the unknown String", got)
	}
	if !volatile.Volatile() || upper.Volatile() {
		t.Errorf("AsVolatile changed the wrong function")
	}
}
