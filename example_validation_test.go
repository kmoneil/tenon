package tenon_test

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kmoneil/tenon"
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

// fromJSON turns what encoding/json gives into a tenon value. Numbers arrive
// as text and stay exact: no binary float stands between the request and the
// value, so a port or an amount is the number that was sent.
//
// A JSON null says null without saying null of what, and a null has a type.
// Which type is the schema's to say, so this endpoint refuses one: the error
// value it gives is carried up by whatever holds it, gathering the path on
// the way, exactly as a failure to convert would be.
func fromJSON(v any) tenon.Value {
	switch v := v.(type) {
	case nil:
		return tenon.ErrorVal(tenon.Diagnostic{
			Code:    "request.untyped_null",
			Message: "null does not say null of what; give a value or leave the field out",
		})
	case bool:
		return tenon.Bool(v)
	case json.Number:
		return tenon.NumberFromText(v.String())
	case string:
		return tenon.String(v)
	case []any:
		elems := make([]tenon.Value, len(v))
		for i, e := range v {
			elems[i] = fromJSON(e)
		}
		return tenon.TupleVal(elems...)
	}
	attrs := map[string]tenon.Value{}
	for name, e := range v.(map[string]any) {
		attrs[name] = fromJSON(e)
	}
	return tenon.ObjectVal(attrs)
}

// A service checking untrusted input: every failure is reported with a stable
// code and the path to it, and a secret cannot be logged by accident.
func Example_validation() {
	for _, body := range []string{
		`{"name": "web", "port": "8080", "password": "hunter2"}`,
		`{"name": "web", "port": "http", "colour": "blue"}`,
		`{"name": "web", "port": null}`,
	} {
		decoder := json.NewDecoder(strings.NewReader(body))
		decoder.UseNumber()
		var fields map[string]any
		if err := decoder.Decode(&fields); err != nil {
			fmt.Println(err)
			continue
		}
		request := fromJSON(fields)

		// The password is a secret from the moment it is read, so nothing
		// derived from it can show it either.
		if held, ok := fields["password"]; ok && held != nil {
			attrs := map[string]tenon.Value{}
			for _, name := range request.Type().AttributeNames() {
				attrs[name] = request.Attribute(name)
			}
			attrs["password"] = tenon.WithMarks(attrs["password"], password{})
			request = tenon.ObjectVal(attrs)
		}

		// Unsafe, because this endpoint takes a port written as text.
		checked := tenon.Convert(request, accepted, tenon.Unsafe)
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
	// 400 convert.unexpected_attribute at .colour: attribute "colour" is not one the constraint allows
	// 400 number.invalid_syntax at .port: "http" is not a number
	// 400 request.untyped_null at .port: null does not say null of what; give a value or leave the field out
}
