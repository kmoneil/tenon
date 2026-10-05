package stdlib

import "github.com/kmoneil/tenon"

// AssertNotNullFunc returns its argument as it is, marks included, and fails
// where the argument is null: a null of a type, or a pending value known to
// be null, with tenon.CodeOperationNullOperand located at the argument. Its
// result is never null, so an argument not known yet answers with itself
// narrowed not null, which a language uses to promise that a value it
// cannot see yet will not be null when it is.
var AssertNotNullFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "AssertNotNull",
	Description: "Returns the given value as it is, failing where it is null.",
	Params: []tenon.Param{{
		Name:         "value",
		Description:  "The value that must not be null.",
		Constraint:   tenon.Any(),
		AllowUnknown: true,
		AllowPending: true,
		AllowMarked:  true,
	}},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		return typeOf(args[0]), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return args[0], nil
	},
})

// typeOf returns the constraint v's type gives: exactly that type, or the
// constraint a pending value carries. A parameter admitting marks hands over
// a marked value, whose type a redacting mark withholds from a reader, so
// the marks come off first: the type is the function's to know, and the
// answer carries them.
func typeOf(v tenon.Value) tenon.Constraint {
	v, _ = tenon.Unmark(v)
	if v.IsPending() {
		return v.Constraint()
	}
	return tenon.Exactly(v.Type())
}

// MakeToFunc returns a function converting its one argument to c, under the
// unsafe policy whatever the call's, since an explicit conversion is what
// the function is for: a language's tostring, tonumber and tolist are
// MakeToFunc of their types. A null converts to a null, so the result may
// be null; a conversion that fails gives its own diagnostics, located at the
// argument, and withholds what a redacting mark on the argument requires,
// since the function sees the argument marked.
func MakeToFunc(c tenon.Constraint) tenon.Function {
	return tenon.NewFunction(tenon.FunctionSpec{
		Name:        "MakeTo",
		Description: "Converts the given value to " + c.String() + ".",
		Params:      []tenon.Param{anything("v", "The value to convert.")},
		Result:      c,
		Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
			return at(0, tenon.Convert(args[0], c, tenon.Unsafe)), nil
		},
	})
}

// at returns v, and where v is an error value, its diagnostics located
// within argument i of the call (FN-030), its marks kept.
func at(i int, v tenon.Value) tenon.Value {
	if !v.IsError() {
		return v
	}
	_, marks := tenon.Unmark(v)
	ds := v.Diagnostics()
	for k, d := range ds {
		path := tenon.Path{}.Index(tenon.NumberFromInt(int64(i)))
		for _, step := range d.Path.Steps() {
			if step.Kind() == tenon.StepAttribute {
				path = path.Attribute(step.Name())
			} else {
				path = path.Index(step.Key())
			}
		}
		ds[k].Path = path
	}
	return tenon.WithMarks(tenon.ErrorVal(ds...), marks...)
}
