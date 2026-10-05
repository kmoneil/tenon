package stdlib

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/uni"
)

// matches returns where the non-empty text sep occurs in s, as a search of
// the library finds it (LS-012): scanning from the start, the earliest
// occurrence beginning at or after the end of the one before whose ends are
// both cut positions of s. settled is how much of s holds the cut positions
// the search reads: a position past it, or at it where it is not the end of
// s, is not known to be one, as at the end of a recorded prefix, which what
// follows may extend. The search takes one pass over s, a candidate refused
// for its ends moving it on by one byte.
func matches(s, sep string, cuts uni.Cuts, settled int) []int {
	var out []int
	for i := 0; i+len(sep) <= len(s); {
		j := strings.Index(s[i:], sep)
		if j < 0 {
			break
		}
		j += i
		if end := j + len(sep); cuts.At(j) && cuts.At(end) && end <= settled {
			out = append(out, j)
			i = end
			continue
		}
		i = j + 1
	}
	return out
}

// pieces returns s cut at the matches of sep, each piece a string value.
func pieces(s, sep string, at []int) []tenon.Value {
	out := make([]tenon.Value, 0, len(at)+1)
	start := 0
	for _, j := range at {
		out = append(out, tenon.String(s[start:j]))
		start = j + len(sep)
	}
	return append(out, tenon.String(s[start:]))
}

// SplitFunc cuts a string at each occurrence of a separator, as the
// library's searches find them: only where both ends of the separator are
// cut positions, so no cluster is split, leftmost and without overlap, so
// "aa" in "aaaaa" gives ["", "", "a"]. The string without a separator is
// one element, the empty string among them. An empty separator cuts the
// string into its extended grapheme clusters, the empty string into none,
// where go-cty's cuts code points. Not known yet, the answer is at least
// one longer than the separators the string's recorded prefix settles.
var SplitFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Split",
	Description: "Produces a list of the parts of the given string between occurrences of the separator.",
	Params:      []tenon.Param{stringParam("separator", "The separator."), stringParam("str", "The string.")},
	Result:      tenon.Exactly(tenon.ListType(tenon.StringType())),
	NotNull:     true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		sep, str := args[0], args[1]
		if sep.IsKnown() && str.IsKnown() {
			s, sp := str.AsString(), sep.AsString()
			if sp == "" {
				var out []tenon.Value
				for c := range uni.Clusters(s) {
					out = append(out, tenon.String(c))
				}
				return tenon.List(tenon.StringType(), out...), nil
			}
			return tenon.List(tenon.StringType(), pieces(s, sp, matches(s, sp, uni.CutsOf(s), len(s)))...), nil
		}
		answer := tenon.Narrow(tenon.Unknown(rc.Type()), tenon.NotNull())
		switch {
		case !sep.IsKnown():
		case sep.AsString() == "":
			// The clusters of the string, as many as its length.
			n := tenon.Length(str)
			ns := []tenon.Narrowing{tenon.NotNull()}
			if lo, _, ok := n.Range().NumberMin(); ok {
				l, _ := lo.AsInt64()
				ns = append(ns, tenon.LengthMin(l))
			}
			if hi, _, ok := n.Range().NumberMax(); ok {
				h, _ := hi.AsInt64()
				ns = append(ns, tenon.LengthMax(h))
			}
			answer = tenon.Narrow(tenon.Unknown(rc.Type()), ns...)
		default:
			p := str.Range().StringPrefix()
			found := matches(p, sep.AsString(), uni.CutsOf(p), len(p)-1)
			answer = tenon.Narrow(answer, tenon.LengthMin(int64(len(found))+1))
		}
		return answer, nil
	},
})

// replaceBound returns the most bytes a Replace of these arguments may
// make (LB-031): 64 times their size, and 64 KiB.
func replaceBound(sizes ...int) int {
	n := 0
	for _, s := range sizes {
		n += s
	}
	return 64*n + 64<<10
}

// ReplaceFunc replaces each occurrence of a text in a string, found as
// SplitFunc finds separators: only where both its ends are cut positions,
// leftmost and without overlap. An empty text is found at every boundary
// between clusters, the start and the end among them, so "abc" with "-"
// is "-a-b-c-". The answer is the string value of the text made, which may
// compose across the replacements. An answer of more than 64 times the
// size of the arguments, and 64 KiB, fails with tenon.CodeFunctionTooLarge
// at the replacement, before any of it is made. Not known yet, the answer
// begins with the string's recorded prefix, its occurrences replaced, up to
// where the text that follows could make or end one.
var ReplaceFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Replace",
	Description: "Replaces each occurrence of the given text in the given string with the replacement.",
	Params: []tenon.Param{
		stringParam("str", "The string."),
		stringParam("substr", "The text to find."),
		stringParam("replace", "The text to put in its place."),
	},
	Result:  text,
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		str, substr, replace := args[0], args[1], args[2]
		if str.IsKnown() && substr.IsKnown() && replace.IsKnown() {
			return replaced(str.AsString(), substr.AsString(), replace.AsString()), nil
		}
		ns := []tenon.Narrowing{tenon.NotNull()}
		if p, ok := replacedPrefix(str, substr, replace); ok {
			if str.IsKnown() && p == str.AsString() && !replace.IsKnown() {
				// The search finds nothing, so the replacement is never read.
				return tenon.String(p), nil
			}
			ns = append(ns, tenon.StringPrefix(p))
		}
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), ns...), nil
	},
})

