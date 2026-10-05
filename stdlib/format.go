package stdlib

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/numbers"
	"github.com/kmoneil/tenon/internal/uni"
)

// formatBound is the greatest width or precision a verb of Format may give
// (LB-031), NU-024's number.
const formatBound = 10000

// verb is a verb of a format string.
type verb struct {
	// offset is the byte offset of its %, and text what it is written as.
	offset int
	text   string
	// The flags.
	minus, plus, space, zero, sharp bool
	// width and precision, each where has says it is given.
	width, precision       int
	hasWidth, hasPrecision bool
	// arg is the argument it reads, counting the format as 0.
	arg    int
	letter byte
}

// formatPiece is literal text of a format string, or a verb.
type formatPiece struct {
	literal string
	verb    *verb
}

// formatSyntax returns the failure of a format string that is no format.
func formatSyntax(message string) tenon.Value {
	return tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeFormatInvalidSyntax, Message: "Format: " + message, Path: argument(0)})
}

// formatVerbs is the letters of the verbs Format has.
const formatVerbs = "vtbdoxXeEfgGsq"

// parseFormat reads the format string f: literal text, %% for a percent
// sign, and verbs, each % then its flags, width, precision, argument index
// and letter, in that order, its argument the one its index names or the
// one after the last verb's. It fails with tenon.CodeFormatInvalidSyntax
// where f is no format, and tenon.CodeFunctionTooLarge where a width or a
// precision passes formatBound.
func parseFormat(f string) ([]formatPiece, tenon.Value) {
	var pieces []formatPiece
	var literal strings.Builder
	next := 1
	for i := 0; i < len(f); {
		c := f[i]
		if c != '%' {
			literal.WriteByte(c)
			i++
			continue
		}
		if i+1 < len(f) && f[i+1] == '%' {
			literal.WriteByte('%')
			i += 2
			continue
		}
		v := &verb{offset: i}
		j := i + 1
	flags:
		for ; j < len(f); j++ {
			switch f[j] {
			case '0':
				v.zero = true
			case '#':
				v.sharp = true
			case '-':
				v.minus = true
			case '+':
				v.plus = true
			case ' ':
				v.space = true
			default:
				break flags
			}
		}
		number := func(at int) (n, end int, ok bool) {
			for end = at; end < len(f) && f[end] >= '0' && f[end] <= '9'; end++ {
				if n <= formatBound {
					n = n*10 + int(f[end]-'0')
				}
			}
			return n, end, n <= formatBound
		}
		if j < len(f) && f[j] >= '1' && f[j] <= '9' {
			var ok bool
			if v.width, j, ok = number(j); !ok {
				return nil, tooLarge(0, "Format: the width of the verb at byte "+strconv.Itoa(i)+" passes "+strconv.Itoa(formatBound)+", the most a verb gives")
			}
			v.hasWidth = true
		}
		if j < len(f) && f[j] == '.' {
			var ok bool
			if v.precision, j, ok = number(j + 1); !ok {
				return nil, tooLarge(0, "Format: the precision of the verb at byte "+strconv.Itoa(i)+" passes "+strconv.Itoa(formatBound)+", the most a verb gives")
			}
			v.hasPrecision = true
		}
		v.arg = next
		if j < len(f) && f[j] == '[' {
			if j+1 >= len(f) || f[j+1] < '1' || f[j+1] > '9' {
				return nil, formatSyntax("the argument index at byte " + strconv.Itoa(j) + " is not a whole number from 1")
			}
			n, end := 0, j+1
			for ; end < len(f) && f[end] >= '0' && f[end] <= '9'; end++ {
				if n <= 1<<20 {
					n = n*10 + int(f[end]-'0')
				}
			}
			if end >= len(f) || f[end] != ']' {
				return nil, formatSyntax("the argument index at byte " + strconv.Itoa(j) + " has no ]")
			}
			v.arg, j = n, end+1
		}
		if j >= len(f) {
			return nil, formatSyntax("the verb at byte " + strconv.Itoa(i) + " has no letter")
		}
		c = f[j]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			r, _ := firstRune(f[j:])
			return nil, formatSyntax(strconv.QuoteRune(r) + " at byte " + strconv.Itoa(j) + " is no part of a verb")
		}
		if strings.IndexByte(formatVerbs, c) < 0 {
			return nil, formatSyntax("%" + string(c) + " at byte " + strconv.Itoa(j) + " is no verb Format has")
		}
		v.letter, v.text = c, f[i:j+1]
		if literal.Len() > 0 {
			pieces = append(pieces, formatPiece{literal: literal.String()})
			literal.Reset()
		}
		pieces = append(pieces, formatPiece{verb: v})
		next = v.arg + 1
		i = j + 1
	}
	if literal.Len() > 0 {
		pieces = append(pieces, formatPiece{literal: literal.String()})
	}
	return pieces, tenon.Value{}
}

// firstRune returns the first scalar value of s.
func firstRune(s string) (rune, bool) {
	for _, r := range s {
		return r, true
	}
	return 0, false
}

