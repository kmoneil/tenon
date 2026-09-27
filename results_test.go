package tenon_test

import (
	"fmt"

	"github.com/kmoneil/tenon"
)

// The tests read what Serialize, Deserialize, ProjectJSON and Unify give as
// the result, the error value a failure holds, and whether there was none.
// These give that, from the *tenon.Error each fails with.

func trySerialize(v tenon.Value) ([]byte, tenon.Value, bool) {
	b, err := tenon.Serialize(v)
	failure, ok := failureOf(err)
	return b, failure, ok
}

func tryDeserialize(data []byte, decoders tenon.Decoders) (tenon.Value, tenon.Value, bool) {
	v, err := tenon.Deserialize(data, decoders)
	failure, ok := failureOf(err)
	return v, failure, ok
}

func tryProjectJSON(v tenon.Value) ([]byte, tenon.Value, bool) {
	b, err := tenon.ProjectJSON(v)
	failure, ok := failureOf(err)
	return b, failure, ok
}

func tryUnify(p tenon.Policy, cs ...tenon.Constraint) (tenon.Constraint, tenon.Value, bool) {
	c, err := tenon.Unify(cs, p)
	failure, ok := failureOf(err)
	return c, failure, ok
}

// failureOf returns the error value that err, a *tenon.Error, holds, and
// false, or the zero Value and true where err is nil. An error of another
// type is a fault in the function that returned it, which failureOf gives as
// an error value of code test.not_tenon_error, so that the test reading it
// fails.
func failureOf(err error) (tenon.Value, bool) {
	if err == nil {
		return tenon.Value{}, true
	}
	if e, ok := err.(*tenon.Error); ok {
		return e.Value(), false
	}
	return tenon.ErrorVal(tenon.Diagnostic{
		Code:    "test.not_tenon_error",
		Message: fmt.Sprintf("the error is a %T, not a *tenon.Error: %v", err, err),
	}), false
}
