package stdlib

import (
	"math/big"
	"slices"
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
	small, c, exp := numbers.Dec(n).Parts()
	if c == nil {
		c = big.NewInt(small)
	}
	negative := c.Sign() < 0
	c = new(big.Int).Abs(c)
	var text string
	switch v.letter {
	case 'e', 'E':
		text = formatE(c, exp, precisionOr(v, 6), v.letter)
	case 'f':
		text = formatF(c, exp, precisionOr(v, 6))
	default:
		text = formatG(v, c, exp)
	}
	return formatSigned(v, negative, text)
}

// precisionOr returns the verb's precision, or def where it gives none.
func precisionOr(v *verb, def int) int {
	if v.hasPrecision {
		return v.precision
	}
	return def
}

// ten is the number ten, which the decimal verbs scale by.
var ten = big.NewInt(10)

// roundScaled returns c × 10^shift rounded half to even to a whole number,
// c not negative: the decimal verbs round the exact value so, as NU-012
// rounds a quotient.
func roundScaled(c *big.Int, shift int64) *big.Int {
	if shift >= 0 {
		return new(big.Int).Mul(c, new(big.Int).Exp(ten, big.NewInt(shift), nil))
	}
	d := new(big.Int).Exp(ten, big.NewInt(-shift), nil)
	q, r := new(big.Int).QuoRem(c, d, new(big.Int))
	switch r.Lsh(r, 1).Cmp(d) {
	case 1:
		q.Add(q, big.NewInt(1))
	case 0:
		if q.Bit(0) == 1 {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}

// digitCount returns how many decimal digits c, not zero, has.
func digitCount(c *big.Int) int64 { return int64(len(c.String())) }

// exponentText returns the exponent x as %e writes it: e or E, its sign,
// and at least two digits.
func exponentText(x int64, letter byte) string {
	sign := "+"
	if x < 0 {
		sign, x = "-", -x
	}
	digits := strconv.FormatInt(x, 10)
	if len(digits) < 2 {
		digits = "0" + digits
	}
	if letter == 'E' || letter == 'G' {
		return "E" + sign + digits
	}
	return "e" + sign + digits
}

// formatE returns the magnitude c × 10^exp by %e with p digits after the
// point: one digit before it, the value rounded half to even, then the
// exponent.
func formatE(c *big.Int, exp int64, p int, letter byte) string {
	var digits string
	x := int64(0)
	if c.Sign() == 0 {
		digits = strings.Repeat("0", p+1)
	} else {
		x = digitCount(c) + exp - 1
		m := roundScaled(c, exp-x+int64(p))
		if digitCount(m) > int64(p)+1 {
			x++
			m.Quo(m, ten)
		}
		digits = m.String()
	}
	return mantissa(digits, p) + exponentText(x, letter)
}

// mantissa returns digits with a point after the first, where p digits
// follow it.
func mantissa(digits string, p int) string {
	if p == 0 {
		return digits[:1]
	}
	return digits[:1] + "." + digits[1:]
}

// formatF returns the magnitude c × 10^exp by %f with p digits after the
// point, the value rounded half to even.
func formatF(c *big.Int, exp int64, p int) string {
	var s string
	if c.Sign() == 0 || digitCount(c)+exp+int64(p) < 0 {
		// Less than a tenth of the last place: it rounds to zero.
		s = "0"
	} else {
		s = roundScaled(c, exp+int64(p)).String()
	}
	if len(s) <= p {
		s = strings.Repeat("0", p-len(s)+1) + s
	}
	if p == 0 {
		return s
	}
	return s[:len(s)-p] + "." + s[len(s)-p:]
}

// formatG returns the magnitude c × 10^exp by %g or %G: with a precision,
// rounded half to even to that many significant digits, one for zero, and
// without one, all its digits; then as %e where the exponent is less than
// -4 or not less than the precision, six where none is given, and as %f
// otherwise, trailing zeros not written. This is Go's rule, which go-cty's
// big.Float follows.
func formatG(v *verb, c *big.Int, exp int64) string {
	if c.Sign() == 0 {
		return "0"
	}
	digits := c.String()
	dp := int64(len(digits)) + exp
	prec := len(digits)
	if v.hasPrecision {
		prec = max(v.precision, 1)
		if len(digits) > prec {
			m := roundScaled(c, int64(prec-len(digits)))
			dp += digitCount(m) - int64(prec)
			digits = strings.TrimRight(m.String(), "0")
		}
	}
	nd := int64(len(digits))
	eprec := int64(prec)
	if eprec > nd && nd >= dp {
		eprec = nd
	}
	if !v.hasPrecision {
		eprec = 6
	}
	if x := dp - 1; x < -4 || x >= eprec {
		if int64(prec) > nd {
			prec = int(nd)
		}
		return mantissa(digits, prec-1) + exponentText(x, v.letter)
	}
	if int64(prec) > dp {
		prec = int(nd)
	}
	coefficient, _ := new(big.Int).SetString(digits, 10)
	return formatF(coefficient, dp-nd, int(max(int64(prec)-dp, 0)))
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
// whether it reached the end; or it fails where the text would pass limit
// bytes (LF-022). No piece is written past the limit, and no verb's text is
// made where the digits its number needs already pass it.
func formatted(pieces []formatPiece, args []tenon.Value, p tenon.Policy) (string, bool, tenon.Value) {
	// The limit is at least 64 KiB, so the arguments are measured only
	// once the text could pass that.
	limit := -1
	fits := func(n int64) bool {
		if n <= 64<<10 {
			return true
		}
		if limit < 0 {
			limit = formatLimit(args)
		}
		return n <= int64(limit)
	}
	tooLarge := func() tenon.Value {
		if limit < 0 {
			limit = formatLimit(args)
		}
		return formatTooLarge(limit)
	}
	var b strings.Builder
	for _, piece := range pieces {
		if piece.verb == nil {
			if !fits(int64(b.Len() + len(piece.literal))) {
				return "", false, tooLarge()
			}
			b.WriteString(piece.literal)
			continue
		}
		a := args[piece.verb.arg]
		if !a.IsNull() && !a.IsKnown() {
			return b.String(), false, tenon.Value{}
		}
		if !fits(int64(b.Len()) + verbLeast(piece.verb, a, p)) {
			return "", false, tooLarge()
		}
		text := formatVerb(piece.verb, a, p)
		if !fits(int64(b.Len() + len(text))) {
			return "", false, tooLarge()
		}
		b.WriteString(text)
	}
	return b.String(), true, tenon.Value{}
}

// formatLimit returns the most bytes Format's answer of these arguments,
// the format at index 0, may hold (LF-022): 64 times their size, and 64
// KiB.
func formatLimit(args []tenon.Value) int {
	size := 0
	for _, a := range args {
		size += argumentSize(a)
	}
	return 64*size + 64<<10
}

// formatTooLarge returns Format's failure of an answer passing limit.
func formatTooLarge(limit int) tenon.Value {
	return tooLarge(0, "Format: the answer passes "+strconv.Itoa(limit)+
		" bytes, 64 times the arguments' size and 64 KiB, the most it makes")
}

// verbLeast returns, without making it, at least how many bytes the verb
// v writes of the known argument a under the policy p where the magnitude
// of a number decides it: the JSON text of %#v, and of %v of anything but
// a string, a number or a bool, exactly; the integer digits an integer
// verb writes in its base, and %f's with its precision; and nothing for
// any other verb, whose text the argument's size and the format's widths
// and precisions bound.
func verbLeast(v *verb, a tenon.Value, p tenon.Policy) int64 {
	if a.IsNull() {
		return 0
	}
	switch v.letter {
	case 'v':
		if !v.sharp {
			switch a.Type().Kind() {
			case tenon.KindString, tenon.KindNumber, tenon.KindBool:
				return 0
			}
		}
		return jsonLen(a, false)
	case 'b', 'd', 'o', 'x', 'X', 'f':
	default:
		return 0
	}
	d := integerDigits(tenon.Convert(a, number, p))
	switch v.letter {
	case 'b':
		// log2(10) is more than 3, log8(10) more than 1, log16(10) more
		// than 4/5.
		return 3*(d-1) + 1
	case 'o':
		return d
	case 'x', 'X':
		return 4*(d-1)/5 + 1
	case 'f':
		if q := int64(precisionOr(v, 6)); q > 0 {
			return d + 1 + q
		}
	}
	return d
}

// integerDigits returns how many decimal digits the integer part of the
// known number n has, one where it is zero.
func integerDigits(n tenon.Value) int64 {
	small, c, exp := numbers.Dec(n).Parts()
	var d int64
	switch {
	case c != nil:
		d = int64(len(c.String()))
		if c.Sign() < 0 {
			d--
		}
	case small == 0:
		return 1
	default:
		for u := small; u != 0; u /= 10 {
			d++
		}
	}
	return max(d+exp, 1)
}

// formatAnswer answers Format of the parsed pieces with the arguments, the
// format at index 0, which arguments has checked: the failure the
// arguments settle, the text, or, where an argument a verb reads is not
// known yet, the unknown string beginning with the text before its verb.
func formatAnswer(pieces []formatPiece, args []tenon.Value, p tenon.Policy) tenon.Value {
	for _, piece := range pieces {
		if piece.verb == nil {
			continue
		}
		if failure, ok := formatCheck(piece.verb, args[piece.verb.arg], p); !ok {
			return failure
		}
	}
	s, complete, failure := formatted(pieces, args, p)
	if !failure.IsZero() {
		return failure
	}
	if complete {
		return tenon.String(s)
	}
	return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(s))
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
		return formatAnswer(pieces, args, p), nil
	},
})

