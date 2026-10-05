package stdlib

import (
	"errors"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon"
)

// JSONEncodeFunc writes a value as JSON text: Format's %#v, the value's
// projection (SE-062) but that a string also escapes <, > and &, and the
// line and paragraph separators, as go-cty's does, and a number is written
// positionally, its digits in full. A null, a language's untyped null among
// them, is null, and a tuple or an object whose types wait on such nulls is
// written as it stands. A capsule whose type declares no display form fails
// with tenon.CodeSerializeUnencodableCapsule, located within the value. An
// answer of more than 64 times the value's size, a number counting its
// canonical text, and 64 KiB, fails with tenon.CodeFunctionTooLarge before
// it is made. A value not known yet answers the unknown string beginning
// with as much of its text as what is known settles, where it is known not
// to be null.
var JSONEncodeFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "JSONEncode",
	Description: "Returns a string containing a JSON representation of the given value.",
	Params: []tenon.Param{{
		Name: "val", Description: "The value to encode.", Constraint: tenon.Any(),
		AllowNull: true, AllowUnknown: true, AllowPending: true,
	}},
	Result:  text,
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		v := args[0]
		size := jsonSettled{canonical: true}
		size.value(v)
		length := jsonSettled{}
		length.value(v)
		if limit := 64*size.n + 64<<10; length.n > limit {
			return tooLarge(0, "JSONEncode: the text would be "+strconv.FormatInt(length.n, 10)+" bytes, more than "+
				strconv.FormatInt(limit, 10)+", 64 times the value's size and 64 KiB, the most it makes"), nil
		}
		if length.capsule {
			if _, err := tenon.ProjectJSON(v); err != nil {
				var e *tenon.Error
				if errors.As(err, &e) {
					var ds []tenon.Diagnostic
					for _, d := range e.Diagnostics() {
						if d.Code == tenon.CodeSerializeUnencodableCapsule {
							ds = append(ds, d)
						}
					}
					if len(ds) > 0 {
						return at(0, tenon.ErrorVal(ds...)), nil
					}
				}
			}
		}
		var b strings.Builder
		b.Grow(int(length.n))
		w := jsonSettled{b: &b}
		if w.value(v) {
			return tenon.String(b.String()), nil
		}
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(b.String())), nil
	},
})

// JSONDecodeFunc reads JSON text into a value, as ParseJSON reads it with
// Any(): a number exactly, an array as a tuple and an object as an object,
// and null as a language's untyped null, a pending value known to be null.
// The text is RFC 8259's grammar strictly, so trailing text, a name given
// twice, a number past the range of numbers and a lone surrogate each fail,
// with ParseJSON's codes located at the argument, from the derivation, where
// go-cty's reads past some of them. Text not known yet whose recorded prefix
// begins, after whitespace, with " answers a string, with t or f a bool, with
// - or a digit a number, and with { an object, none of them null, with n
// the null; and with any other character it fails now.
var JSONDecodeFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "JSONDecode",
	Description: "Parses the given string as JSON and returns a value corresponding to what the JSON document describes.",
	Params:      []tenon.Param{stringParam("str", "The JSON text to decode.")},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if !args[0].IsKnown() {
			c, failure := jsonFirst(args[0].Range().StringPrefix())
			if !failure.IsZero() {
				return tenon.Constraint{}, tenon.NewError(failure)
			}
			return c, nil
		}
		v, failure := decodeJSON(args[0].AsString())
		if !failure.IsZero() {
			return tenon.Constraint{}, tenon.NewError(failure)
		}
		return typeOf(v), nil
	},
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		if args[0].IsKnown() {
			v, _ := decodeJSON(args[0].AsString())
			return v, nil
		}
		prefix := strings.TrimLeft(args[0].Range().StringPrefix(), jsonWhitespace)
		switch {
		case prefix == "":
			return unknownOf(rc), nil
		case prefix[0] == 'n':
			return tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly()), nil
		case prefix[0] == '"':
			answer := tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull())
			if known := jsonStringStart(prefix[1:]); known != "" {
				answer = tenon.Narrow(answer, tenon.StringPrefix(known))
			}
			return answer, nil
		}
		return tenon.Narrow(unknownOf(rc), tenon.NotNull()), nil
	},
})

// jsonWhitespace is what RFC 8259 lets stand around a value.
const jsonWhitespace = " \t\n\r"

// decodeJSON reads the JSON text s with Any(), or returns its failure
// located at the argument.
func decodeJSON(s string) (tenon.Value, tenon.Value) {
	v, err := tenon.ParseJSON([]byte(s), tenon.Any(), tenon.Safe)
	if err != nil {
		var e *tenon.Error
		if errors.As(err, &e) {
			return tenon.Value{}, at(0, e.Value())
		}
		return tenon.Value{}, invalid(0, "JSONDecode: "+err.Error())
	}
	return v, tenon.Value{}
}

// jsonFirst returns what JSON text beginning with prefix, after
// whitespace, decodes to: a string, a bool, a number or an object by its
// first character, anything for [, n or nothing yet; or the failure of a
// first character that begins no JSON value.
func jsonFirst(prefix string) (tenon.Constraint, tenon.Value) {
	prefix = strings.TrimLeft(prefix, jsonWhitespace)
	if prefix == "" {
		return tenon.Any(), tenon.Value{}
	}
	switch c := prefix[0]; {
	case c == '"':
		return text, tenon.Value{}
	case c == 't' || c == 'f':
		return boolean, tenon.Value{}
	case c == '-' || '0' <= c && c <= '9':
		return number, tenon.Value{}
	case c == '{':
		return tenon.ObjectWith(map[string]tenon.Field{}, false), tenon.Value{}
	case c == '[' || c == 'n':
		return tenon.Any(), tenon.Value{}
	}
	return tenon.Constraint{}, tenon.ErrorVal(tenon.Diagnostic{
		Code:    tenon.CodeJSONInvalidSyntax,
		Message: "JSONDecode: the text begins " + strconv.QuoteRune([]rune(prefix)[0]) + ", which begins no JSON value",
		Path:    argument(0),
	})
}

// jsonStringStart returns the characters a JSON string's text s, after its
// opening quotation mark, settles: those before its first escape, its
// closing quotation mark or a control character, or its end.
func jsonStringStart(s string) string {
	if i := strings.IndexFunc(s, func(r rune) bool { return r == '\\' || r == '"' || r < 0x20 }); i >= 0 {
		return s[:i]
	}
	return s
}
