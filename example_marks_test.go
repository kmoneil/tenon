package tenon_test

import (
	"fmt"

	"github.com/kmoneil/tenon"
)

// secret marks a value whose contents must not be shown.
type secret struct{}

func (secret) MarkID() string                 { return "secret" }
func (secret) Propagation() tenon.Propagation { return tenon.Propagate }
func (secret) Redacting() bool                { return true }

// A redacting mark keeps a value's contents out of everything that shows it,
// and travels into whatever is derived from the value.
func ExampleWithMarks() {
	login := tenon.Object(map[string]tenon.Value{
		"user":     tenon.String("ada"),
		"password": tenon.WithMarks(tenon.String("hunter2"), secret{}),
	})
	fmt.Println(login)

	// What is derived from the secret carries the mark, and shows nothing.
	fmt.Println(tenon.Length(login.Attribute("password")))

	// A diagnostic about it names it by its placeholder, never by its text.
	fmt.Println(tenon.Convert(login.Attribute("password"), tenon.Exactly(tenon.NumberType()), tenon.Unsafe))

	// The projection a log or a response would hold refuses it.
	_, err := tenon.ProjectJSON(login)
	fmt.Println(err)
	// Output:
	// {"password": redacted("secret"), "user": "ada"}
	// redacted("secret")
	// marked(error(number.invalid_syntax: "redacted(\"secret\") does not convert to exactly(number)"), "secret")
	// serialize.redacted: the value carries the redacting mark redacted("secret"), and is not projected at .password
}
