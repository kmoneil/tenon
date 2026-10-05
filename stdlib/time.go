package stdlib

import (
	"strconv"
	"strings"
	"time"

	"github.com/kmoneil/tenon"
)

// timestamp is a date-time of RFC 3339 read by the profile of LT-001 to
// LT-003: its fields as written, its fraction of a second exact, and its
// offset from UTC in minutes.
type timestamp struct {
	year, month, day     int
	hour, minute, second int
	// fraction is the digits after the point, as written, none where there
	// is no point.
	fraction string
	offset   int
}

// weekday returns the day of the week of the timestamp's date, by the
// proleptic Gregorian calendar.
func (t timestamp) weekday() time.Weekday {
	return time.Date(t.year, time.Month(t.month), t.day, 0, 0, 0, 0, time.UTC).Weekday()
}

// daysIn returns how many days the month has in the year, by the
// proleptic Gregorian calendar.
func daysIn(month, year int) int {
	switch month {
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// parseTimestamp reads s as RFC 3339's date-time (LT-001 to LT-003), or
// returns why it is not one.
func parseTimestamp(s string) (timestamp, string) {
	var t timestamp
	digits := func(at, n int, what string) (int, string) {
		v := 0
		for k := at; k < at+n; k++ {
			switch {
			case k >= len(s):
				return 0, "it ends at byte " + strconv.Itoa(k) + ", within its " + what
			case s[k] < '0' || s[k] > '9':
				return 0, "its " + what + " is not " + strconv.Itoa(n) + " digits: byte " + strconv.Itoa(k) + " is " + strconv.QuoteRune(rune(s[k]))
			}
			v = v*10 + int(s[k]-'0')
		}
		return v, ""
	}
	sep := func(at int, want string, what string) string {
		if len(s) <= at {
			return "it ends at byte " + strconv.Itoa(at) + ", before " + what
		}
		if !strings.ContainsRune(want, rune(s[at])) {
			return what + " was expected at byte " + strconv.Itoa(at) + ", and " + strconv.QuoteRune(rune(s[at])) + " found"
		}
		return ""
	}
	var why string
	for _, step := range []func() string{
		func() string { t.year, why = digits(0, 4, "year"); return why },
		func() string { return sep(4, "-", "'-'") },
		func() string { t.month, why = digits(5, 2, "month"); return why },
		func() string { return sep(7, "-", "'-'") },
		func() string { t.day, why = digits(8, 2, "day"); return why },
		func() string { return sep(10, "Tt", "'T'") },
		func() string { t.hour, why = digits(11, 2, "hour"); return why },
		func() string { return sep(13, ":", "':'") },
		func() string { t.minute, why = digits(14, 2, "minute"); return why },
		func() string { return sep(16, ":", "':'") },
		func() string { t.second, why = digits(17, 2, "second"); return why },
	} {
		if why := step(); why != "" {
			return timestamp{}, why
		}
	}
	rest := s[19:]
	if strings.HasPrefix(rest, ".") {
		n := 1
		for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
			n++
		}
		if n == 1 {
			return timestamp{}, "its fraction of a second has no digit, at byte 20"
		}
		t.fraction, rest = rest[1:n], rest[n:]
	}
	at := len(s) - len(rest)
	switch {
	case rest == "Z" || rest == "z":
	case rest == "":
		return timestamp{}, "it ends at byte " + strconv.Itoa(at) + ", before its offset from UTC, Z or +hh:mm or -hh:mm"
	case rest[0] == '+' || rest[0] == '-':
		if len(rest) != 6 || rest[3] != ':' {
			return timestamp{}, "its offset from UTC at byte " + strconv.Itoa(at) + " is not +hh:mm or -hh:mm"
		}
		h, hwhy := digitsOf(rest[1:3])
		m, mwhy := digitsOf(rest[4:6])
		if !hwhy || !mwhy {
			return timestamp{}, "its offset from UTC at byte " + strconv.Itoa(at) + " is not +hh:mm or -hh:mm"
		}
		if h > 23 || m > 59 {
			return timestamp{}, "its offset from UTC, " + rest + ", is not within 00:00 to 23:59"
		}
		t.offset = h*60 + m
		if rest[0] == '-' {
			t.offset = -t.offset
		}
	default:
		return timestamp{}, "Z or an offset from UTC was expected at byte " + strconv.Itoa(at) + ", and " + strconv.QuoteRune(rune(rest[0])) + " found"
	}
	switch {
	case t.month < 1 || t.month > 12:
		return timestamp{}, "its month, " + s[5:7] + ", is not 01 to 12"
	case t.day < 1 || t.day > daysIn(t.month, t.year):
		return timestamp{}, "its day, " + s[8:10] + ", is not in " + time.Month(t.month).String() + " " + s[0:4]
	case t.hour > 23:
		return timestamp{}, "its hour, " + s[11:13] + ", is not 00 to 23"
	case t.minute > 59:
		return timestamp{}, "its minute, " + s[14:16] + ", is not 00 to 59"
	case t.second == 60:
		return timestamp{}, "its second is 60, a leap second, which no timestamp here can hold"
	case t.second > 59:
		return timestamp{}, "its second, " + s[17:19] + ", is not 00 to 59"
	}
	return t, ""
}

// digitsOf reads the decimal digits s, and reports whether it is all
// digits.
func digitsOf(s string) (int, bool) {
	v := 0
	for k := range len(s) {
		if s[k] < '0' || s[k] > '9' {
			return 0, false
		}
		v = v*10 + int(s[k]-'0')
	}
	return v, true
}

// timeSyntax returns the failure of argument i of fn, text of the time
// language that is not one.
func timeSyntax(fn string, i int, message string) tenon.Value {
	return tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeTimeInvalidSyntax, Message: fn + ": " + message, Path: argument(i)})
}

