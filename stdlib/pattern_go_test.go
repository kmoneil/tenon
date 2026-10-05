//go:build !go1.27

package stdlib

import (
	"math/rand/v2"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/kmoneil/tenon"
)

// This file runs only below go1.27, where Go's regexp reads the Unicode
// version tenon's tables hold: there it is the oracle the explicit
// patterns are held to.

func TestPatternsAgreeWithGo(t *testing.T) {
	if unicode.Version != "15.0.0" {
		t.Fatalf("Go's unicode package is Unicode %s", unicode.Version)
	}
	pieces := []string{"a", "b", "k", "s", `\x{212A}`, `\x{17F}`, `\x{3A3}`, `\x{1E9E}`, "(?i)", "(?-i)", ".", `\pL`, `\p{Greek}`, `[a-z]`, `[^k]`, "(", ")", "|", "*", "+", "?", "{2}", `\w`, `(?P<n>x)`, "^", "$"}
	texts := []string{"", "a", "K", "\U0000212A", "s\U0000017F", "\U000003A3\U000003C2", "\U000000DF\U00001E9E", "abc", "kKk", "\U000003B1x", "nx"}
	rng := rand.New(rand.NewPCG(20261005, 7))
	compared := 0
	for range 20000 {
		var b strings.Builder
		for range 1 + rng.IntN(6) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		p := b.String()
		want, werr := regexp.Compile(p)
		got, failure := compilePattern("Regex", 0, p)
		if (werr == nil) != failure.IsZero() {
			t.Fatalf("pattern %q: Go compiles %v, tenon %v", p, werr == nil, failure.IsZero())
		}
		if werr != nil {
			continue
		}
		compared++
		for _, s := range texts {
			if g, w := got.FindAllStringSubmatchIndex(s, -1), want.FindAllStringSubmatchIndex(s, -1); !slices.EqualFunc(g, w, slices.Equal) {
				t.Fatalf("pattern %q on %q: tenon %v, Go %v", p, s, g, w)
			}
			if !slices.Equal(got.SubexpNames(), want.SubexpNames()) {
				t.Fatalf("pattern %q: groups %v, Go %v", p, got.SubexpNames(), want.SubexpNames())
			}
		}
	}
	if compared < 5000 {
		t.Fatalf("only %d patterns compiled", compared)
	}
}

// goReference reads the references of a Go template, its $$ passed over,
// as Go's Regexp.Expand does: a test's own reading, to hold readExpansion
// to.
var goReference = regexp.MustCompile(`\$(?:\$|\{([\pL\p{Nd}_]+)\}|([\pL\p{Nd}_]+))`)

// goMissing reports whether a Go template refers to a group re does not
// have.
func goMissing(re *regexp.Regexp, template string) bool {
	for _, m := range goReference.FindAllStringSubmatch(template, -1) {
		name := m[1] + m[2]
		if name == "" {
			continue
		}
		if n, err := strconv.Atoi(name); err == nil && len(name) <= 9 && (name == "0" || name[0] != '0') && strings.Trim(name, "0123456789") == "" {
			if n > re.NumSubexp() {
				return true
			}
		} else if !slices.Contains(re.SubexpNames()[1:], name) {
			return true
		}
	}
	return false
}

func TestRegexReplaceAgreesWithGo(t *testing.T) {
	pieces := []string{"a", "b", "(a)", "(b)", "(?P<x>a)", "(?P<n>b)", "|", "*", "+", "?", ".", "^", "$", `\b`, "(", ")", "x*"}
	replacements := []string{"$1", "${1}", "$2", "$0", "$x", "${x}", "$n", "$$", "$", "{", "}", "-", "1", "x", "\U000000E9", "$\U000000E9", " ", "${", "_"}
	texts := []string{"", "a", "b", "ab", "aab", "ba b", "abba", "b\U000000E9a"}
	rng := rand.New(rand.NewPCG(20261005, 13))
	compared, refused := 0, 0
	for range 20000 {
		var p, r strings.Builder
		for range 1 + rng.IntN(4) {
			p.WriteString(pieces[rng.IntN(len(pieces))])
		}
		for range rng.IntN(5) {
			r.WriteString(replacements[rng.IntN(len(replacements))])
		}
		re, err := regexp.Compile(p.String())
		if err != nil {
			continue
		}
		s := texts[rng.IntN(len(texts))]
		got := tenon.Call(RegexReplaceFunc, []tenon.Value{tenon.String(s), tenon.String(p.String()), tenon.String(r.String())}, tenon.Safe)
		if goMissing(re, r.String()) {
			refused++
			if !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeRegexMissingGroup {
				t.Fatalf("RegexReplace(%q, %q, %q) = %v, want regex.missing_group", s, p.String(), r.String(), got)
			}
			continue
		}
		compared++
		if want := tenon.String(re.ReplaceAllString(s, r.String())); !got.Equal(want) {
			t.Fatalf("RegexReplace(%q, %q, %q) = %v, Go %v", s, p.String(), r.String(), got, want)
		}
	}
	t.Logf("compared %d, refused %d", compared, refused)
	if compared < 5000 || refused < 1000 {
		t.Fatalf("compared %d and refused %d", compared, refused)
	}
}
