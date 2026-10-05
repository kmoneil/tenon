package stdlib

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/uni"
)

// text is the constraint of a String parameter or result.
var text = tenon.Exactly(tenon.StringType())

// stringParam returns a parameter taking a string, admitting one not known
// yet, so that an answer can keep what the argument's recorded prefix
// settles of it.
func stringParam(name, description string) tenon.Param {
	return tenon.Param{Name: name, Description: description, Constraint: text, AllowUnknown: true}
}

// mapping returns a function of one string answering the string value of
// f of it. Where the string is not known yet, the answer is the unknown
// string beginning with prefix of its recorded prefix: what f of any string
// beginning with that prefix begins with.
func mapping(name, description string, f, prefix func(string) string) tenon.Function {
	return tenon.NewFunction(tenon.FunctionSpec{
		Name:        name,
		Description: description,
		Params:      []tenon.Param{stringParam("str", "The string.")},
		Result:      text,
		NotNull:     true,
		Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
			s := args[0]
			if !s.IsKnown() {
				p := prefix(s.Range().StringPrefix())
				return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(p)), nil
			}
			return tenon.String(f(s.AsString())), nil
		},
	})
}

// UpperFunc is a string in uppercase, by Unicode's default case conversion
// of the version tenon states: each code point's full uppercase mapping,
// with no language's tailoring, so "straße" is "STRASSE" where go-cty's,
// on Go's simple mapping, is "STRAßE". Not known yet, the answer begins
// with the uppercase of the argument's recorded prefix.
var UpperFunc = mapping("Upper",
	"Returns the given string in uppercase, by Unicode's default case conversion.",
	uni.Upper, uni.Upper)

// LowerFunc is a string in lowercase, as UpperFunc is in uppercase, a
// capital sigma ending a word being the final sigma: "ΟΔΟΣ" is "οδος".
// Not known yet, the answer begins with the lowercase of the argument's
// recorded prefix up to a capital sigma whose end of word what follows the
// prefix decides.
var LowerFunc = mapping("Lower",
	"Returns the given string in lowercase, by Unicode's default case conversion.",
	uni.Lower, uni.LowerPrefix)

// TitleFunc is a string with each code point that begins it or follows a
// separator replaced by its full titlecase mapping, and the rest as they
// are: go-cty's rule, written down. A separator is an ASCII code point
// other than a letter, a digit or the underscore, or a White_Space code
// point, so "o'neil" is "O'Neil", "foo.example.com" is "Foo.Example.Com",
// "hello_world" is "Hello_world" and "hELLO" is "HELLO"; punctuation outside
// ASCII separates nothing. The titlecase mapping is the full one, so
// "ßtraße" is "Sstraße". Not known yet, the answer begins with the title of
// the argument's recorded prefix.
var TitleFunc = mapping("Title",
	"Replaces the first code point of the string, and each one after a separator, with its titlecase equivalent.",
	title, title)

// title applies TitleFunc's rule to s.
func title(s string) string {
	b := make([]byte, 0, len(s))
	after := true
	for _, r := range s {
		if after {
			b = append(b, uni.Title(r)...)
		} else {
			b = utf8.AppendRune(b, r)
		}
		after = separator(r)
	}
	return string(b)
}

// separator reports whether r separates words for TitleFunc: an ASCII code
// point other than a letter, a digit or the underscore, or a White_Space
// code point.
func separator(r rune) bool {
	if r < utf8.RuneSelf {
		return !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '_')
	}
	return uni.IsWhiteSpace(r)
}

// StrlenFunc is how many extended grapheme clusters a string holds: its
// length (ST-005), as LengthFunc gives it for a string. Not known yet, the
// answer is within the lengths the string's range allows.
var StrlenFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Strlen",
	Description: "Returns the number of grapheme clusters in the given string.",
	Params:      []tenon.Param{stringParam("str", "The string.")},
	Result:      number,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		return tenon.Length(args[0]), nil
	},
})

// ReverseFunc is a string with its extended grapheme clusters in reverse
// order. The answer is the string value of them, so where clusters meet
// anew they may compose or segment otherwise: reversing a combining acute
// and an e after it gives "é", one cluster, and three regional indicators
// pair afresh. Reversing twice need not give the string back.
var ReverseFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Reverse",
	Description: "Returns the given string with its grapheme clusters in reverse order.",
	Params:      []tenon.Param{{Name: "str", Description: "The string.", Constraint: text}},
	Result:      text,
	NotNull:     true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		s := args[0].AsString()
		clusters := slices.Collect(uni.Clusters(s))
		slices.Reverse(clusters)
		return tenon.String(strings.Join(clusters, "")), nil
	},
})

