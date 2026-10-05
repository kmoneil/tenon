package stdlib

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/uni"
)

// reference is a reference to a group in a RegexReplace replacement: by
// number where num is not negative, and by name otherwise.
type reference struct {
	name string
	num  int
}

// token returns the reference as Go's Regexp.Expand reads it whatever the
// toolchain's Unicode: braced, a number by its digits and a name, which
// names a group of the pattern only in ASCII, as it is.
func (r reference) token() string {
	if r.num >= 0 {
		return "${" + strconv.Itoa(r.num) + "}"
	}
	return "${" + r.name + "}"
}

// expansion is a RegexReplace replacement read: its literal text, each
// piece before the reference of its index and the last after them all.
type expansion struct {
	lits []string
	refs []reference
}

// readExpansion reads a replacement as Go's Regexp.Expand reads a
// template, with the letters and digits of the Unicode version tenon
// states (LR-011): $$ is a $; $name and ${name} refer to a group, name the
// longest run of letters, decimal digits and underscores; and a $ that
// begins neither is literal.
func readExpansion(r string) expansion {
	var e expansion
	var lit strings.Builder
	for {
		before, after, found := strings.Cut(r, "$")
		lit.WriteString(before)
		if !found {
			break
		}
		r = after
		if strings.HasPrefix(r, "$") {
			lit.WriteByte('$')
			r = r[1:]
			continue
		}
		ref, rest, ok := readReference(r)
		if !ok {
			lit.WriteByte('$')
			continue
		}
		e.lits = append(e.lits, lit.String())
		e.refs = append(e.refs, ref)
		lit.Reset()
		r = rest
	}
	e.lits = append(e.lits, lit.String())
	return e
}

// readReference reads the reference r begins with, after its $, and what
// follows it, or reports there is none.
func readReference(r string) (reference, string, bool) {
	braced := strings.HasPrefix(r, "{")
	s := strings.TrimPrefix(r, "{")
	n := 0
	for n < len(s) {
		c, w := utf8.DecodeRuneInString(s[n:])
		if c != '_' && !unicode.Is(uni.Categories["L"], c) && !unicode.Is(uni.Categories["Nd"], c) {
			break
		}
		n += w
	}
	if n == 0 {
		return reference{}, "", false
	}
	name, rest := s[:n], s[n:]
	if braced {
		if !strings.HasPrefix(rest, "}") {
			return reference{}, "", false
		}
		rest = rest[1:]
	}
	return reference{name: name, num: groupNumber(name)}, rest, true
}

// groupNumber returns the number a name refers to a group by, a name of at
// most nine ASCII digits not beginning with 0 unless it is 0, and -1 for
// any other name, which refers to a group by name.
func groupNumber(name string) int {
	if len(name) > 9 || len(name) > 1 && name[0] == '0' {
		return -1
	}
	num := 0
	for i := range len(name) {
		if name[i] < '0' || name[i] > '9' {
			return -1
		}
		num = num*10 + int(name[i]-'0')
	}
	return num
}

// missingGroup returns the failure of a replacement, argument 2, that
// refers to a group re does not have (LR-012), or the zero Value.
func missingGroup(re *regexp.Regexp, e expansion) tenon.Value {
	for _, ref := range e.refs {
		var what string
		switch {
		case ref.num > re.NumSubexp():
			what = "group " + strconv.Itoa(ref.num) + ", which the pattern does not have (it has " + strconv.Itoa(re.NumSubexp()) + ")"
		case ref.num < 0 && !slices.Contains(re.SubexpNames()[1:], ref.name):
			what = "a group named " + strconv.Quote(ref.name) + ", which the pattern does not have"
			// $1x names the group 1x: say how to write group 1 and an x.
			if k := strings.IndexFunc(ref.name, func(c rune) bool { return c < '0' || c > '9' }); k > 0 {
				if num := groupNumber(ref.name[:k]); num >= 0 && num <= re.NumSubexp() {
					what += "; ${" + ref.name[:k] + "}" + ref.name[k:] + " is group " + ref.name[:k] + " followed by " + strconv.Quote(ref.name[k:])
				}
			}
		default:
			continue
		}
		return tenon.ErrorVal(tenon.Diagnostic{
			Code:    tenon.CodeRegexMissingGroup,
			Message: "RegexReplace: the replacement refers to " + what,
			Path:    argument(2),
		})
	}
	return tenon.Value{}
}

