package ctytenon

import "github.com/kmoneil/tenon"

// The codes of the diagnostics a value's crossing fails with where tenon has
// no code of its own for the failure. They are in the area cty, which belongs
// to this package rather than to tenon's specification.
const (
	// CodeUnmappedMark is the code of a value carrying a mark the Bridge
	// maps to no mark of the other side.
	CodeUnmappedMark tenon.Code = "cty.unmapped_mark"
	// CodeUnpairedCapsule is the code of a value of a capsule type, or of a
	// type holding one, that the Bridge pairs with no capsule type of the
	// other side, and of a cty capsule value holding what its pair does not:
	// a pointer of another type, or a nil one.
	CodeUnpairedCapsule tenon.Code = "cty.unpaired_capsule"
	// CodeOneOf is the code of a pending value whose constraint holds a
	// OneOf, which no cty type says.
	CodeOneOf tenon.Code = "cty.one_of"
)
