package stdlib

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/numbers"
)

// jsonText returns the JSON text of the wholly known value v, carrying no
// mark, as the library writes JSON: SE-062's projection, but for two
// things go-cty's consumers rely on. A string escapes <, > and &, and the
// line and paragraph separators, besides what JSON requires, as
// encoding/json does, with lowercase hexadecimal; and a number is written
// positionally, its digits in full whatever its exponent. There is no
// whitespace between tokens.
func jsonText(v tenon.Value) string {
	var b strings.Builder
	writeJSON(&b, v)
	return b.String()
}

// writeJSON writes the JSON text of v to b.
func writeJSON(b *strings.Builder, v tenon.Value) {
	if v.IsNull() {
		b.WriteString("null")
		return
	}
	switch t := v.Type(); t.Kind() {
	case tenon.KindBool:
		b.WriteString(strconv.FormatBool(v.AsBool()))
	case tenon.KindNumber:
		b.WriteString(positional(v))
	case tenon.KindString:
		writeJSONString(b, v.AsString())
	case tenon.KindList, tenon.KindSet, tenon.KindTuple:
		b.WriteByte('[')
		for i, e := range v.Elements() {
			if i > 0 {
				b.WriteByte(',')
			}
			writeJSON(b, e)
		}
		b.WriteByte(']')
	case tenon.KindMap:
		b.WriteByte('{')
		i := 0
		for k, e := range v.MapEntries() {
			if i > 0 {
				b.WriteByte(',')
			}
			i++
			writeJSONString(b, k)
			b.WriteByte(':')
			writeJSON(b, e)
		}
		b.WriteByte('}')
	case tenon.KindObject:
		b.WriteByte('{')
		i := 0
		for name, e := range v.Attributes() {
			if i > 0 {
				b.WriteByte(',')
			}
			i++
			writeJSONString(b, name)
			b.WriteByte(':')
			writeJSON(b, e)
		}
		b.WriteByte('}')
	default:
		// A capsule is the display form its type declares, as a string,
		// as SE-062 projects it.
		projected, err := tenon.ProjectJSON(v)
		var s string
		if err != nil || json.Unmarshal(projected, &s) != nil {
			s = v.String()
		}
		writeJSONString(b, s)
	}
}

// writeJSONString writes s as a JSON string to b, escaping as encoding/json
// does: the quotation mark and the reverse solidus, the controls below
// U+0020, as \b, \f, \n, \r, \t or \u00XX, the characters <, > and &, and
// the line and paragraph separators.
func writeJSONString(b *strings.Builder, s string) {
	const hex = "0123456789abcdef"
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == '<' || r == '>' || r == '&':
			b.WriteString(`\u00`)
			b.WriteByte(hex[r>>4])
			b.WriteByte(hex[r&0xF])
		case r == 0x2028 || r == 0x2029:
			b.WriteString(`\u202`)
			b.WriteByte(hex[r&0xF])
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}

// positional returns the known number v written positionally: its digits
// in full, a point where it has a fraction, a minus sign where negative,
// never an exponent.
func positional(v tenon.Value) string {
	small, coefficient, exp := numbers.Dec(v).Parts()
	if coefficient == nil {
		coefficient = big.NewInt(small)
	}
	digits := new(big.Int).Abs(coefficient).String()
	sign := ""
	if coefficient.Sign() < 0 {
		sign = "-"
	}
	switch {
	case digits == "0":
		return "0"
	case exp >= 0:
		return sign + digits + strings.Repeat("0", int(exp))
	}
	point := len(digits) + int(exp)
	if point <= 0 {
		return sign + "0." + strings.Repeat("0", -point) + strings.TrimRight(digits, "0")
	}
	if fraction := strings.TrimRight(digits[point:], "0"); fraction != "" {
		return sign + digits[:point] + "." + fraction
	}
	return sign + digits[:point]
}