// readTimestamp reads the timestamp, argument i of fn, or returns its
// failure.
func readTimestamp(fn string, i int, s string) (timestamp, tenon.Value) {
	t, why := parseTimestamp(s)
	if why != "" {
		return timestamp{}, timeSyntax(fn, i, strconv.Quote(s)+" is not an RFC 3339 timestamp: "+why)
	}
	return t, tenon.Value{}
}

// dateToken is a piece of a FormatDate format: literal text, or a
// directive, a run of one ASCII letter.
type dateToken struct {
	literal   string
	directive string
}

// dateDirectives are the directive letters and the lengths of each that
// the format language has (LT-005).
var dateDirectives = map[byte][]int{
	'Y': {2, 4}, 'M': {1, 2, 3, 4}, 'D': {1, 2}, 'E': {3, 4},
	'h': {1, 2}, 'H': {1, 2}, 'A': {2}, 'a': {2},
	'm': {1, 2}, 's': {1, 2}, 'Z': {1, 3, 4, 5},
}

// isLetter reports whether b is an ASCII letter.
func isLetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }

// parseDateFormat reads a FormatDate format into its tokens (LT-004),
// whole, or returns why it is not one: a run of one ASCII letter is a
// directive, and must be one the language has; text between quotation
// marks, ” within it a quotation mark, is literal, and so is ” outside
// it; and any other text is literal.
func parseDateFormat(f string) ([]dateToken, string) {
	var out []dateToken
	for i := 0; i < len(f); {
		switch c := f[i]; {
		case c == '\'':
			if strings.HasPrefix(f[i:], "''") {
				out = append(out, dateToken{literal: "'"})
				i += 2
				continue
			}
			var b strings.Builder
			j := i + 1
			for {
				k := strings.IndexByte(f[j:], '\'')
				if k < 0 {
					return nil, "the quotation mark at byte " + strconv.Itoa(i) + " begins literal text that no quotation mark closes"
				}
				b.WriteString(f[j : j+k])
				j += k + 1
				if strings.HasPrefix(f[j:], "'") {
					b.WriteByte('\'')
					j++
					continue
				}
				break
			}
			out = append(out, dateToken{literal: b.String()})
			i = j
		case isLetter(c):
			j := i + 1
			for j < len(f) && f[j] == c {
				j++
			}
			run := f[i:j]
			lengths, ok := dateDirectives[c]
			if !ok {
				return nil, "the letter " + strconv.QuoteRune(rune(c)) + " at byte " + strconv.Itoa(i) + " is no directive; quote it, as '" + string(c) + "', to write it"
			}
			if !slicesContains(lengths, len(run)) {
				return nil, "the directive " + strconv.Quote(run) + " at byte " + strconv.Itoa(i) + " has no meaning: " + string(c) + " is written " + directiveForms(c, lengths)
			}
			out = append(out, dateToken{directive: run})
			i = j
		default:
			j := i + 1
			for j < len(f) && f[j] != '\'' && !isLetter(f[j]) {
				j++
			}
			out = append(out, dateToken{literal: f[i:j]})
			i = j
		}
	}
	return out, ""
}

// slicesContains reports whether ns holds n.
func slicesContains(ns []int, n int) bool {
	for _, m := range ns {
		if m == n {
			return true
		}
	}
	return false
}

// directiveForms names the forms of the directive letter c, as in "Y or
// YYYY".
func directiveForms(c byte, lengths []int) string {
	var forms []string
	for _, n := range lengths {
		forms = append(forms, strings.Repeat(string(c), n))
	}
	if len(forms) == 1 {
		return forms[0]
	}
	return strings.Join(forms[:len(forms)-1], ", ") + " or " + forms[len(forms)-1]
}