// arguments checks that the verbs' arguments are those given, n of them:
// every verb's is among them, and every one is read, or one after it is.
func arguments(pieces []formatPiece, n int) tenon.Value {
	highest := 0
	for _, p := range pieces {
		if p.verb == nil {
			continue
		}
		if p.verb.arg > n {
			return invalid(0, "Format: "+p.verb.text+" at byte "+strconv.Itoa(p.verb.offset)+" reads argument "+
				strconv.Itoa(p.verb.arg)+", and "+strconv.Itoa(n)+" are given")
		}
		highest = max(highest, p.verb.arg)
	}
	if highest < n {
		if highest == 0 {
			return invalid(highest+1, "Format: the format has no verb, and an argument is given")
		}
		return invalid(highest+1, "Format: argument "+strconv.Itoa(highest+1)+" is read by no verb, the last read being "+strconv.Itoa(highest))
	}
	return tenon.Value{}
}

// padded returns s padded to the verb's width, counted in grapheme
// clusters: with spaces on the left, with zeros on the left where the 0
// flag is given, and with spaces on the right where the - flag is, which
// overrides 0.
func padded(v *verb, s string) string {
	if !v.hasWidth {
		return s
	}
	n := v.width - uni.GraphemeCount(s)
	switch {
	case n <= 0:
		return s
	case v.minus:
		return s + strings.Repeat(" ", n)
	case v.zero:
		return strings.Repeat("0", n) + s
	}
	return strings.Repeat(" ", n) + s
}

// clustersOf returns the first n extended grapheme clusters of s.
func clustersOf(s string, n int) string {
	end := 0
	for c := range uni.Clusters(s) {
		if n == 0 {
			break
		}
		end += len(c)
		n--
	}
	return s[:end]
}

// formatCheck returns the failure that formatting the argument a by the
// verb v gives whatever a is not known of yet: a null for a verb but %v,
// and a value its verb cannot convert, under the policy p.
func formatCheck(v *verb, a tenon.Value, p tenon.Policy) (tenon.Value, bool) {
	if a.IsNull() {
		if v.letter == 'v' {
			return tenon.Value{}, true
		}
		return tenon.ErrorVal(tenon.Diagnostic{
			Code:    tenon.CodeOperationNullOperand,
			Message: "Format: " + v.text + " at byte " + strconv.Itoa(v.offset) + " has no text for a null",
			Path:    argument(v.arg),
		}), false
	}
	var target tenon.Type
	switch v.letter {
	case 't':
		target = tenon.BoolType()
	case 's', 'q':
		target = tenon.StringType()
	case 'v':
		return tenon.Value{}, true
	default:
		target = tenon.NumberType()
	}
	c := tenon.Convert(a, tenon.Exactly(target), p)
	if c.IsError() {
		return at(v.arg, c), false
	}
	if c.IsKnown() && strings.IndexByte("bdoxX", v.letter) >= 0 && fractional(c) {
		return invalid(v.arg, "Format: "+v.text+" at byte "+strconv.Itoa(v.offset)+" writes an integer, and "+c.String()+" is not one"), false
	}
	return tenon.Value{}, true
}

// formatVerb returns the text of the known argument a by the verb v, under
// the policy p, which formatCheck has passed.
func formatVerb(v *verb, a tenon.Value, p tenon.Policy) string {
	if a.IsNull() {
		return padded(v, "null")
	}
	switch v.letter {
	case 'v':
		switch {
		case v.sharp:
			return padded(v, jsonText(a))
		case a.Type().Kind() == tenon.KindString:
			return formatString(v, a.AsString())
		case a.Type().Kind() == tenon.KindNumber:
			// The canonical text (NU-020), as a string converts to.
			s := tenon.Convert(a, text, tenon.Unsafe).AsString()
			return formatSigned(v, less(a, zero), strings.TrimPrefix(s, "-"))
		case a.Type().Kind() == tenon.KindBool:
			return padded(v, strconv.FormatBool(a.AsBool()))
		}
		return padded(v, jsonText(a))
	case 't':
		return padded(v, strconv.FormatBool(tenon.Convert(a, tenon.Exactly(tenon.BoolType()), p).AsBool()))
	case 's':
		return formatString(v, tenon.Convert(a, text, p).AsString())
	case 'q':
		s := tenon.Convert(a, text, p).AsString()
		if v.hasPrecision {
			s = clustersOf(s, v.precision)
		}
		var b strings.Builder
		writeJSONString(&b, s)
		return padded(v, b.String())
	}
	return formatNumber(v, tenon.Convert(a, number, p))
}

// formatString returns the string s by %s: its first clusters where a
// precision is given, as many as it says, none for a precision of zero,
// then padded.
func formatString(v *verb, s string) string {
	if v.hasPrecision {
		s = clustersOf(s, v.precision)
	}
	return padded(v, s)
}

// formatSigned returns the text of a number by the verb v, where negative
// says it is negative and digits are the rest: a minus sign where it is
// negative, a plus sign or a space where it is not and the + or the space
// flag is given, then the digits, padded to the width with spaces on the
// left, with zeros between the sign and the digits where the 0 flag is
// given, or with spaces on the right where the - flag is, which overrides
// 0.
func formatSigned(v *verb, negative bool, digits string) string {
	return formatSignedPrefixed(v, negative, "", digits)
}