// replaced answers Replace of known arguments.
func replaced(s, sub, rep string) tenon.Value {
	var at []int
	if sub == "" {
		at = append(at, 0)
		i := 0
		for c := range uni.Clusters(s) {
			i += len(c)
			at = append(at, i)
		}
	} else {
		at = matches(s, sub, uni.CutsOf(s), len(s))
	}
	size := len(s) + len(at)*(len(rep)-len(sub))
	if limit := replaceBound(len(s), len(sub), len(rep)); size > limit {
		return tooLarge(2, "Replace: the answer would be "+strconv.Itoa(size)+" bytes, more than "+strconv.Itoa(limit)+
			", 64 times the arguments' size and 64 KiB, the most it makes")
	}
	var b strings.Builder
	b.Grow(size)
	start := 0
	for _, j := range at {
		b.WriteString(s[start:j])
		b.WriteString(rep)
		start = j + len(sub)
	}
	b.WriteString(s[start:])
	return tenon.String(b.String())
}

// replacedPrefix returns what Replace's answer begins with where an
// argument is not known yet, and false where nothing is known of it: the
// known text, the string or its recorded prefix, its settled occurrences
// replaced where the replacement is known, up to the first occurrence where
// it is not, and up to where an occurrence could reach past the text where
// the string is not known yet.
func replacedPrefix(str, substr, replace tenon.Value) (string, bool) {
	if !substr.IsKnown() || substr.AsString() == "" {
		return "", false
	}
	sub := substr.AsString()
	s, settled := "", 0
	if str.IsKnown() {
		s = str.AsString()
		settled = len(s)
	} else {
		s = str.Range().StringPrefix()
		// An occurrence could begin where what is left of the prefix
		// begins the text, and end past it.
		settled = len(s)
		for k := range len(s) + 1 {
			if strings.HasPrefix(sub, s[k:]) {
				settled = k
				break
			}
		}
	}
	at := matches(s[:settled], sub, uni.CutsOf(s), settled)
	if !replace.IsKnown() {
		if len(at) > 0 {
			return s[:at[0]], true
		}
		return s[:settled], true
	}
	var b strings.Builder
	start := 0
	for _, j := range at {
		b.WriteString(s[start:j])
		b.WriteString(replace.AsString())
		start = j + len(sub)
	}
	b.WriteString(s[start:settled])
	return b.String(), true
}

// TrimPrefixFunc removes a prefix from a string where the string begins
// with it and the prefix ends at a cut position, so no cluster is split:
// trimming "q" from "q" and a combining acute leaves the string as it is.
// The prefix is removed once. Not known yet, the answer begins with what
// the string's recorded prefix settles of it.
var TrimPrefixFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "TrimPrefix",
	Description: "Removes the given prefix from the start of the given string, where the string begins with it.",
	Params:      []tenon.Param{stringParam("str", "The string."), stringParam("prefix", "The prefix.")},
	Result:      text,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		str, prefix := args[0], args[1]
		if str.IsKnown() && prefix.IsKnown() {
			s, pre := str.AsString(), prefix.AsString()
			if strings.HasPrefix(s, pre) && uni.CutsOf(s).At(len(pre)) {
				return tenon.String(s[len(pre):]), nil
			}
			return str, nil
		}
		ns := []tenon.Narrowing{tenon.NotNull()}
		if !str.IsKnown() && prefix.IsKnown() {
			p, pre := str.Range().StringPrefix(), prefix.AsString()
			switch {
			case len(pre) < len(p) && strings.HasPrefix(p, pre):
				// The cut at the prefix's end lies within the recorded
				// prefix, which settles it.
				if uni.CutsOf(p).At(len(pre)) {
					ns = append(ns, tenon.StringPrefix(p[len(pre):]))
				} else {
					ns = append(ns, tenon.StringPrefix(p))
				}
			case !strings.HasPrefix(pre, p):
				// The string does not begin with the prefix.
				ns = append(ns, tenon.StringPrefix(p))
			}
		}
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), ns...), nil
	},
})

// TrimSuffixFunc removes a suffix from a string where the string ends with
// it and the suffix begins at a cut position, so no cluster is split:
// trimming a combining acute from "x", "q" and the acute leaves the string
// as it is, and trimming "\n" from "a\r\n" leaves "a\r". The suffix is
// removed once. Not known yet, the answer begins with the string's recorded
// prefix but as many bytes as the suffix holds, which a short continuation
// may take.
var TrimSuffixFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "TrimSuffix",
	Description: "Removes the given suffix from the end of the given string, where the string ends with it.",
	Params:      []tenon.Param{stringParam("str", "The string."), stringParam("suffix", "The suffix.")},
	Result:      text,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		str, suffix := args[0], args[1]
		if str.IsKnown() && suffix.IsKnown() {
			s, suf := str.AsString(), suffix.AsString()
			if k := len(s) - len(suf); strings.HasSuffix(s, suf) && uni.CutsOf(s).At(k) {
				return tenon.String(s[:k]), nil
			}
			return str, nil
		}
		ns := []tenon.Narrowing{tenon.NotNull()}
		if !str.IsKnown() && suffix.IsKnown() {
			p := str.Range().StringPrefix()
			k := max(0, len(p)-len(suffix.AsString()))
			for k > 0 && k < len(p) && !utf8.RuneStart(p[k]) {
				k--
			}
			ns = append(ns, tenon.StringPrefix(p[:k]))
		}
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), ns...), nil
	},
})
