package stdlib

import (
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