// regexReplaced answers RegexReplace of known arguments, the replacement
// read and its references checked.
func regexReplaced(re *regexp.Regexp, s, pattern, replace string, e expansion) tenon.Value {
	var template strings.Builder
	lit := 0
	for k, l := range e.lits {
		lit += len(l)
		template.WriteString(strings.ReplaceAll(l, "$", "$$"))
		if k < len(e.refs) {
			template.WriteString(e.refs[k].token())
		}
	}
	// Each of at most len(s)+1 matches writes the literal text, and each
	// reference over all of them at most the string, since matches do not
	// overlap. Where that may pass the bound, the answer is measured, a
	// pass for the matches and one for each group referred to, none of
	// them longer than the string, before it is made.
	limit := int64(replaceBound(len(s), len(pattern), len(replace)))
	n := int64(len(s))
	if n+(n+1)*int64(lit)+int64(len(e.refs))*n > limit {
		matches, matched := int64(0), int64(0)
		unmatched := int64(len(re.ReplaceAllStringFunc(s, func(m string) string {
			matches++
			matched += int64(len(m))
			return ""
		})))
		size := unmatched + matches*int64(lit)
		counts := map[string]int64{}
		for _, ref := range e.refs {
			counts[ref.token()]++
		}
		for _, token := range slices.Sorted(maps.Keys(counts)) {
			if size > limit {
				break
			}
			referred := matched
			if token != "${0}" {
				referred = int64(len(re.ReplaceAllString(s, token))) - unmatched
			}
			size += counts[token] * referred
		}
		if size > limit {
			return tooLarge(2, "RegexReplace: the answer would be more than "+strconv.FormatInt(limit, 10)+
				" bytes, 64 times the arguments' size and 64 KiB, the most it makes")
		}
	}
	return tenon.String(re.ReplaceAllString(s, template.String()))
}

// RegexReplaceFunc replaces each match of a pattern in a string, as
// RegexAllFunc finds them, with the replacement, which may refer to what
// the match captured as Go's Regexp.Expand reads a template: $1 or ${1}
// by number, $name or ${name} by name, $$ a $. A name is read with the
// letters and digits of the Unicode version tenon states, so $1x refers
// to a group named 1x and ${1}x to group 1 and an x; a reference to a
// group the pattern does not have fails with tenon.CodeRegexMissingGroup,
// where go-cty's writes nothing for it. The answer is the string value of
// the text made, which may compose across the replacements; an answer of
// more than 64 times the size of the arguments, and 64 KiB, fails with
// tenon.CodeFunctionTooLarge at the replacement, before any of it is made.
// A replacement not known yet leaves the answer beginning with the text
// before the first match, and the string itself where there is none.
var RegexReplaceFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "RegexReplace",
	Description: "Applies the given pattern to the given string and replaces all matches with the given replacement.",
	Params: []tenon.Param{
		{Name: "str", Description: "The string.", Constraint: text},
		{Name: "pattern", Description: "The pattern.", Constraint: text},
		stringParam("replace", "The replacement, which may refer to the groups of each match."),
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if !args[1].IsKnown() {
			return text, nil
		}
		re, failure := compilePattern("RegexReplace", 1, args[1].AsString())
		if failure.IsZero() && args[2].IsKnown() {
			failure = missingGroup(re, readExpansion(args[2].AsString()))
		}
		if !failure.IsZero() {
			return tenon.Constraint{}, tenon.NewError(failure)
		}
		return text, nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		s, pattern := args[0].AsString(), args[1].AsString()
		re, _ := compilePattern("RegexReplace", 1, pattern)
		if !args[2].IsKnown() {
			loc := re.FindStringIndex(s)
			if loc == nil {
				return args[0], nil
			}
			return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(s[:loc[0]])), nil
		}
		replace := args[2].AsString()
		return regexReplaced(re, s, pattern, replace, readExpansion(replace)), nil
	},
})
