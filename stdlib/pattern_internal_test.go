package stdlib

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// compiles reports whether pattern compiles, and fails t with the code and
// path of the failure where want says it does not.
func compiles(t *testing.T, pattern string) bool {
	t.Helper()
	_, failure := compilePattern("Regex", 0, pattern)
	if failure.IsZero() {
		return true
	}
	if d := failure.Diagnostics()[0]; d.Code != tenon.CodeRegexInvalidSyntax || !d.Path.Equal(argument(0)) {
		t.Errorf("pattern %q fails with %s at %v, want %s at [0]", pattern, d.Code, d.Path, tenon.CodeRegexInvalidSyntax)
	}
	return false
}

// patternMatches reports whether pattern, which compiles, matches s.
func patternMatches(t *testing.T, pattern, s string) bool {
	t.Helper()
	re, failure := compilePattern("Regex", 0, pattern)
	if !failure.IsZero() {
		t.Fatalf("pattern %q fails: %v", pattern, failure)
	}
	return re.MatchString(s)
}

func TestConformance_LR001_Syntax(t *testing.T) {
	conformance.Covers(t, "LR-001")
	for _, p := range []string{
		``, `a|b`, `(a)(?:b)(?P<x>c)(?<y>d)`, `a*?b+?c??d{2,3}?`, `^\A\z$\b\B`, `[[:alpha:]]`, `\pL\p{Greek}\PN\p{Letter}\p{greek}`,
		`(?i)k(?-i)k(?ms).`, `\x{10FFFF}\Q*+?\E`,
	} {
		if !compiles(t, p) {
			t.Errorf("pattern %q does not compile", p)
		}
	}
	for _, p := range []string{`(`, `)`, `[a`, `a**`, `\1`, `(?=a)`, `(?!a)`, `(?<=a)`, `\C`, `\Z`, `(?P<a b>x)`, `x{2,1}`, `\p{NoSuchClass}`} {
		if compiles(t, p) {
			t.Errorf("pattern %q compiles", p)
		}
	}
	// The Perl and POSIX classes are ASCII; \pN is Unicode's.
	arabicThree := "\U00000663"
	if patternMatches(t, `^\d$`, arabicThree) || !patternMatches(t, `^\pN$`, arabicThree) {
		t.Errorf("\\d and \\pN of an Arabic-Indic three: want no and yes")
	}
	if !patternMatches(t, `^\w+$`, "abc_1") || patternMatches(t, `^\w+$`, "\U000000E9t\U000000E9") {
		t.Errorf("\\w: want ASCII word characters alone")
	}
}

func TestConformance_LR002_Limits(t *testing.T) {
	conformance.Covers(t, "LR-002")
	if !compiles(t, `a{1000}`) {
		t.Error("a{1000} does not compile")
	}
	for _, p := range []string{`a{1001}`, `(a{100}){100}`, strings.Repeat("(", 1001) + "a" + strings.Repeat(")", 1001)} {
		if compiles(t, p) {
			t.Errorf("pattern of %d bytes passing a limit compiles", len(p))
		}
	}
}

func TestConformance_LR003_Unicode(t *testing.T) {
	conformance.Covers(t, "LR-003")
	// Unicode 15.0.0's properties, on every toolchain: a Garay letter, of
	// Unicode 16.0, is no letter, and its script is no script yet.
	garay := "\U00010D50"
	if patternMatches(t, `^\pL$`, garay) {
		t.Error("\\pL matches a letter of Unicode 16.0")
	}
	if compiles(t, `\p{Garay}`) {
		t.Error("\\p{Garay} compiles")
	}
	if !patternMatches(t, `^\p{Greek}+$`, "\U000003B1\U000003B2") || !patternMatches(t, `^\p{Lu}$`, "A") {
		t.Error("\\p{Greek} and \\p{Lu}")
	}
	// Simple case folding of Unicode 15.0.0: the Kelvin sign is a k, and
	// the capital sharp s folds with the small one.
	if !patternMatches(t, `^(?i)k$`, "\U0000212A") || !patternMatches(t, `^(?i)\x{1E9E}$`, "\U000000DF") {
		t.Error("(?i) folding: k and the Kelvin sign, the two sharp s")
	}
	if !patternMatches(t, `^(?i)\x{3C3}+$`, "\U000003A3\U000003C2\U000003C3") {
		t.Error("(?i) folding: the three sigmas")
	}
	if patternMatches(t, `^(?i)k$`, "x") {
		t.Error("(?i)k matches x")
	}
}
