package tenon

// Code is a stable, machine-readable diagnostic code. It names an area and a
// condition joined by a dot, as in "number.divide_by_zero".
type Code string

// The diagnostic codes of the conditions that the specification names. Every
// code tenon reports is one of these; a caller minting its own follows the
// same shape, as in "myapp.unknown_setting".
const (
	CodeBoolInvalidSyntax           Code = "bool.invalid_syntax"
	CodeConvertLengthMismatch       Code = "convert.length_mismatch"
	CodeConvertMissingAttribute     Code = "convert.missing_attribute"
	CodeConvertNoCommonType         Code = "convert.no_common_type"
	CodeConvertNoConversion         Code = "convert.no_conversion"
	CodeConvertUnexpectedAttribute  Code = "convert.unexpected_attribute"
	CodeConvertUnsafe               Code = "convert.unsafe"
	CodeMapDuplicateKey             Code = "map.duplicate_key"
	CodeNumberDivideByZero          Code = "number.divide_by_zero"
	CodeNumberInvalidSyntax         Code = "number.invalid_syntax"
	CodeNumberModuloByZero          Code = "number.modulo_by_zero"
	CodeNumberOutOfRange            Code = "number.out_of_range"
	CodeOperationNullOperand        Code = "operation.null_operand"
	CodeOperationWrongType          Code = "operation.wrong_type"
	CodeRangeContradiction          Code = "range.contradiction"
	CodeSerializeMalformed          Code = "serialize.malformed"
	CodeSerializeNotCanonical       Code = "serialize.not_canonical"
	CodeSerializeNotKnown           Code = "serialize.not_known"
	CodeSerializeRedacted           Code = "serialize.redacted"
	CodeSerializeTooLarge           Code = "serialize.too_large"
	CodeSerializeUnencodableCapsule Code = "serialize.unencodable_capsule"
	CodeSerializeUnencodableMark    Code = "serialize.unencodable_mark"
	CodeSerializeUnknownCapsule     Code = "serialize.unknown_capsule"
	CodeSerializeUnknownMark        Code = "serialize.unknown_mark"
	CodeSerializeUnsupportedVersion Code = "serialize.unsupported_version"
	CodeStringInvalidUTF8           Code = "string.invalid_utf8"
	CodeUnifyNoCommonConstraint     Code = "unify.no_common_constraint"
)