// iteration is how FormatList reads an argument: iterated, one of its
// members to each element, or repeated, the whole of it to each.
type iteration struct {
	iterated bool
	// length is how many members an iterated argument has, where known.
	length    int
	known     bool
	lo, hi    int64
	bounded   bool
	members   []tenon.Value
	undecided bool
}

// iterationOf reads the argument a of FormatList: a list, a set or a tuple
// not null is iterated, a pending value that can be nothing else too, and
// any other value repeated; a pending value that may be either is
// undecided.
func iterationOf(a tenon.Value) iteration {
	if a.IsNull() {
		return iteration{}
	}
	kinds := []tenon.Kind{tenon.KindList, tenon.KindSet, tenon.KindTuple}
	if a.IsPending() && !a.HasMembers() {
		ks := kindsOf(a.Constraint())
		switch {
		case !slices.ContainsFunc(ks, func(k tenon.Kind) bool { return slices.Contains(kinds, k) }):
			return iteration{}
		case slices.ContainsFunc(ks, func(k tenon.Kind) bool { return !slices.Contains(kinds, k) }):
			return iteration{undecided: true}
		}
		return iteration{iterated: true}
	}
	if k, ok := kindOf(a); ok && !slices.Contains(kinds, k) {
		return iteration{}
	}
	it := iteration{iterated: true}
	lo, hi, bounded := lengthOf(a)
	it.lo, it.hi, it.bounded = lo, hi, bounded
	switch k, _ := kindOf(a); {
	case a.HasMembers() && (k != tenon.KindSet || bounded && lo == hi):
		it.members = a.Elements()
		it.length, it.known = len(it.members), true
	case k == tenon.KindTuple:
		for _, t := range a.Type().TupleElementTypes() {
			it.members = append(it.members, tenon.Unknown(t))
		}
		it.length, it.known = len(it.members), true
	case bounded && lo == hi:
		// A list or a set not known yet of one length: so many members,
		// each not known yet.
		for range lo {
			it.members = append(it.members, tenon.Unknown(a.Type().ElementType()))
		}
		it.length, it.known = int(lo), true
	}
	return it
}

