package stdlib

import (
	"regexp"
	"slices"
	"strconv"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/resyntax"
	"github.com/kmoneil/tenon/internal/uni"
)

// patternSyntax returns the failure of argument i, a pattern that is none.
func patternSyntax(fn string, i int, message string) tenon.Value {
	return tenon.ErrorVal(tenon.Diagnostic{Code: tenon.CodeRegexInvalidSyntax, Message: fn + ": " + message, Path: argument(i)})
}

// compilePattern compiles the pattern, argument i of fn: parsed by tenon's
// copy of Go 1.26's regexp/syntax with its Perl flags, whose Unicode
// classes and case folding read the tables of the Unicode version tenon
// states; its case-folding literals made classes of their folding orbits;
// and the text that leaves, naming only explicit ranges, compiled by Go's
// regexp, whose meaning then follows no toolchain's tables.
func compilePattern(fn string, i int, pattern string) (*regexp.Regexp, tenon.Value) {
	re, err := resyntax.Parse(pattern, resyntax.Perl)
	if err != nil {
		if e, ok := err.(*resyntax.Error); ok {
			return nil, patternSyntax(fn, i, string(e.Code)+": "+strconv.Quote(e.Expr))
		}
		return nil, patternSyntax(fn, i, err.Error())
	}
	compiled, err := regexp.Compile(explicitFolds(re).String())
	if err != nil {
		return nil, patternSyntax(fn, i, "the pattern, its case folding made explicit, passes a limit: "+err.Error())
	}
	return compiled, tenon.Value{}
}

// explicitFolds returns re with each literal that case folding applies to
// replaced by the classes of its code points' folding orbits, so its text
// says nothing that Go's own tables would read.
func explicitFolds(re *resyntax.Regexp) *resyntax.Regexp {
	for k, sub := range re.Sub {
		re.Sub[k] = explicitFolds(sub)
	}
	if re.Op != resyntax.OpLiteral || re.Flags&resyntax.FoldCase == 0 {
		return re
	}
	flags := re.Flags &^ resyntax.FoldCase
	var parts []*resyntax.Regexp
	for _, r := range re.Rune {
		orbit := []rune{r}
		for f := uni.SimpleFold(r); f != r; f = uni.SimpleFold(f) {
			orbit = append(orbit, f)
		}
		if len(orbit) == 1 {
			parts = append(parts, &resyntax.Regexp{Op: resyntax.OpLiteral, Flags: flags, Rune: []rune{r}})
			continue
		}
		slices.Sort(orbit)
		class := &resyntax.Regexp{Op: resyntax.OpCharClass, Flags: flags}
		for _, c := range orbit {
			if n := len(class.Rune); n > 0 && class.Rune[n-1] == c-1 {
				class.Rune[n-1] = c
				continue
			}
			class.Rune = append(class.Rune, c, c)
		}
		parts = append(parts, class)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return &resyntax.Regexp{Op: resyntax.OpConcat, Flags: flags, Sub: parts}
}