// clamped returns the whole number v within lo and hi, lo where it is less
// and hi where it is more, whatever its magnitude.
func clamped(v tenon.Value, lo, hi int64) int64 {
	switch {
	case less(v, tenon.NumberFromInt(lo)):
		return lo
	case less(tenon.NumberFromInt(hi), v):
		return hi
	}
	n, _ := v.AsInt64()
	return n
}

// whole checks that the known number v, argument i, is a whole number, of
// any magnitude.
func whole(fn string, i int, what string, v tenon.Value) (tenon.Value, bool) {
	if fractional(v) {
		return invalid(i, fn+": the "+what+" "+v.String()+" is not a whole number"), false
	}
	return tenon.Value{}, true
}

// SubstrFunc is the part of a string that begins length extended grapheme
// clusters after offset: a negative offset counts from the end, and a
// position still before the start is the start; an offset at or past the
// end gives the empty string; a length of zero gives the empty string
// whatever the offset, where go-cty's gives the rest for a negative offset
// (#217); a negative length takes the rest. Offset and length are whole
// numbers of any magnitude, and a fraction fails at it with
// tenon.CodeFunctionInvalidArgument. Not known yet, the answer is at most
// length long, or as long as a negative offset counts; where the string is
// not known yet and the offset and length are, the clusters its recorded
// prefix settles are read, the answer known where they hold it all.
var SubstrFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Substr",
	Description: "Extracts a substring from the given string, counting grapheme clusters.",
	Params: []tenon.Param{
		stringParam("str", "The string."),
		{Name: "offset", Description: "The cluster to begin at, counting from the end where negative.", Constraint: number, AllowUnknown: true},
		{Name: "length", Description: "How many clusters to take, the rest where negative.", Constraint: number, AllowUnknown: true},
	},
	Result:  text,
	NotNull: true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		str, offset, length := args[0], args[1], args[2]
		for i, v := range []tenon.Value{offset, length} {
			if v.IsKnown() {
				if failure, ok := whole("Substr", i+1, []string{"offset", "length"}[i], v); !ok {
					return failure, nil
				}
			}
		}
		if length.IsKnown() && length.Equal(zero) {
			return tenon.String(""), nil
		}
		if str.IsKnown() && offset.IsKnown() && length.IsKnown() {
			clusters := slices.Collect(uni.Clusters(str.AsString()))
			return tenon.String(strings.Join(substr(clusters, offset, length), "")), nil
		}
		return substrNotKnown(str, offset, length), nil
	},
})

// substr returns the clusters Substr takes of clusters, offset and length
// known.
func substr(clusters []string, offset, length tenon.Value) []string {
	n := int64(len(clusters))
	o := clamped(offset, -n-1, n)
	if o < 0 {
		o = max(0, o+n)
	}
	rest := clusters[o:]
	if less(length, zero) {
		return rest
	}
	return rest[:clamped(length, 0, int64(len(rest)))]
}

// substrNotKnown answers Substr where an argument is not known yet: at most
// length long, or as long as a negative offset counts back, or as the string
// is; and, where the string is not known yet but its offset and length are,
// what its recorded prefix settles. Every cluster of the prefix but its
// last is a cluster of the string, which what follows the prefix may
// extend.
func substrNotKnown(str, offset, length tenon.Value) tenon.Value {
	ns := []tenon.Narrowing{tenon.NotNull()}
	hi, bounded := int64(0), false
	most := func(n int64) {
		if !bounded || n < hi {
			hi, bounded = n, true
		}
	}
	if length.IsKnown() && !less(length, zero) {
		most(clamped(length, 0, 1<<62))
	}
	if offset.IsKnown() && less(offset, zero) {
		most(clamped(negated(offset), 0, 1<<62))
	}
	if str.IsKnown() {
		most(int64(uni.GraphemeCount(str.AsString())))
	}
	if !str.IsKnown() && offset.IsKnown() && !less(offset, zero) && length.IsKnown() {
		settled := slices.Collect(uni.Clusters(str.Range().StringPrefix()))
		if len(settled) > 0 {
			settled = settled[:len(settled)-1]
		}
		taken := substr(settled, offset, length)
		o := clamped(offset, 0, int64(len(settled)))
		if !less(length, zero) && !less(tenon.NumberFromInt(int64(len(settled))-o), length) {
			return tenon.String(strings.Join(taken, ""))
		}
		ns = append(ns, tenon.StringPrefix(strings.Join(taken, "")))
	}
	if bounded {
		ns = append(ns, tenon.LengthMax(hi))
	}
	return tenon.Narrow(tenon.Unknown(tenon.StringType()), ns...)
}

// stringList is the constraint of a parameter taking a list of strings.
var stringList = tenon.ListOf(text)

