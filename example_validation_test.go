package tenon_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/gotenon"
)

// password marks a value whose contents must not be shown. A redacting mark
// keeps them out of display forms, diagnostic messages and JSON projections,
// and travels into whatever is derived from the value.
type password struct{}

func (password) MarkID() string                 { return "password" }
func (password) Propagation() tenon.Propagation { return tenon.Propagate }
func (password) Redacting() bool                { return true }

// accepted is what this endpoint accepts. The object is closed, so an
// attribute it does not name is a failure rather than something carried on
// silently, and the password is optional.
var accepted = tenon.ObjectWith(map[string]tenon.Field{
	"name":     tenon.Required(tenon.Exactly(tenon.StringType())),
	"port":     tenon.Required(tenon.Exactly(tenon.NumberType())),
	"password": tenon.Optional(tenon.Exactly(tenon.StringType())),
}, true)

// A service checking untrusted input: every failure is reported with a stable
// code and the path to it, and a secret cannot be logged by accident.
func Example_validation() {
	for _, body := range []string{
		`{"name": "web", "port": 8080, "password": "hunter2"}`,
		`{"name": "web", "port": "8080"}`,
		`{"name": "web", "port": 8080, "colour": "blue"}`,
		`{"name": "web", "port": null}`,
	} {
		// UseNumber, so that the document's numbers arrive as the numbers it
		// wrote rather than as the float64 nearest to them.
		decoder := json.NewDecoder(strings.NewReader(body))
		decoder.UseNumber()
		var fields map[string]any
		if err := decoder.Decode(&fields); err != nil {
			fmt.Println("400", err)
			continue
		}

		// The password is a secret from the moment it is read: a tenon.Value
		// among the fields carries its marks through encoding.
		if text, ok := fields["password"].(string); ok {
			fields["password"] = tenon.WithMarks(tenon.String(text), password{})
		}

		// map[string]any encodes by what each value holds, so the document's
		// own types survive: 8080 is a number and "8080" is text.
		request, err := gotenon.Encode(fields)
		if err != nil {
			var failed *gotenon.DiagnosticError
			errors.As(err, &failed)
			for _, d := range failed.Diagnostics() {
				fmt.Printf("400 %s at %s: %s\n", d.Code, d.Path, d.Message)
			}
			continue
		}

		// Safe, because this endpoint takes the types the document wrote and
		// does not read a port out of text.
		checked := tenon.Convert(request, accepted, tenon.Safe)
		if checked.IsError() {
			for _, d := range checked.Diagnostics() {
				fmt.Printf("400 %s at %s: %s\n", d.Code, d.Path, d.Message)
			}
			continue
		}
		fmt.Println("200", checked)

		// What comes back out is what a log or a response may hold. The
		// projection refuses the secret rather than printing it.
		if _, failure, ok := tenon.ProjectJSON(checked); !ok {
			fmt.Println("   not loggable:", failure.Diagnostics()[0].Code, "at", failure.Diagnostics()[0].Path)
		}
		public, _, _ := tenon.ProjectJSON(tenon.ObjectVal(map[string]tenon.Value{
			"name": checked.Attribute("name"),
			"port": checked.Attribute("port"),
		}))
		fmt.Println("  ", string(public))
	}
	// Output:
	// 200 {"name": "web", "password": redacted("password"), "port": 8080}
	//    not loggable: serialize.redacted at .password
	//    {"name":"web","port":8080}
	// 400 convert.unsafe at .port: string converts to exactly(number) only unsafely, and the policy is safe
	// 400 convert.unexpected_attribute at .colour: attribute "colour" is not one the constraint allows
	// 400 encode.untyped_nil at .port: a nil interface {} holds no value, and no type follows from it
}
