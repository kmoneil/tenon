package stdlib

import (
	"regexp"
	"slices"
	"strconv"

	"github.com/kmoneil/tenon"
)

// patternShape returns the type of what a match of re gives: a string
// where it has no group, a tuple of a string for each group where its
// groups are all unnamed, and an object of a string for each name where
// they are all named. A pattern mixing the two fails with
// tenon.CodeRegexMixedGroups, and one naming a group twice with
// tenon.CodeRegexDuplicateGroup, both at the pattern, argument i.
func patternShape(fn string, i int, re *regexp.Regexp) (tenon.Type, tenon.Value) {
	names := re.SubexpNames()[1:]
	if len(names) == 0 {
		return tenon.StringType(), tenon.Value{}
	}
	named := 0
	for _, n := range names {
		if n != "" {
			named++
		}
	}
	switch named {
	case 0:
		types := make([]tenon.Type, len(names))
		for k := range types {
			types[k] = tenon.StringType()
		}
		return tenon.TupleType(types...), tenon.Value{}
	case len(names):
		attrs := map[string]tenon.Type{}
		for k, n := range names {
			if slices.Contains(names[:k], n) {
				return tenon.Type{}, tenon.ErrorVal(tenon.Diagnostic{
					Code:    tenon.CodeRegexDuplicateGroup,
					Message: fn + ": the pattern names the group " + strconv.Quote(n) + " twice, and one capture would be lost",
					Path:    argument(i),
				})
			}
			attrs[n] = tenon.StringType()
		}
		return tenon.ObjectType(attrs), tenon.Value{}
	}
	return tenon.Type{}, tenon.ErrorVal(tenon.Diagnostic{
		Code:    tenon.CodeRegexMixedGroups,
		Message: fn + ": the pattern has named and unnamed groups both, and a match would have no one shape",
		Path:    argument(i),
	})
}

// knownPattern compiles the known pattern, argument i, and returns its
// shape, or the failure that it is no pattern or has no shape.
func knownPattern(fn string, i int, pattern string) (*regexp.Regexp, tenon.Type, tenon.Value) {
	re, failure := compilePattern(fn, i, pattern)
	if !failure.IsZero() {
		return nil, tenon.Type{}, failure
	}
	shape, failure := patternShape(fn, i, re)
	return re, shape, failure
}

// captured returns what a match of re in s at loc gives, of the type t:
// the match, or its groups, each a string value, null where the group took
// no part in it.
func captured(re *regexp.Regexp, s string, loc []int, t tenon.Type) tenon.Value {
	group := func(k int) tenon.Value {
		if loc[2*k] < 0 {
			return tenon.Null(tenon.StringType())
		}
		return tenon.String(s[loc[2*k]:loc[2*k+1]])
	}
	switch t.Kind() {
	case tenon.KindString:
		return group(0)
	case tenon.KindTuple:
		elems := make([]tenon.Value, re.NumSubexp())
		for k := range elems {
			elems[k] = group(k + 1)
		}
		return tenon.Tuple(elems...)
	}
	attrs := map[string]tenon.Value{}
	for k, n := range re.SubexpNames()[1:] {
		attrs[n] = group(k + 1)
	}
	return tenon.Object(attrs)
}

// captureBound returns the most a Regex or RegexAll of a pattern and a
// string of these sizes may answer (LB-031): 64 times their size, and 64
// KiB, its size counting each capture's bytes and one more.
func captureBound(pattern, s string) int { return replaceBound(len(pattern), len(s)) }

// captureSize returns what the answer of a match of re at loc counts
// toward captureBound: one more than the bytes of each capture, the match
// itself where re has no group, a group that took no part counting one.
func captureSize(re *regexp.Regexp, loc []int) int {
	if re.NumSubexp() == 0 {
		return loc[1] - loc[0] + 1
	}
	n := 0
	for k := 1; k <= re.NumSubexp(); k++ {
		n++
		if loc[2*k] >= 0 {
			n += loc[2*k+1] - loc[2*k]
		}
	}
	return n
}

