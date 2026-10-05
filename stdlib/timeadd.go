package stdlib

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/kmoneil/tenon"
)

// durationUnit is a unit of a TimeAdd duration: its length in seconds, a
// multiple of a power of ten, mul times 10^-exp.
type durationUnit struct {
	mul int64
	exp int
}

// durationUnits are the units a duration may name (LT-008), Go's.
var durationUnits = map[string]durationUnit{
	"ns": {1, 9}, "us": {1, 6}, "\U000000B5s": {1, 6}, "\U000003BCs": {1, 6},
	"ms": {1, 3}, "s": {1, 0}, "m": {60, 0}, "h": {3600, 0},
}

// durationBound is the most bytes a duration's text may have (LT-008), as
// NU-024 bounds a number's.
const durationBound = 10000

// exact is a number of seconds, exactly: v times 10^-scale.
type exact struct {
	v     *big.Int
	scale int
}

// at returns e's value as an integer of 10^-scale seconds, scale not less
// than e's.
func (e exact) at(scale int) *big.Int {
	return new(big.Int).Mul(e.v, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale-e.scale)), nil))
}

// plus returns e + f.
func (e exact) plus(f exact) exact {
	s := max(e.scale, f.scale)
	return exact{new(big.Int).Add(e.at(s), f.at(s)), s}
}

// parseDuration reads s by the grammar of LT-008, Go's ParseDuration's, and
// returns its value exactly, or why it is no duration.
func parseDuration(s string) (exact, string) {
	total := exact{new(big.Int), 0}
	rest := s
	negative := false
	if rest != "" && (rest[0] == '-' || rest[0] == '+') {
		negative = rest[0] == '-'
		rest = rest[1:]
	}
	if rest == "0" {
		return total, ""
	}
	if rest == "" {
		return exact{}, "it has no number"
	}
	for rest != "" {
		at := len(s) - len(rest)
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		whole, frac := rest[:i], ""
		rest = rest[i:]
		if strings.HasPrefix(rest, ".") {
			j := 1
			for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
				j++
			}
			frac, rest = rest[1:j], rest[j:]
		}
		if whole == "" && frac == "" {
			return exact{}, "a number was expected at byte " + strconv.Itoa(at)
		}
		j := 0
		for j < len(rest) && rest[j] != '.' && (rest[j] < '0' || rest[j] > '9') {
			j++
		}
		if j == 0 {
			return exact{}, "the number at byte " + strconv.Itoa(at) + " has no unit: ns, us, ms, s, m or h"
		}
		name := rest[:j]
		rest = rest[j:]
		u, ok := durationUnits[name]
		if !ok {
			return exact{}, "its unit " + strconv.Quote(name) + " is none of ns, us, ms, s, m and h"
		}
		digits, _ := new(big.Int).SetString(strings.TrimLeft(whole+frac, "0")+"0", 10)
		digits.Div(digits, big.NewInt(10))
		digits.Mul(digits, big.NewInt(u.mul))
		total = total.plus(exact{digits, len(frac) + u.exp})
	}
	if negative {
		total.v.Neg(total.v)
	}
	return total, ""
}