// memberAt returns v with each diagnostic located at argument j moved to
// its member i.
func memberAt(v tenon.Value, j, i int) tenon.Value {
	ds := v.Diagnostics()
	for k, d := range ds {
		steps := d.Path.Steps()
		if len(steps) == 0 || !steps[0].Equal(argument(j).Steps()[0]) {
			continue
		}
		path := argument(j).Index(tenon.NumberFromInt(int64(i)))
		for _, step := range steps[1:] {
			if step.Kind() == tenon.StepAttribute {
				path = path.Attribute(step.Name())
			} else {
				path = path.Index(step.Key())
			}
		}
		ds[k].Path = path
	}
	return tenon.ErrorVal(ds...)
}

// argumentSize returns how many bytes the argument a stands for, for
// Format's and FormatList's bounds (LF-021, LF-022): a string's, and for
// any other value known the JSON text's, each number in its canonical text,
// so that a number's size is what it says, not the digits it may be
// written out to. A value not known yet stands for none.
func argumentSize(a tenon.Value) int {
	switch {
	case !a.IsNull() && !a.IsKnown():
		return 0
	case !a.IsNull() && a.Type().Kind() == tenon.KindString:
		return len(a.AsString())
	}
	return int(jsonLen(a, true))
}

// FormatListFunc writes its arguments into a format string as Format does,
// once for each member of the lists, sets and tuples among them, the other
// arguments repeated each time, and answers the list of the strings made.
// The iterated arguments have one length, an argument of another failing
// with tenon.CodeFunctionInvalidArgument at it; with none, the format is
// written once. The format and the arguments its verbs read are checked
// however many times it is written, and a failure in one element is
// located at the argument and the member it read. A set's members are
// read in the canonical order (EQ-044). An answer of more than 64 times the
// size of the arguments, and 64 KiB, fails with tenon.CodeFunctionTooLarge
// at the format. Not known yet, the answer is the unknown list of the
// length the arguments settle.
var FormatListFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "FormatList",
	Description: "Writes the arguments into the format string once for each member of the lists among them.",
	Params:      []tenon.Param{stringParam("format", "The format string.")},
	VarParam:    &tenon.Param{Name: "args", Description: "The arguments its verbs read; lists, sets and tuples iterated, the others repeated.", Constraint: tenon.Any(), AllowNull: true, AllowUnknown: true, AllowPending: true},
	Result:      tenon.Exactly(tenon.ListType(tenon.StringType())),
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, p tenon.Policy) (tenon.Value, error) {
		format := args[0]
		var pieces []formatPiece
		if format.IsKnown() {
			var failure tenon.Value
			if pieces, failure = parseFormat(format.AsString()); failure.IsZero() {
				failure = arguments(pieces, len(args)-1)
			}
			if !failure.IsZero() {
				return failure, nil
			}
		}
		its := make([]iteration, len(args))
		n, first, decided := -1, 0, format.IsKnown()
		lo, hi, bounded := int64(0), int64(0), false
		for j := 1; j < len(args); j++ {
			it := iterationOf(args[j])
			its[j] = it
			switch {
			case it.undecided:
				decided = false
			case !it.iterated:
			case it.known && n < 0:
				n, first = it.length, j
			case it.known && it.length != n:
				return invalid(j, "FormatList: argument "+strconv.Itoa(j)+" has "+strconv.Itoa(it.length)+
					" members, and argument "+strconv.Itoa(first)+" has "+strconv.Itoa(n)), nil
			case !it.known:
				decided = false
				lo = max(lo, it.lo)
				if it.bounded && (!bounded || it.hi < hi) {
					hi, bounded = it.hi, true
				}
			}
		}
		if n >= 0 {
			for j := 1; j < len(args); j++ {
				if it := its[j]; it.iterated && !it.known && (int64(n) < it.lo || it.bounded && int64(n) > it.hi) {
					return invalid(j, "FormatList: argument "+strconv.Itoa(j)+" has from "+strconv.FormatInt(it.lo, 10)+
						" members, and argument "+strconv.Itoa(first)+" has "+strconv.Itoa(n)), nil
				}
			}
		}
		if !decided {
			ns := []tenon.Narrowing{tenon.NotNull()}
			switch {
			case n >= 0:
				ns = append(ns, tenon.LengthMin(int64(n)), tenon.LengthMax(int64(n)))
			default:
				ns = append(ns, tenon.LengthMin(lo))
				if bounded {
					ns = append(ns, tenon.LengthMax(hi))
				}
			}
			return tenon.Narrow(tenon.Unknown(tenon.ListType(tenon.StringType())), ns...), nil
		}
		if n < 0 {
			n = 1
		}
		size := len(format.AsString())
		for _, a := range args[1:] {
			size += argumentSize(a)
		}
		limit, made := 64*size+64<<10, 0
		elems := make([]tenon.Value, n)
		each := make([]tenon.Value, len(args))
		for i := range n {
			each[0] = format
			for j := 1; j < len(args); j++ {
				if its[j].iterated {
					each[j] = its[j].members[i]
				} else {
					each[j] = args[j]
				}
			}
			e := formatAnswer(pieces, each, p)
			if e.IsError() {
				for j := 1; j < len(args); j++ {
					if its[j].iterated {
						e = memberAt(e, j, i)
					}
				}
				return e, nil
			}
			if e.IsKnown() {
				if made += len(e.AsString()); made > limit {
					return tooLarge(0, "FormatList: the answer passes "+strconv.Itoa(limit)+
						" bytes, 64 times the arguments' size and 64 KiB, the most it makes"), nil
				}
			}
			elems[i] = e
		}
		return tenon.List(tenon.StringType(), elems...), nil
	},
})
