package stdlib

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/uni"
)

// trimmed returns where the text that trims s keeps begins and ends: past
// the longest prefix of code points that in reports true of whose end is a
// cut position, and before the longest such suffix whose start is one.
func trimmed(s string, in func(rune) bool) (start, end int) {
	cuts := uni.CutsOf(s)
	all := true
	for i, r := range s {
		if cuts.At(i) {
			start = i
		}
		if !in(r) {
			all = false
			break
		}
	}
	if all {
		return len(s), len(s)
	}
	end = len(s)
	for j := len(s); j > start; {
		r, size := utf8.DecodeLastRuneInString(s[:j])
		if !in(r) {
			break
		}
		j -= size
		if cuts.At(j) {
			end = j
		}
	}
	return start, end
}

// trimmedPrefix returns what trimming any string beginning with p keeps
// begins with, and false where nothing is known of it: p past the prefix
// the trim settles within it, before the code points that in reports true
// of at its end, which the rest of the string may continue. Where every
// code point of p is one, the trim may reach past it.
func trimmedPrefix(p string, in func(rune) bool) (string, bool) {
	first := strings.IndexFunc(p, func(r rune) bool { return !in(r) })
	if first < 0 {
		return "", false
	}
	_, size := utf8.DecodeRuneInString(p[first:])
	start, _ := trimmed(p[:first+size], in)
	last := strings.LastIndexFunc(p, func(r rune) bool { return !in(r) })
	_, size = utf8.DecodeRuneInString(p[last:])
	return p[start : last+size], true
}

// trimAnswer answers a trim of str by the code points in reports true of,
// from the recorded prefix where str is not known yet.
func trimAnswer(str tenon.Value, in func(rune) bool) tenon.Value {
	if str.IsKnown() {
		s := str.AsString()
		start, end := trimmed(s, in)
		return tenon.String(s[start:end])
	}
	ns := []tenon.Narrowing{tenon.NotNull()}
	if p, ok := trimmedPrefix(str.Range().StringPrefix(), in); ok {
		ns = append(ns, tenon.StringPrefix(p))
	}
	return tenon.Narrow(tenon.Unknown(tenon.StringType()), ns...)
}

// TrimFunc removes from both ends of a string the longest runs of the code
// points the cutset holds, each a set of code points as go-cty's is, so
// "xyhixy" trimmed of "yx" is "hi"; a run ends, and begins, only at a cut
// position, so no cluster is split: trimming a thumb from a thumb with a
// skin tone leaves it whole. Not known yet, the answer begins with what the
// string's recorded prefix settles, where it holds a code point the cutset
// does not.
var TrimFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Trim",
	Description: "Removes the given code points from the start and the end of the given string.",
	Params:      []tenon.Param{stringParam("str", "The string."), stringParam("cutset", "The code points to remove.")},
	Result:      text,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		str, cutset := args[0], args[1]
		if !cutset.IsKnown() {
			return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull()), nil
		}
		set := cutset.AsString()
		return trimAnswer(str, func(r rune) bool { return strings.ContainsRune(set, r) }), nil
	},
})

// TrimSpaceFunc removes from both ends of a string the longest runs of
// White_Space code points, of the Unicode version tenon states, ending and
// beginning at cut positions: a space carrying a combining mark stays.
// Not known yet, the answer begins with what the string's recorded prefix
// settles.
var TrimSpaceFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "TrimSpace",
	Description: "Removes white space from the start and the end of the given string.",
	Params:      []tenon.Param{stringParam("str", "The string.")},
	Result:      text,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return trimAnswer(args[0], uni.IsWhiteSpace), nil
	},
})

// lineEnd reports whether r is a CR or an LF.
func lineEnd(r rune) bool { return r == '\r' || r == '\n' }

// ChompFunc removes every CR and LF at the end of a string, in any order,
// as go-cty's does: the other line terminators, NEL, the line and
// paragraph separators, stay. Not known yet, the answer begins with the
// string's recorded prefix less the CRs and LFs it ends with.
var ChompFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Chomp",
	Description: "Removes every carriage return and line feed at the end of the given string.",
	Params:      []tenon.Param{stringParam("str", "The string.")},
	Result:      text,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		str := args[0]
		if str.IsKnown() {
			return tenon.String(strings.TrimRightFunc(str.AsString(), lineEnd)), nil
		}
		p := strings.TrimRightFunc(str.Range().StringPrefix(), lineEnd)
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(p)), nil
	},
})

// indentBound returns the most bytes an Indent of a string of n bytes may
// make (LB-031): 64 times its size, and 64 KiB.
func indentBound(n int) int { return 64*n + 64<<10 }

// indented returns s with n spaces after each LF.
func indented(s string, n int) string {
	return strings.ReplaceAll(s, "\n", "\n"+strings.Repeat(" ", n))
}

// IndentFunc puts a number of spaces after every LF of a string, the last
// included, as go-cty's does; a CR alone, NEL and the line and paragraph
// separators break no line. The number is a whole number not less than
// zero, which fails with tenon.CodeFunctionInvalidArgument where it is not,
// where go-cty's panics on a negative one; and an answer of more than 64
// times the size of the string, and 64 KiB, fails with
// tenon.CodeFunctionTooLarge at it before any of it is made, where go-cty's
// may exhaust the host's memory. A string with no LF is the answer whatever
// the number. Not known yet, the answer begins with the string's recorded
// prefix indented, or up to its first LF where the number is not known.
var IndentFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Indent",
	Description: "Adds the given number of spaces after each line feed of the given string.",
	Params: []tenon.Param{
		{Name: "spaces", Description: "How many spaces to add.", Constraint: number, AllowUnknown: true},
		stringParam("str", "The string."),
	},
	Result:  text,
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		spaces, str := args[0], args[1]
		if spaces.IsKnown() {
			if failure, ok := wholeAtLeastZero("Indent", 0, "number of spaces", spaces); !ok {
				return failure, nil
			}
		}
		if str.IsKnown() && strings.IndexByte(str.AsString(), '\n') < 0 {
			return str, nil
		}
		if spaces.IsKnown() && str.IsKnown() {
			s := str.AsString()
			lines, limit := strings.Count(s, "\n"), indentBound(len(s))
			n := clamped(spaces, 0, int64(limit))
			if size := int64(len(s)) + n*int64(lines); size > int64(limit) {
				return tooLarge(0, "Indent: the answer would be more than "+strconv.Itoa(limit)+
					" bytes, 64 times the string's size and 64 KiB, the most it makes"), nil
			}
			return tenon.String(indented(s, int(n))), nil
		}
		var p string
		if str.IsKnown() {
			p = str.AsString()
		} else {
			p = str.Range().StringPrefix()
		}
		if spaces.IsKnown() {
			n := clamped(spaces, 0, int64(indentBound(len(p))))
			if int64(len(p))+n*int64(strings.Count(p, "\n")) <= int64(indentBound(len(p))) {
				p = indented(p, int(n))
			} else {
				p = p[:strings.IndexByte(p, '\n')+1]
			}
		} else if i := strings.IndexByte(p, '\n'); i >= 0 {
			p = p[:i+1]
		}
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(p)), nil
	},
})