// daysFromCivil returns the number of days from 1970-01-01 to the date,
// by the proleptic Gregorian calendar.
func daysFromCivil(y, m, d int64) int64 {
	if m <= 2 {
		y--
	}
	era := y / 400
	if y < 0 {
		era = (y - 399) / 400
	}
	yoe := y - era*400
	mp := m - 3
	if m <= 2 {
		mp = m + 9
	}
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// civilFromDays returns the date the number of days from 1970-01-01 is, by
// the proleptic Gregorian calendar.
func civilFromDays(z int64) (y, m, d int64) {
	z += 719468
	era := z / 146097
	if z < 0 {
		era = (z - 146096) / 146097
	}
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y = yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d = doy - (153*mp+2)/5 + 1
	m = mp + 3
	if mp >= 10 {
		m = mp - 9
	}
	if m <= 2 {
		y++
	}
	return y, m, d
}

// instant returns the timestamp's fields as seconds from 1970-01-01T00:00:00
// in its own offset, exactly.
func (t timestamp) instant() exact {
	days := daysFromCivil(int64(t.year), int64(t.month), int64(t.day))
	seconds := days*86400 + int64(t.hour)*3600 + int64(t.minute)*60 + int64(t.second)
	frac, _ := new(big.Int).SetString(t.fraction+"0", 10)
	frac.Div(frac, big.NewInt(10))
	whole := exact{big.NewInt(seconds), 0}
	return whole.plus(exact{frac, len(t.fraction)})
}

// rfc3339 writes the instant e, seconds from 1970-01-01T00:00:00 in the
// offset, as an RFC 3339 timestamp in that offset (LT-010), or reports
// that its year is outside 0000 to 9999.
func rfc3339(e exact, offset int) (string, bool) {
	unit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(e.scale)), nil)
	seconds, frac := new(big.Int).DivMod(e.v, unit, new(big.Int))
	lo, hi := big.NewInt(daysFromCivil(0, 1, 1)*86400), big.NewInt(daysFromCivil(10000, 1, 1)*86400)
	if seconds.Cmp(lo) < 0 || seconds.Cmp(hi) >= 0 {
		return "", false
	}
	s := seconds.Int64()
	days, rem := s/86400, s%86400
	if rem < 0 {
		days, rem = days-1, rem+86400
	}
	y, m, d := civilFromDays(days)
	two := func(n int64) string {
		if n < 10 {
			return "0" + strconv.FormatInt(n, 10)
		}
		return strconv.FormatInt(n, 10)
	}
	var b strings.Builder
	b.WriteString(strings.Repeat("0", 4-len(strconv.FormatInt(y, 10))) + strconv.FormatInt(y, 10))
	b.WriteString("-" + two(m) + "-" + two(d) + "T" + two(rem/3600) + ":" + two(rem/60%60) + ":" + two(rem%60))
	if frac.Sign() != 0 {
		digits := frac.String()
		b.WriteString("." + strings.TrimRight(strings.Repeat("0", e.scale-len(digits))+digits, "0"))
	}
	if offset == 0 {
		b.WriteString("Z")
	} else {
		sign, o := "+", int64(offset)
		if o < 0 {
			sign, o = "-", -o
		}
		b.WriteString(sign + two(o/60) + ":" + two(o%60))
	}
	return b.String(), true
}

// TimeAddFunc adds a duration to a timestamp, exactly: the timestamp is
// RFC 3339's, as FormatDateFunc reads it, and the duration Go's
// ParseDuration's language, an optional sign and numbers, each with a
// fraction where it has one, of the units ns, us (µs and μs), ms, s, m and
// h, or 0 alone. The sum of each number times its unit is added with no
// rounding anywhere, where go-cty's reads each number through a float64
// into int64 nanoseconds, so it has no 292-year limit; and the answer, in
// the timestamp's own offset, Z for none, keeps the fraction of a second,
// with no zero after its last other digit and no point where it is zero,
// so whole seconds give go-cty's bytes, and 500ms is no longer dropped. A
// timestamp or duration that is none fails with tenon.CodeTimeInvalidSyntax
// at it, now even beside an argument not known yet; a duration's text
// past 10,000 bytes with tenon.CodeFunctionTooLarge; and an answer outside
// the years 0000 to 9999, which go-cty's writes though no timestamp reads
// it, with tenon.CodeTimeOutOfRange at the duration.
var TimeAddFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "TimeAdd",
	Description: "Adds the duration represented by the given duration string to the given RFC 3339 timestamp string, returning another RFC 3339 timestamp.",
	Params: []tenon.Param{
		stringParam("timestamp", "The RFC 3339 timestamp."),
		stringParam("duration", "The duration to add, as 1h30m or -15.5s."),
	},
	Result:  text,
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		stamp, dur := args[0], args[1]
		var t timestamp
		if stamp.IsKnown() {
			var failure tenon.Value
			if t, failure = readTimestamp("TimeAdd", 0, stamp.AsString()); !failure.IsZero() {
				return failure, nil
			}
		}
		var d exact
		if dur.IsKnown() {
			s := dur.AsString()
			if len(s) > durationBound {
				return tooLarge(1, "TimeAdd: the duration's text passes "+strconv.Itoa(durationBound)+" bytes, the most it reads"), nil
			}
			var why string
			if d, why = parseDuration(s); why != "" {
				return timeSyntax("TimeAdd", 1, strconv.Quote(s)+" is not a duration: "+why), nil
			}
		}
		if !stamp.IsKnown() || !dur.IsKnown() {
			return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull()), nil
		}
		answer, ok := rfc3339(t.instant().plus(d), t.offset)
		if !ok {
			return tenon.ErrorVal(tenon.Diagnostic{
				Code:    tenon.CodeTimeOutOfRange,
				Message: "TimeAdd: " + strconv.Quote(dur.AsString()) + " moves " + strconv.Quote(stamp.AsString()) + " outside the years 0000 to 9999, which no timestamp holds",
				Path:    argument(1),
			}), nil
		}
		return tenon.String(answer), nil
	},
})
