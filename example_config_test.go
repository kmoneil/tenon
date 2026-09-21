package tenon_test

import (
	"fmt"

	"github.com/kmoneil/tenon"
)

// scope is what a configuration language evaluates against: the variables in
// hand, some of which are not known until the plan is applied.
type scope map[string]tenon.Value

// lookup returns the value of a variable. A variable that is not there is an
// error value, which is a value like any other: the expression it appears in
// carries it, and the language reports it where it reports the rest.
func (s scope) lookup(name string) tenon.Value {
	if v, ok := s[name]; ok {
		return v
	}
	return tenon.ErrorVal(tenon.Diagnostic{
		Code:    "config.undefined_variable",
		Message: "no variable named " + quoted(name),
	})
}

// quoted writes a name as a configuration language would show it.
func quoted(name string) string { return `"` + name + `"` }

// The value layer of a configuration language: expressions answer what they
// can, defer what is not known yet, and locate what is wrong.
func Example_configLanguage() {
	vars := scope{
		"base":  tenon.NumberFromInt(3),
		"scale": tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull(), tenon.NumberMin(tenon.NumberFromInt(2), true)),
	}

	// An expression over what is in hand is answered exactly.
	fmt.Println(tenon.Mul(vars.lookup("base"), tenon.NumberFromInt(2)))

	// One over a variable that is not known yet is answered as far as it can
	// be, and carries what is known about the rest.
	fmt.Println(tenon.Mul(vars.lookup("scale"), vars.lookup("base")))

	// A failure travels through the expression rather than stopping it.
	broken := tenon.Add(vars.lookup("missing"), tenon.NumberFromInt(1))
	fmt.Println(broken.Diagnostics()[0].Code, broken.Diagnostics()[0].Message)

	// The branches of a conditional need one type between them. The language
	// unifies what each branch says and converts both to it, so the answer
	// has a type whichever branch is taken.
	yes, no := tenon.NumberFromInt(8080), tenon.String("auto")
	common, _, ok := tenon.Unify(tenon.Unsafe, tenon.Exactly(yes.Type()), tenon.Exactly(no.Type()))
	fmt.Println(common, ok)
	fmt.Println(tenon.Convert(yes, common, tenon.Unsafe), tenon.Convert(no, common, tenon.Unsafe))

	// What the file says is checked against what the schema asks for, and
	// each failure is located where a reader would look for it.
	file := tenon.ObjectVal(map[string]tenon.Value{
		"name":  tenon.String("web"),
		"ports": tenon.TupleVal(tenon.String("80"), tenon.String("https")),
	})
	schema := tenon.ObjectWith(map[string]tenon.Field{
		"name":  tenon.Required(tenon.Exactly(tenon.StringType())),
		"ports": tenon.Required(tenon.ListOf(tenon.Exactly(tenon.NumberType()))),
	}, true)
	for _, d := range tenon.Convert(file, schema, tenon.Unsafe).Diagnostics() {
		fmt.Printf("%s at %s: %s\n", d.Code, d.Path, d.Message)
	}
	// Output:
	// 6
	// unknown(number, not null, >= 6)
	// config.undefined_variable no variable named "missing"
	// exactly(string) true
	// "8080" "auto"
	// number.invalid_syntax at .ports[1]: "https" is not a number
}
