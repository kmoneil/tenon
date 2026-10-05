package stdlib_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// regexReplace calls RegexReplace with the string, the pattern and the
// replacement.
func regexReplace(s, pattern, replace string) tenon.Value {
	return call(stdlib.RegexReplaceFunc, tenon.String(s), tenon.String(pattern), tenon.String(replace))
}

func TestConformance_LR010_RegexReplace(t *testing.T) {
	conformance.Covers(t, "LR-010")
	for _, tt := range []struct{ s, pattern, replace, want string }{
		{"abc", "b", "X", "aXc"},
		{"a1b22", `\d+`, "#", "a#b#"},
		// Matches as RegexAll finds them: an empty match just after
		// another is passed over.
		{"abc", "x*", "-", "-a-b-c-"},
		{"baaab", "a*", "-", "-b-b-"},
		{"", "", "-", "-"},
		{"abc", "x", "-", "abc"},
		// Leftmost-first, over scalar values.
		{"ab", "a|ab", "X", "Xb"},
		{"q\U00000301", ".", "x", "xx"},
		// The text made is a string value: an acute after an e composes.
		{"e-y", "-", "\U00000301", "\U000000E9y"},
		// Named and unnamed groups both, and a name used twice, are no
		// failure here: a name refers to the first group so named that
		// took part.
		{"ab", `(?P<x>a)(b)`, "$2${x}", "ba"},
		{"b", `(?P<x>a)|(?P<x>b)`, "[${x}]", "[b]"},
	} {
		if got := regexReplace(tt.s, tt.pattern, tt.replace); !got.Equal(tenon.String(tt.want)) {
			t.Errorf("RegexReplace(%q, %q, %q) = %v, want %q", tt.s, tt.pattern, tt.replace, got, tt.want)
		}
	}
	failsWith(t, "RegexReplace of no pattern", regexReplace("ab", "(", "x"), tenon.CodeRegexInvalidSyntax, at(1))
}

func TestConformance_LR011_Expansion(t *testing.T) {
	conformance.Covers(t, "LR-011")
	for _, tt := range []struct{ s, pattern, replace, want string }{
		{"abc", "(b)", "[$1]", "a[b]c"},
		{"abc", "(b)", "[${1}x]", "a[bx]c"},
		{"abc", "(b)", "[$0$0]", "a[bb]c"},
		{"abc", "(?P<n>b)", "[$n${n}]", "a[bb]c"},
		{"abc", "(?P<n_1>b)", "[$n_1]", "a[b]c"},
		// $$ is a $, and a $ beginning no reference is literal: at the
		// end, before what no name begins with, and before a { that no
		// name and } follow.
		{"abc", "b", "$$", "a$c"},
		{"abc", "b", "$$1", "a$1c"},
		{"abc", "b", "x$", "ax$c"},
		{"abc", "b", "$-$ $.", "a$-$ $.c"},
		{"abc", "(b)", "[${1]", "a[${1]c"},
		{"abc", "(b)", "[${}]", "a[${}]c"},
		{"abc", "(b)", "[${1-}]", "a[${1-}]c"},
		{"abc", "b", `\1`, `a\1c`},
		// A group that took no part is empty.
		{"b", "(a)|(b)", "[$1|$2]", "[|b]"},
		// A number with a leading zero is a name.
		{"abc", "(?P<01>b)", "[$01]", "a[b]c"},
		// The name ends where the letters and digits of Unicode 15.0.0
		// end: a letter of Unicode 16.0's Garay script ends it, as it does
		// on every Go toolchain.
		{"abc", "(b)", "$1\U00010D50", "ab\U00010D50c"},
	} {
		if got := regexReplace(tt.s, tt.pattern, tt.replace); !got.Equal(tenon.String(tt.want)) {
			t.Errorf("RegexReplace(%q, %q, %q) = %v, want %q", tt.s, tt.pattern, tt.replace, got, tt.want)
		}
	}
}

func TestConformance_LR012_MissingGroup(t *testing.T) {
	conformance.Covers(t, "LR-012")
	for _, tt := range []struct{ pattern, replace string }{
		{"(b)", "$2"},
		{"b", "$1"},
		{"(b)", "${x}"},
		// The name is 1x, not group 1 followed by an x; and a letter or
		// digit beyond ASCII continues it.
		{"(b)", "[$1x]"},
		{"(b)", "$1\U000000E9"},
		{"(b)", "$1\U00000663"},
		{"(b)", "$01"},
		// Nine digits are a number, ten a name.
		{"(b)", "$100000000"},
		{"(b)", "$1000000000"},
	} {
		failsWith(t, "RegexReplace(abc, "+tt.pattern+", "+tt.replace+")", regexReplace("abc", tt.pattern, tt.replace), tenon.CodeRegexMissingGroup, at(2))
	}
	got := regexReplace("abc", "(b)", "[$1x]")
	if msg := got.Diagnostics()[0].Message; !strings.Contains(msg, `"1x"`) || !strings.Contains(msg, "${1}x") {
		t.Errorf("the message of $1x = %q, want it to name 1x and say ${1}x", msg)
	}
}

