package gotenon_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/gotenon"
)

// service is a Go type the program already has. Its fields are named for
// tenon by their tags, and the port may be absent.
type service struct {
	Name    string      `tenon:"name"`
	Port    int         `tenon:"port,optional"`
	Tags    []string    `tenon:"tags"`
	Address tenon.Value `tenon:"address"`
}

// Encode makes a value from a Go value, so a program with Go types already
// can hand them to anything that speaks tenon.
func ExampleEncode() {
	value, err := gotenon.Encode(service{
		Name: "web",
		Port: 8080,
		Tags: []string{"edge", "public"},
		// A field of type tenon.Value carries what Go has no type for: this
		// address is not known yet, though its network is settled.
		Address: tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix("10.")),
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(value)
	// Output:
	// {"address": unknown(string, not null, prefix "10.", length >= 3), "name": "web", "port": 8080, "tags": list(string)["edge", "public"]}
}

// Numbers encode exactly. A float64 is the terminating decimal it holds, not
// a rounded rendering of it, so nothing is invented on the way in.
func ExampleEncode_numbers() {
	tenth, _ := gotenon.Encode(0.1)
	fmt.Println(tenth)

	// To carry the number that was meant rather than the float64 nearest to
	// it, encode text or a rational.
	exact, _ := gotenon.Encode(big.NewRat(1, 10))
	fmt.Println(exact)
	fmt.Println(tenon.Equals(tenth, exact))
	// Output:
	// 0.1000000000000000055511151231257827021181583404541015625
	// 0.1
	// false
}

// Decode fills a Go value from a tenon value, converting it under the policy
// the caller chooses.
func ExampleDecode() {
	value := tenon.Object(map[string]tenon.Value{
		"name":    tenon.String("web"),
		"tags":    tenon.List(tenon.StringType(), tenon.String("edge")),
		"address": tenon.String("10.0.0.7"),
	})
	got, err := gotenon.Decode[service](value, tenon.Safe)
	if err != nil {
		fmt.Println(err)
		return
	}
	// port was absent, and an optional field takes its zero value.
	fmt.Printf("%s %d %v %v\n", got.Name, got.Port, got.Tags, got.Address)
	// Output:
	// web 0 [edge] "10.0.0.7"
}

// What cannot be decoded comes back as diagnostics, each located by its path,
// rather than as a zero value or a panic.
func ExampleDecode_diagnostics() {
	value := tenon.Object(map[string]tenon.Value{
		"name":    tenon.String("web"),
		"port":    tenon.String("http"),
		"tags":    tenon.List(tenon.StringType()),
		"address": tenon.Unknown(tenon.StringType()),
	})
	_, err := gotenon.Decode[service](value, tenon.Unsafe)
	var failed *tenon.Error
	if !errors.As(err, &failed) {
		fmt.Println("decoded:", err)
		return
	}
	for _, d := range failed.Diagnostics() {
		fmt.Printf("%s at %s: %s\n", d.Code, d.Path, d.Message)
	}
	// Output:
	// number.invalid_syntax at .port: "http" is not a number
}

// Data whose types are not known at compile time reaches a Go program as any.
// Encode takes it by what each value holds, so a JSON document becomes a value
// without a Go type written for it.
func Example_json() {
	document := `{"name": "web", "port": 8080, "ratio": 0.1, "tags": ["edge", 2], "on": true}`

	// UseNumber, so that the document's numbers arrive as the numbers it
	// wrote rather than as the float64 nearest to them.
	decoder := json.NewDecoder(strings.NewReader(document))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		fmt.Println(err)
		return
	}
	value, err := gotenon.Encode(fields)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(value)

	// The number in the document is the number in the value, to the last
	// digit, where reading it as a float64 would have given the binary value
	// nearest to it.
	fmt.Println(tenon.Equals(value.Attribute("ratio"), tenon.NumberFromText("0.1")))

	// A null says null without saying null of what, and a null has a type, so
	// which null it is is the schema's to say and encoding says where it is.
	_, err = gotenon.Encode(map[string]any{"name": "web", "port": nil})
	fmt.Println(err)
	// Output:
	// {"name": "web", "on": true, "port": 8080, "ratio": 0.1, "tags": ["edge", 2]}
	// true
	// encode.untyped_nil: a nil interface {} holds no value, and no type follows from it at .port
}

// Service is what a program expects a service's configuration to be.
type Service struct {
	Name     string   `tenon:"name"`
	Port     int      `tenon:"port"`
	Replicas int      `tenon:"replicas,optional"`
	Tags     []string `tenon:"tags,optional"`
}

// A document a program reads, checked against the Go type it expects and
// decoded into it, with every part that does not fit reported where it is.
func Example_quickStart() {
	// Read the document as encoding/json does, keeping its numbers as
	// written rather than as the nearest float64.
	decoder := json.NewDecoder(strings.NewReader(`{"name": "web", "port": 8080, "tags": ["edge"]}`))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		fmt.Println(err)
		return
	}

	// Take it in as a value, whatever it holds, then decode the value into
	// the Go type, converting it under the policy given.
	value, err := gotenon.Encode(document)
	if err != nil {
		fmt.Println(err)
		return
	}
	service, err := gotenon.Decode[Service](value, tenon.Safe)
	fmt.Printf("%+v %v\n", service, err)

	// A document that does not fit says where, for each part.
	wrong, _ := gotenon.Encode(map[string]any{"name": "web", "port": "http", "colour": "blue"})
	_, err = gotenon.Decode[Service](wrong, tenon.Safe)
	fmt.Println(err)
	// Output:
	// {Name:web Port:8080 Replicas:0 Tags:[edge]} <nil>
	// convert.unexpected_attribute: attribute "colour" is not one the constraint allows at .colour; convert.unsafe: string converts to exactly(number) only unsafely, and the policy is safe at .port
}
