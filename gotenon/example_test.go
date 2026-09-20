package gotenon_test

import (
	"errors"
	"fmt"
	"math/big"

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
	value := tenon.ObjectVal(map[string]tenon.Value{
		"name":    tenon.String("web"),
		"tags":    tenon.ListVal(tenon.StringType(), tenon.String("edge")),
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
	value := tenon.ObjectVal(map[string]tenon.Value{
		"name":    tenon.String("web"),
		"port":    tenon.String("http"),
		"tags":    tenon.ListVal(tenon.StringType()),
		"address": tenon.Unknown(tenon.StringType()),
	})
	_, err := gotenon.Decode[service](value, tenon.Unsafe)
	var failed *gotenon.DiagnosticError
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