// formatDirective writes the directive d of the timestamp t (LT-005).
func formatDirective(d string, t timestamp) string {
	two := func(n int) string {
		if n < 10 {
			return "0" + strconv.Itoa(n)
		}
		return strconv.Itoa(n)
	}
	offset := func(colon bool) string {
		sign, o := "+", t.offset
		if o < 0 {
			sign, o = "-", -o
		}
		if colon {
			return sign + two(o/60) + ":" + two(o%60)
		}
		return sign + two(o/60) + two(o%60)
	}
	hour12 := t.hour % 12
	if hour12 == 0 {
		hour12 = 12
	}
	switch d {
	case "YY":
		return two(t.year % 100)
	case "YYYY":
		return strings.Repeat("0", 4-len(strconv.Itoa(t.year))) + strconv.Itoa(t.year)
	case "M":
		return strconv.Itoa(t.month)
	case "MM":
		return two(t.month)
	case "MMM":
		return time.Month(t.month).String()[:3]
	case "MMMM":
		return time.Month(t.month).String()
	case "D":
		return strconv.Itoa(t.day)
	case "DD":
		return two(t.day)
	case "EEE":
		return t.weekday().String()[:3]
	case "EEEE":
		return t.weekday().String()
	case "h":
		return strconv.Itoa(t.hour)
	case "hh":
		return two(t.hour)
	case "H":
		return strconv.Itoa(hour12)
	case "HH":
		return two(hour12)
	case "AA", "aa":
		m := "AM"
		if t.hour >= 12 {
			m = "PM"
		}
		if d == "aa" {
			m = strings.ToLower(m)
		}
		return m
	case "m":
		return strconv.Itoa(t.minute)
	case "mm":
		return two(t.minute)
	case "s":
		return strconv.Itoa(t.second)
	case "ss":
		return two(t.second)
	case "Z":
		if t.offset == 0 {
			return "Z"
		}
		return offset(true)
	case "ZZZ":
		if t.offset == 0 {
			return "UTC"
		}
		return offset(false)
	case "ZZZZ":
		return offset(false)
	case "ZZZZZ":
		return offset(true)
	}
	return ""
}

// FormatDateFunc writes a timestamp by a format, go-cty's: a run of one
// ASCII letter is a directive, YY or YYYY the year, M to MMMM the month,
// D or DD the day, EEE or EEEE the day of the week, h or hh the hour of
// 24 and H or HH of 12, AA or aa its half of the day, m or mm the minute, s
// or ss the second, and Z, ZZZ, ZZZZ or ZZZZZ the offset; text between
// quotation marks is literal, ” a quotation mark, and so is any other
// text. The timestamp is RFC 3339's date-time, its T and Z in either case,
// where go-cty's takes capitals alone; the leap second is refused, as
// go-cty's refuses it. A format or a timestamp that is none fails with
// tenon.CodeTimeInvalidSyntax, located at it, whatever the other is; a
// format of any length is read whole, where go-cty's drops text past 64 KiB
// without a word, and literal text a quotation mark does not close fails,
// where go-cty's may take its doubled one as closing. Where the timestamp
// is not known yet, the answer begins with the format's text before its
// first directive.
var FormatDateFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "FormatDate",
	Description: "Formats a timestamp given in RFC 3339 syntax into another timestamp in some other machine-oriented time syntax, as described in the format string.",
	Params: []tenon.Param{
		stringParam("format", "The format of the timestamp to write."),
		stringParam("time", "The RFC 3339 timestamp."),
	},
	Result:  text,
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		format, stamp := args[0], args[1]
		var tokens []dateToken
		if format.IsKnown() {
			var why string
			if tokens, why = parseDateFormat(format.AsString()); why != "" {
				return timeSyntax("FormatDate", 0, why), nil
			}
		}
		var t timestamp
		if stamp.IsKnown() {
			var failure tenon.Value
			if t, failure = readTimestamp("FormatDate", 1, stamp.AsString()); !failure.IsZero() {
				return failure, nil
			}
		}
		var b strings.Builder
		switch {
		case format.IsKnown():
			for _, tok := range tokens {
				if tok.directive == "" {
					b.WriteString(tok.literal)
					continue
				}
				if !stamp.IsKnown() {
					return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(b.String())), nil
				}
				b.WriteString(formatDirective(tok.directive, t))
			}
			return tenon.String(b.String()), nil
		}
		// The format's recorded prefix: its literal text before a letter
		// or a quotation mark, which may begin anything.
		p := format.Range().StringPrefix()
		if i := strings.IndexFunc(p, func(r rune) bool { return r == '\'' || r < 0x80 && isLetter(byte(r)) }); i >= 0 {
			p = p[:i]
		}
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(p)), nil
	},
})