// nullElement returns the failure of element j of argument i, known to be
// null, which a function joining or ordering strings has no answer for.
func nullElement(fn string, i, j int) tenon.Value {
	return tenon.ErrorVal(tenon.Diagnostic{
		Code:    tenon.CodeOperationNullOperand,
		Message: fn + ": element " + strconv.Itoa(j) + " is null, and has no text to give",
		Path:    argument(i).Index(tenon.NumberFromInt(int64(j))),
	})
}

// firstNull returns the failure of the first element known to be null
// among the lists, argument first and those after it, and false where none
// is: it fails the call now, whatever else is not known yet (UN-011).
func firstNull(fn string, first int, lists []tenon.Value) (tenon.Value, bool) {
	for k, l := range lists {
		if !l.HasMembers() {
			continue
		}
		for j, e := range l.Elements() {
			if e.IsNull() {
				return nullElement(fn, first+k, j), true
			}
		}
	}
	return tenon.Value{}, false
}

// JoinFunc joins the strings of one list or more, in order, with a
// separator between each two, as one string: its value (LS-004), across
// whose joins normalization may compose. A null element fails with
// tenon.CodeOperationNullOperand at it, whatever else is not known yet.
// Not known yet, the answer begins with the text known from the start: the
// elements and separators up to the first part not known, and what that
// part's recorded prefix says, a separator before a list not known yet
// left out, since it may be empty.
var JoinFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Join",
	Description: "Concatenates the elements of the given lists of strings, with the separator between each two.",
	Params: []tenon.Param{
		stringParam("separator", "The separator."),
		{Name: "list", Description: "The first list.", Constraint: stringList, AllowUnknown: true},
	},
	VarParam: &tenon.Param{Name: "lists", Description: "The further lists.", Constraint: stringList, AllowUnknown: true},
	Result:   text,
	NotNull:  true,
	Impl: func(args []tenon.Value, _ tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		sep, lists := args[0], args[1:]
		if failure, ok := firstNull("Join", 1, lists); ok {
			return failure, nil
		}
		text, complete := joined(sep, lists)
		if complete {
			return tenon.String(text), nil
		}
		return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix(text)), nil
	},
})

// joined returns the text Join makes of the lists, and whether it is all of
// it: up to the first part not known, a list's elements, an element or the
// separator between two, with what that part's recorded prefix says, and
// no separator before a list not known yet, which may be empty.
func joined(sep tenon.Value, lists []tenon.Value) (string, bool) {
	var b strings.Builder
	first := true
	for _, l := range lists {
		if !l.HasMembers() {
			return b.String(), false
		}
		for _, e := range l.Elements() {
			if !first {
				if !sep.IsKnown() {
					b.WriteString(sep.Range().StringPrefix())
					return b.String(), false
				}
				b.WriteString(sep.AsString())
			}
			first = false
			if !e.IsKnown() {
				b.WriteString(e.Range().StringPrefix())
				return b.String(), false
			}
			b.WriteString(e.AsString())
		}
	}
	return b.String(), true
}

// SortFunc orders a list of strings ascending by their scalar values, as
// LessThan orders strings, duplicates kept: "B" before "a", "10" before "9".
// A null element fails with tenon.CodeOperationNullOperand at it, whatever
// else is not known yet. A list holding an element not known yet answers
// a list as long, each element a string not known yet, not null, since
// where each lands depends on the others; one of one element is itself.
// Not known yet, the answer is the unknown list of the argument's lengths.
var SortFunc = tenon.NewFunction(tenon.FunctionSpec{
	Name:        "Sort",
	Description: "Orders the given list of strings by their Unicode scalar values.",
	Params:      []tenon.Param{{Name: "list", Description: "The list.", Constraint: stringList, AllowUnknown: true}},
	Result:      tenon.Exactly(tenon.ListType(tenon.StringType())),
	NotNull:     true,
	Impl: func(args []tenon.Value, rc tenon.Constraint, _ tenon.Policy) (tenon.Value, error) {
		l := args[0]
		if failure, ok := firstNull("Sort", 0, args); ok {
			return failure, nil
		}
		if !l.HasMembers() {
			lo, hi, bounded := lengthOf(l)
			return unknownList(tenon.StringType(), lo, hi, bounded), nil
		}
		elems := l.Elements()
		if !l.IsKnown() {
			if len(elems) == 1 {
				return tenon.List(tenon.StringType(), tenon.Narrow(elems[0], tenon.NotNull())), nil
			}
			for i := range elems {
				elems[i] = tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull())
			}
			return tenon.List(tenon.StringType(), elems...), nil
		}
		slices.SortStableFunc(elems, func(a, b tenon.Value) int { return strings.Compare(a.AsString(), b.AsString()) })
		return tenon.List(tenon.StringType(), elems...), nil
	},
})