// capturesTooLarge returns the failure of an answer of fn passing limit.
func capturesTooLarge(fn string, limit int) tenon.Value {
	return tooLarge(0, fn+": the answer passes "+strconv.Itoa(limit)+
		", 64 times the arguments' size and 64 KiB, counting each capture's bytes and one more, the most it makes")
}

// RegexFunc matches a pattern against a string, the first match from the
// start, leftmost-first, over the string's scalar values: where the pattern
// has no group, the text it matched; where its groups are unnamed, a tuple
// of their texts; and where they are named, an object of them by name; a
// group that took no part is null. A pattern mixing named and unnamed
// groups fails with tenon.CodeRegexMixedGroups and one naming a group twice
// with tenon.CodeRegexDuplicateGroup, where go-cty's keeps the later
// capture; no match fails with tenon.CodeRegexNoMatch; and an answer
// whose captures, each counting its bytes and one more, pass 64 times the
// arguments' size and 64 KiB fails with tenon.CodeFunctionTooLarge. A
// pattern not known yet leaves the answer's type open.
var RegexFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Regex",
	Description: "Applies the given pattern to the given string and returns what its first match captures.",
	Params: []tenon.Param{
		stringParam("pattern", "The pattern."),
		{Name: "string", Description: "The string.", Constraint: text},
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if !args[0].IsKnown() {
			return tenon.Any(), nil
		}
		_, shape, failure := knownPattern("Regex", 0, args[0].AsString())
		if !failure.IsZero() {
			return tenon.Constraint{}, tenon.NewError(failure)
		}
		return tenon.Exactly(shape), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		if !args[0].IsKnown() {
			return unknownOf(rc), nil
		}
		re, shape, _ := knownPattern("Regex", 0, args[0].AsString())
		s := args[1].AsString()
		loc := re.FindStringSubmatchIndex(s)
		if loc == nil {
			return tenon.ErrorVal(tenon.Diagnostic{
				Code:    tenon.CodeRegexNoMatch,
				Message: "Regex: the pattern matches no part of the string",
				Path:    argument(1),
			}), nil
		}
		if limit := captureBound(args[0].AsString(), s); captureSize(re, loc) > limit {
			return capturesTooLarge("Regex", limit), nil
		}
		return captured(re, s, loc, shape), nil
	},
})

// RegexAllFunc is every match of a pattern in a string, as RegexFunc gives
// each, in order: matches do not overlap, and an empty match just after
// another is passed over, as Go's regexp passes it over. No match is the
// empty list. It is bounded as RegexFunc is, over all its matches. A
// pattern not known yet leaves the elements' type open.
var RegexAllFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "RegexAll",
	Description: "Applies the given pattern to the given string and returns a list of what each of its matches captures.",
	Params: []tenon.Param{
		stringParam("pattern", "The pattern."),
		{Name: "string", Description: "The string.", Constraint: text},
	},
	ResultOf: func(args []tenon.Value, _ tenon.Policy) (tenon.Constraint, error) {
		if !args[0].IsKnown() {
			return tenon.ListOf(tenon.Any()), nil
		}
		_, shape, failure := knownPattern("RegexAll", 0, args[0].AsString())
		if !failure.IsZero() {
			return tenon.Constraint{}, tenon.NewError(failure)
		}
		return tenon.Exactly(tenon.ListType(shape)), nil
	},
	NotNull: true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		if !args[0].IsKnown() {
			return unknownOf(rc), nil
		}
		re, shape, _ := knownPattern("RegexAll", 0, args[0].AsString())
		s := args[1].AsString()
		// Each match counts at least one for each group, or for itself:
		// more matches than this would pass the bound, so no more are
		// taken, and what is held for them stays near the bound.
		limit := captureBound(args[0].AsString(), s)
		locs := re.FindAllStringSubmatchIndex(s, limit/max(1, re.NumSubexp())+1)
		size := 0
		for _, loc := range locs {
			if size += captureSize(re, loc); size > limit {
				return capturesTooLarge("RegexAll", limit), nil
			}
		}
		out := make([]tenon.Value, len(locs))
		for k, loc := range locs {
			out[k] = captured(re, s, loc, shape)
		}
		return tenon.List(shape, out...), nil
	},
})