// formatSignedPrefixed is formatSigned with a prefix between the sign and
// the digits, which zeros pad after.
func formatSignedPrefixed(v *verb, negative bool, prefix, digits string) string {
	sign := ""
	switch {
	case negative:
		sign = "-"
	case v.plus:
		sign = "+"
	case v.space:
		sign = " "
	}
	if v.zero && !v.minus && v.hasWidth {
		if n := v.width - len(sign) - len(prefix) - len(digits); n > 0 {
			return sign + prefix + strings.Repeat("0", n) + digits
		}
	}
	return padded(&verb{hasWidth: v.hasWidth, width: v.width, minus: v.minus}, sign+prefix+digits)
}

// formatNumber returns the text of the known number n by an integer or a
// decimal verb.
func formatNumber(v *verb, n tenon.Value) string {
	if strings.IndexByte("bdoxX", v.letter) >= 0 {
		return formatInteger(v, n)
	}
	internalPanic("Format: %s has no writer yet", v.text)
	return ""
}

// formatInteger returns the text of the known integer n by %d, %b, %o, %x
// or %X: its digits in base 10, 2, 8 or 16, in capitals for %X, at least as
// many as a precision says, none for a precision of zero and n zero; after
// the sign, the # flag's prefix, 0b, 0, 0x or 0X, the octal one only where
// the digits do not begin with 0; padded as formatSigned pads, but with no
// zeros where a precision is given.
func formatInteger(v *verb, n tenon.Value) string {
	i, _ := numbers.Dec(n).BigInt()
	base := map[byte]int{'d': 10, 'b': 2, 'o': 8, 'x': 16, 'X': 16}[v.letter]
	digits := new(big.Int).Abs(i).Text(base)
	if v.letter == 'X' {
		digits = strings.ToUpper(digits)
	}
	if v.hasPrecision {
		if i.Sign() == 0 && v.precision == 0 {
			digits = ""
		} else if pad := v.precision - len(digits); pad > 0 {
			digits = strings.Repeat("0", pad) + digits
		}
	}
	prefix := ""
	if v.sharp {
		switch v.letter {
		case 'b':
			prefix = "0b"
		case 'o':
			if !strings.HasPrefix(digits, "0") {
				prefix = "0"
			}
		case 'x':
			prefix = "0x"
		case 'X':
			prefix = "0X"
		}
	}
	w := *v
	w.zero = v.zero && !v.hasPrecision
	return formatSignedPrefixed(&w, i.Sign() < 0, prefix, digits)
}

// formatted renders the pieces with the arguments, the format at index 0,
// up to the first verb whose argument is not known yet, and reports
// whether it reached the end.
func formatted(pieces []formatPiece, args []tenon.Value, p tenon.Policy) (string, bool) {
	var b strings.Builder
	for _, piece := range pieces {
		if piece.verb == nil {
			b.WriteString(piece.literal)
			continue
		}
		a := args[piece.verb.arg]
		if !a.IsNull() && !a.IsKnown() {
			return b.String(), false
		}
		b.WriteString(formatVerb(piece.verb, a, p))
	}
	return b.String(), true
}

// FormatFunc writes its arguments into a format string, by go-cty's verb
// language: literal text, %% for a percent sign, and verbs, each % then
// flags, width, precision, an argument index and a letter. Widths and
// precisions count grapheme clusters, and pass 10,000 only with
// tenon.CodeFunctionTooLarge. A string not a format fails with
// tenon.CodeFormatInvalidSyntax, a verb reading an argument not given, or an
// argument no verb reads, with tenon.CodeFunctionInvalidArgument. %v and
// %#v write a null, a language's untyped null among them, as null; another
// verb fails on it with tenon.CodeOperationNullOperand. What the arguments
// already settle fails now, whatever else is not known yet; otherwise, not
// known yet, the answer begins with the text up to the first verb whose
// argument is not known.
var FormatFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Format",
	Description: "Writes the given arguments into the format string, by its verbs.",
	Params:      []tenon.Param{stringParam("format", "The format string.")},
	VarParam:    &tenon.Param{Name: "args", Description: "The arguments its verbs read, in order.", Constraint: tenon.Any(), AllowNull: true, AllowUnknown: true, AllowPending: true},
	Result:      text,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
		format := args[0]
		if !format.IsKnown() {
			prefix, _, _ := strings.Cut(format.Range().StringPrefix(), "%")
			return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(prefix)), nil
		}
		pieces, failure := parseFormat(format.AsString())
		if failure.IsZero() {
			failure = arguments(pieces, len(args)-1)
		}
		if !failure.IsZero() {
			return failure, nil
		}
		for _, piece := range pieces {
			if piece.verb == nil {
				continue
			}
			if failure, ok := formatCheck(piece.verb, args[piece.verb.arg], p); !ok {
				return failure, nil
			}
		}
		s, complete := formatted(pieces, args, p)
		if complete {
			return tenon.String(s), nil
		}
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(s)), nil
	},
})