func TestConformance_LR013_Bound(t *testing.T) {
	conformance.Covers(t, "LR-013")
	// The empty pattern matches at each of the 1,001 positions of 1,000
	// bytes: k bytes of replacement make 1,000 + 1,001k bytes, against 64
	// times 1,000 + k, and 65,536. At 137 bytes that is 138,137 against
	// 138,304; at 138, 139,138 against 138,368.
	s := strings.Repeat("a", 1000)
	if got := regexReplace(s, "", strings.Repeat("x", 137)); got.IsError() || len(got.AsString()) != 138137 {
		t.Errorf("RegexReplace under the bound = %v", got)
	}
	failsWith(t, "RegexReplace past the bound", regexReplace(s, "", strings.Repeat("x", 138)), tenon.CodeFunctionTooLarge, at(2))
	// References are measured: a thousand references to a group of one
	// byte, matched once in 100,001, could make 100 MB by the most each may
	// write, and make 101,000.
	far := strings.Repeat("b", 100000) + "a"
	if got := regexReplace(far, "(a)", strings.Repeat("$1", 1000)); got.IsError() || len(got.AsString()) != 101000 {
		t.Errorf("RegexReplace of a group matched once = %v", got)
	}
	// A hundred references to a group of 3,000 bytes make 300,000 bytes,
	// against 64 times 3,207 and 65,536, 270,784.
	failsWith(t, "RegexReplace of many references", regexReplace(strings.Repeat("a", 3000), "(a+)", strings.Repeat("${1}", 100)), tenon.CodeFunctionTooLarge, at(2))
}

func TestConformance_LR014_NotKnown(t *testing.T) {
	conformance.Covers(t, "LR-014")
	str := tenon.Unknown(tenon.StringType())
	// The replacement not known yet: the string where nothing matches, and
	// otherwise what comes before the first match, "ab ", kept whole since
	// a space composes with no mark the replacement may begin with
	// (UN-006).
	if got := call(stdlib.RegexReplaceFunc, tenon.String("xyz"), tenon.String(`\d`), str); !got.Equal(tenon.String("xyz")) {
		t.Errorf("RegexReplace with no match and the replacement unknown = %v, want xyz", got)
	}
	got := call(stdlib.RegexReplaceFunc, tenon.String("ab 1"), tenon.String(`\d`), str)
	if got.IsKnown() || got.Range().StringPrefix() != "ab " || !notNull(got) {
		t.Errorf("RegexReplace with the replacement unknown = %v, want the unknown string beginning %q, not null", got, "ab ")
	}
	// For random strings, patterns and replacements, the answer for the
	// replacement begins with what was promised without it.
	patterns := []string{`\d`, "b", "a|b", "x*", "(a)", "^", "$", `\s+`, "(?i)A"}
	promisesHoldFor(t, stdlib.RegexReplaceFunc, 2, func(rng *rand.Rand) []tenon.Value {
		return []tenon.Value{tenon.String(randomText(rng, 8)), tenon.String(patterns[rng.IntN(len(patterns))]), {}}
	})
	// The string or the pattern not known yet: the unknown string.
	for _, args := range [][]tenon.Value{
		{str, tenon.String("b"), tenon.String("x")},
		{tenon.String("abc"), str, tenon.String("x")},
		{str, str, str},
	} {
		if got := call(stdlib.RegexReplaceFunc, args...); got.IsKnown() || !got.Type().Equal(tenon.StringType()) || !notNull(got) {
			t.Errorf("RegexReplace(%v) = %v, want the unknown string, not null", args, got)
		}
	}
	// What the known pattern and replacement settle fails now.
	failsWith(t, "RegexReplace of no pattern, the string unknown", call(stdlib.RegexReplaceFunc, str, tenon.String("("), tenon.String("x")), tenon.CodeRegexInvalidSyntax, at(1))
	failsWith(t, "RegexReplace of a missing group, the string unknown", call(stdlib.RegexReplaceFunc, str, tenon.String("(b)"), tenon.String("$2")), tenon.CodeRegexMissingGroup, at(2))
}
