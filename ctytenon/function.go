package ctytenon

import (
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/kmoneil/tenon"
)

// FunctionToCty returns f as a cty function, so that a host still evaluating
// with cty, HCL's expression evaluator among them, calls a tenon function in
// place: arguments cross to tenon, tenon's call boundary answers, and the
// result crosses back. The policy is the one every wrapped call converts
// its arguments under.
//
// The cty parameters carry f's names, descriptions and constraints as
// ConstraintToCty carries them, for hosts that read them, and every
// allowance cty has, so that nothing is answered on the cty side: a null,
// unknown, dynamic or marked argument reaches tenon's boundary and is
// answered there, a cty.DynamicVal crossing as a pending value. A host that
// converts arguments before calling, as HCL does, hands the call what cty's
// conversion made of them, and the call converts that under its own policy.
//
// A failing call returns the error cty callers expect: a *tenon.Error
// carrying the diagnostics, each located by its zero-based argument index.
// A defect of the function's author panics in tenon and reaches a cty
// caller as cty's own convention returns it, a function.PanicError: this
// boundary alone converts it.
//
// FunctionToCty fails where a parameter's constraint does not cross, as a
// OneOf does not. A derived result constraint crosses at each call, and a
// call whose derived constraint does not cross fails with the crossing's
// error.
func (b Bridge) FunctionToCty(f tenon.Function, p tenon.Policy) (function.Function, error) {
	params := f.Params()
	cparams := make([]function.Parameter, len(params))
	for i := range params {
		cp, err := b.paramToCty(&params[i])
		if err != nil {
			return function.Function{}, err
		}
		cparams[i] = cp
	}
	var cvar *function.Parameter
	if vp := f.VarParam(); vp != nil {
		cp, err := b.paramToCty(vp)
		if err != nil {
			return function.Function{}, err
		}
		cvar = &cp
	}
	if rc, static := f.Result(); static {
		if _, err := b.ConstraintToCty(rc); err != nil {
			return function.Function{}, err
		}
	}
	return function.New(&function.Spec{
		Description: f.Description(),
		Params:      cparams,
		VarParam:    cvar,
		Type: func(args []cty.Value) (cty.Type, error) {
			targs, err := b.argsFromCty(args)
			if err != nil {
				return cty.NilType, err
			}
			rc, terr := tenon.ResultConstraint(f, targs, p)
			if terr != nil {
				return cty.NilType, terr
			}
			return b.ConstraintToCty(rc)
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			targs, err := b.argsFromCty(args)
			if err != nil {
				return cty.NilVal, err
			}
			r := tenon.Call(f, targs, p)
			if r.IsError() {
				return cty.NilVal, tenon.NewError(r)
			}
			return b.ToCty(r)
		},
	}), nil
}

// paramToCty crosses one parameter. Every allowance is granted on the cty
// side, so that each state crosses and tenon's boundary answers it.
func (b Bridge) paramToCty(prm *tenon.Param) (function.Parameter, error) {
	t, err := b.ConstraintToCty(prm.Constraint)
	if err != nil {
		return function.Parameter{}, err
	}
	return function.Parameter{
		Name:             prm.Name,
		Description:      prm.Description,
		Type:             t,
		AllowNull:        true,
		AllowUnknown:     true,
		AllowDynamicType: true,
		AllowMarked:      true,
	}, nil
}

// argsFromCty crosses a call's arguments.
func (b Bridge) argsFromCty(args []cty.Value) ([]tenon.Value, error) {
	out := make([]tenon.Value, len(args))
	for i, a := range args {
		v, err := b.FromCty(a)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// FunctionFromCty returns a cty function as a tenon Function, for a host
// mid-migration calling what it already has through tenon. Parameters cross
// as ConstraintFromCty crosses their types, and each cty allowance becomes
// the admission it means, AllowDynamicType admitting pending; the result
// derives through the function's own ReturnTypeForValues, and the call runs
// through its own Call, their errors failing the tenon call as every
// implementation failure fails it.
//
// A cty function that answers known arguments with an unknown result, as
// one wrapped by cty's Unpredictable does, breaks the contract a tenon
// function makes: known in, known, given or error out, unless volatility is
// declared. Cross the function beneath the wrapper and declare it with
// [tenon.Function.AsVolatile]. A dynamic null is given, so a function
// answering one, as jsondecode does for a document saying null, keeps the
// contract: it crosses as the pending null tenon reads a JSON null as.
//
// The cty function sees an argument unmarked where its parameter does not
// allow marks, so where tenon's call removed a redacting mark, the
// function's failure keeps its code and has its message withheld, since
// cty functions such as parseint quote their argument.
//
// FunctionFromCty fails where a parameter's type does not cross, as one
// holding an unpaired capsule type does not.
func (b Bridge) FunctionFromCty(f function.Function) (tenon.Function, error) {
	params := f.Params()
	tparams := make([]tenon.Param, len(params))
	for i := range params {
		tp, err := b.paramFromCty(&params[i])
		if err != nil {
			return tenon.Function{}, err
		}
		tparams[i] = tp
	}
	var tvar *tenon.Param
	if vp := f.VarParam(); vp != nil {
		tp, err := b.paramFromCty(vp)
		if err != nil {
			return tenon.Function{}, err
		}
		tvar = &tp
	}
	return tenon.NewFunction(tenon.FunctionSpec{
		Description: f.Description(),
		Params:      tparams,
		VarParam:    tvar,
		ResultOf: func(args []tenon.Value) (tenon.Constraint, error) {
			cargs, err := b.argsToCty(args)
			if err != nil {
				return tenon.Constraint{}, err
			}
			ty, err := f.ReturnTypeForValues(cargs)
			if err != nil {
				return tenon.Constraint{}, err
			}
			return b.ConstraintFromCty(ty)
		},
		Impl: func(args []tenon.Value, _ tenon.Constraint) (tenon.Value, error) {
			cargs, err := b.argsToCty(args)
			if err != nil {
				return tenon.Value{}, err
			}
			r, err := f.Call(cargs)
			if err != nil {
				return tenon.Value{}, err
			}
			return b.FromCty(r)
		},
	}), nil
}

// paramFromCty crosses one parameter, each allowance becoming the admission
// it means.
func (b Bridge) paramFromCty(prm *function.Parameter) (tenon.Param, error) {
	c, err := b.ConstraintFromCty(prm.Type)
	if err != nil {
		return tenon.Param{}, err
	}
	return tenon.Param{
		Name:         prm.Name,
		Description:  prm.Description,
		Constraint:   c,
		AllowNull:    prm.AllowNull,
		AllowUnknown: prm.AllowUnknown,
		AllowPending: prm.AllowDynamicType,
		AllowMarked:  prm.AllowMarked,
	}, nil
}

// argsToCty crosses a call's arguments back.
func (b Bridge) argsToCty(args []tenon.Value) ([]cty.Value, error) {
	out := make([]cty.Value, len(args))
	for i, a := range args {
		v, err := b.ToCty(a)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
