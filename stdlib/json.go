package stdlib

import (
	"encoding/json"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

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

// jsonSettled writes the JSON text of a value as far as what is known of
// it settles the text, and counts its bytes: to b where b is not nil, and
// in n either way, each number in its canonical text where canonical (for
// a bound's measure, never written so) and positionally otherwise.
type jsonSettled struct {
	b         *strings.Builder
	n         int64
	canonical bool
	capsule   bool // whether a capsule was written
}

func (w *jsonSettled) text(s string) {
	if w.b != nil {
		w.b.WriteString(s)
	}
	w.n += int64(len(s))
}

// value writes v, carrying no mark, and reports whether its text is whole:
// a null is null; a known value is its text; a tuple or an object, a pending
// one holding its members among them, a list or a map, holding members not
// known, are their text up to the first member whose text is not whole, and
// that member's settled part; a set holding a member not known is [ alone,
// since that member may come first in the canonical order; and a value
// with no members to read that is known not to be null is what its type
// settles, " for a string, [ for a list, a set or a tuple, and { for a map
// or an object. A value that may yet be null settles nothing.
func (w *jsonSettled) value(v tenon.Value) bool {
	switch {
	case v.IsNull():
		w.text("null")
		return true
	case v.IsKnown():
		if w.b != nil {
			start := w.b.Len()
			writeJSON(w.b, v)
			w.n += int64(w.b.Len() - start)
		} else {
			w.n += jsonLen(v, w.canonical)
		}
		w.capsule = w.capsule || holdsCapsule(v)
		return true
	}
	if n := tenon.IsNull(v); !n.IsKnown() || n.AsBool() {
		return false
	}
	var kinds []tenon.Kind
	if v.IsPending() {
		kinds = kindsOf(v.Constraint())
	} else {
		kinds = []tenon.Kind{v.Type().Kind()}
	}
	if v.HasMembers() {
		switch kinds[0] {
		case tenon.KindList, tenon.KindTuple:
			w.text("[")
			for i, e := range v.Elements() {
				if i > 0 {
					w.text(",")
				}
				if !w.value(e) {
					return false
				}
			}
			w.text("]")
			return true
		case tenon.KindMap:
			w.text("{")
			i := 0
			for k, e := range v.MapEntries() {
				if i > 0 {
					w.text(",")
				}
				i++
				w.jsonString(k)
				w.text(":")
				if !w.value(e) {
					return false
				}
			}
			w.text("}")
			return true
		case tenon.KindObject:
			w.text("{")
			i := 0
			for name, e := range v.Attributes() {
				if i > 0 {
					w.text(",")
				}
				i++
				w.jsonString(name)
				w.text(":")
				if !w.value(e) {
					return false
				}
			}
			w.text("}")
			return true
		}
	}
	switch {
	case all(kinds, tenon.KindString):
		w.text(`"`)
	case all(kinds, tenon.KindList, tenon.KindSet, tenon.KindTuple):
		w.text("[")
	case all(kinds, tenon.KindMap, tenon.KindObject):
		w.text("{")
	}
	return false
}

// jsonString writes s as a JSON string.
func (w *jsonSettled) jsonString(s string) {
	if w.b != nil {
		writeJSONString(w.b, s)
	}
	w.n += jsonStringLen(s)
}

// all reports whether every kind of ks is one of those given.
func all(ks []tenon.Kind, of ...tenon.Kind) bool {
	for _, k := range ks {
		if !slices.Contains(of, k) {
			return false
		}
	}
	return len(ks) > 0
}

// holdsCapsule reports whether the known value v is or holds a capsule.
func holdsCapsule(v tenon.Value) bool {
	if v.IsNull() {
		return false
	}
	switch v.Type().Kind() {
	case tenon.KindCapsule:
		return true
	case tenon.KindList, tenon.KindSet, tenon.KindTuple:
		return slices.ContainsFunc(v.Elements(), holdsCapsule)
	case tenon.KindMap:
		for _, e := range v.MapEntries() {
			if holdsCapsule(e) {
				return true
			}
		}
	case tenon.KindObject:
		for _, e := range v.Attributes() {
			if holdsCapsule(e) {
				return true
			}
		}
	}
	return false
}

// jsonLen returns how many bytes the JSON text of the known value v
// carrying no mark has, without writing it: as jsonText writes it, or, where
// canonical, with each number in its canonical text (NU-020).
func jsonLen(v tenon.Value, canonical bool) int64 {
	if v.IsNull() {
		return 4
	}
	switch t := v.Type(); t.Kind() {
	case tenon.KindBool:
		return int64(len(strconv.FormatBool(v.AsBool())))
	case tenon.KindNumber:
		if canonical {
			return int64(len(v.String()))
		}
		return positionalLen(v)
	case tenon.KindString:
		return jsonStringLen(v.AsString())
	case tenon.KindList, tenon.KindSet, tenon.KindTuple:
		n := int64(2)
		for i, e := range v.Elements() {
			if i > 0 {
				n++
			}
			n += jsonLen(e, canonical)
		}
		return n
	case tenon.KindMap:
		n, i := int64(2), 0
		for k, e := range v.MapEntries() {
			if i > 0 {
				n++
			}
			i++
			n += jsonStringLen(k) + 1 + jsonLen(e, canonical)
		}
		return n
	case tenon.KindObject:
		n, i := int64(2), 0
		for name, e := range v.Attributes() {
			if i > 0 {
				n++
			}
			i++
			n += jsonStringLen(name) + 1 + jsonLen(e, canonical)
		}
		return n
	}
	var b strings.Builder
	writeJSON(&b, v)
	return int64(b.Len())
}

// jsonStringLen returns how many bytes writeJSONString writes of s.
func jsonStringLen(s string) int64 {
	n := int64(2)
	for _, r := range s {
		switch {
		case r == '"' || r == '\\' || r == '\b' || r == '\f' || r == '\n' || r == '\r' || r == '\t':
			n += 2
		case r < 0x20 || r == '<' || r == '>' || r == '&' || r == 0x2028 || r == 0x2029:
			n += 6
		default:
			n += int64(utf8.RuneLen(r))
		}
	}
	return n
}

// positionalLen returns how many bytes positional writes of the known
// number v, without writing them.
func positionalLen(v tenon.Value) int64 {
	small, coefficient, exp := numbers.Dec(v).Parts()
	if coefficient == nil {
		coefficient = big.NewInt(small)
	}
	digits := new(big.Int).Abs(coefficient).String()
	sign := int64(0)
	if coefficient.Sign() < 0 {
		sign = 1
	}
	switch {
	case digits == "0":
		return 1
	case exp >= 0:
		return sign + int64(len(digits)) + exp
	}
	point := int64(len(digits)) + exp
	if point <= 0 {
		return sign + 2 - point + int64(len(strings.TrimRight(digits, "0")))
	}
	if fraction := strings.TrimRight(digits[point:], "0"); fraction != "" {
		return sign + point + 1 + int64(len(fraction))
	}
	return sign + point
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
