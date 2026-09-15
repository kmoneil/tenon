package tenon

// Code is a stable, machine-readable diagnostic code. It names an area and a
// condition joined by a dot, as in "number.divide_by_zero".
type Code string

// The diagnostic codes of the conditions that tenon reports.
const (
	CodeNumberInvalidSyntax Code = "number.invalid_syntax"
	CodeNumberOutOfRange    Code = "number.out_of_range"
	CodeStringInvalidUTF8   Code = "string.invalid_utf8"
)
