# tenon

[![Go Reference](https://pkg.go.dev/badge/github.com/kmoneil/tenon.svg)](https://pkg.go.dev/github.com/kmoneil/tenon)
[![check](https://github.com/kmoneil/tenon/actions/workflows/check.yml/badge.svg)](https://github.com/kmoneil/tenon/actions/workflows/check.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/kmoneil/tenon/badge)](https://scorecard.dev/viewer/?uri=github.com/kmoneil/tenon)

tenon is a Go library for values whose types a program does not know at compile
time: the value layer of a configuration language, and of anything else that
carries data a person supplied.

A program holding data it did not declare has three problems that Go's own
types do not answer. Part of the data is not known yet. Part of it must not be
shown. Part of it is wrong. A tenon value answers each of them in the value
itself, so the program does not have to carry a second set of bookkeeping
beside its data.

```go
// A service's configuration: the name is in hand, the address it will be
// reachable at is not known until the service is created.
config := tenon.Object(map[string]tenon.Value{
	"name":     tenon.String("web"),
	"replicas": tenon.NumberFromInt(3),
	"address":  tenon.Unknown(tenon.StringType()),
})
fmt.Println(config)

// Known attributes are read as Go values.
fmt.Println(config.Attribute("name").AsString())

// One that is not known says so rather than giving a zero value, and an
// operation over it gives a value that is not known either.
address := config.Attribute("address")
fmt.Println(address.IsKnown(), tenon.Length(address))
// Output:
// {"address": unknown(string), "name": "web", "replicas": 3}
// web
// false unknown(number, not null, >= 0)
```

    go get github.com/kmoneil/tenon@v0.14.0

Version 0.14.0 implements version 0.12.0 of the tenon specification. A
conformance test covers every one of its 227 rules, as `CONFORMANCE.md`
reports; `CHANGELOG.md` says what each release holds, and, where there are
any, which rules the report states more widely than its test exercises.

Every example below is a program in the test suite, run by `make check`, so
nothing here is code that has never been compiled. They are on pkg.go.dev
under [Examples](https://pkg.go.dev/github.com/kmoneil/tenon#pkg-examples).

## What is not known yet

An unknown value is a promise about a value that is not in hand. What the
program does know about it is recorded as it learns it, and operations answer
what that settles.

```go
port := tenon.Unknown(tenon.NumberType())
fmt.Println(port)

// What the program does know is recorded as it learns it.
port = tenon.Narrow(port, tenon.NotNull(), tenon.NumberMin(tenon.NumberFromInt(1024), true))
fmt.Println(port)

// Operations read what is recorded and answer what it settles: no port
// of 1024 or more is port 80, or comes before it, whatever else it turns
// out to be.
fmt.Println(tenon.Equals(port, tenon.NumberFromInt(80)))
fmt.Println(tenon.LessThan(port, tenon.NumberFromInt(80)))
fmt.Println(tenon.Mul(port, tenon.NumberFromInt(2)))
// Output:
// unknown(number)
// unknown(number, not null, >= 1024)
// false
// false
// unknown(number, not null, >= 2048)
```

Narrowing is monotone: what a value has said, it goes on saying. A narrowing
that leaves one value gives that value, known; one that leaves nothing is a
contradiction, and gives an error value rather than a value that cannot exist.
Bounds, prefixes and lengths say nothing about null, so where a value may still
be null, narrowings that leave it no other value leave it null.
See `ExampleUnknown` and `ExampleNarrow`.

## What must not be shown

A mark is a label that travels with a value. A redacting mark keeps the value's
contents, its keys and attribute names among them, out of display forms, the Go
syntax `%#v` prints, diagnostics and JSON projections, and follows into
whatever is derived from it, so a secret cannot reach a log by a route nobody
thought about. A mark is any comparable Go type that says how it behaves:

```go
// secret marks a value whose contents must not be shown.
type secret struct{}

func (secret) MarkID() string                 { return "secret" }
func (secret) Propagation() tenon.Propagation { return tenon.Propagate }
func (secret) Redacting() bool                { return true }
```

```go
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
```

See `Example_validation` for a service that takes a secret from a request,
and `Example_planAndApply` for a diff that reports a secret changing without
showing either secret.

## What is wrong

Data that is wrong becomes an error value carrying a diagnostic for each
problem: a stable code for programs, a message for people, and the path to the
part that failed. An operation over an error value gives an error value, so a
failure travels to where it is handled instead of being checked at every step.

```go
ports := tenon.List(tenon.StringType(), tenon.String("80"), tenon.String("http"), tenon.String("443"))
converted := tenon.Convert(ports, tenon.ListOf(tenon.Exactly(tenon.NumberType())), tenon.Unsafe)
for _, d := range converted.Diagnostics() {
	fmt.Printf("%s at %s: %s\n", d.Code, d.Path, d.Message)
}
// Output:
// number.invalid_syntax at .[1]: "http" is not a number
```

Passing a value of the wrong type to an operation is not a diagnostic but a
panic: that is a mistake in the program, not in the data.

## One value, one encoding

However a value was built, two values that say the same thing are one value:
they are `Identical`, they hash alike, and they serialize to the same bytes.
Numbers are exact and never rounded, strings are compared in Normalization Form
C, and a set holds each value once by what its members are rather than by how
they were written. `Serialize` writes a CBOR document that carries unknown
values, their bounds, marks and diagnostics, and `Deserialize` reads it back
into a value identical to the one that was sent.

## Go values

Package `gotenon` maps Go values to tenon values and back. A program reads a
JSON document with `ParseJSON`, in one pass and strictly, by what each part
holds or into the constraint it gives, and one that has Go types for the data
decodes into them, converting under the policy it chooses and failing with a
diagnostic for each part that does not fit.

```go
// Service is what a program expects a service's configuration to be.
type Service struct {
	Name     string   `tenon:"name"`
	Port     int      `tenon:"port"`
	Replicas int      `tenon:"replicas,optional"`
	Tags     []string `tenon:"tags,optional"`
}
```

```go
// Read the document as a value, whatever it holds, its numbers exactly
// as written, then decode the value into the Go type, converting it
// under the policy given.
value, err := tenon.ParseJSON([]byte(`{"name": "web", "port": 8080, "tags": ["edge"]}`), tenon.Any(), tenon.Safe)
if err != nil {
	fmt.Println(err)
	return
}
service, err := gotenon.Decode[Service](value, tenon.Safe)
fmt.Printf("%+v %v\n", service, err)

// A document that does not fit says where, for each part.
wrong, _ := tenon.ParseJSON([]byte(`{"name": "web", "port": "http", "colour": "blue"}`), tenon.Any(), tenon.Safe)
_, err = gotenon.Decode[Service](wrong, tenon.Safe)
fmt.Println(err)
// Output:
// {Name:web Port:8080 Replicas:0 Tags:[edge]} <nil>
// convert.unexpected_attribute: attribute "colour" is not one the constraint allows at .colour; convert.unsafe: string converts to exactly(number) only unsafely, and the policy is safe at .port
```

Fields are named by their `tenon` tag, and decoding into a struct is closed
and exact: an attribute that no field names fails, and a name matches only as
it is written, case included. A type that marshals itself to text, as
`time.Time` does, crosses as its text. The
[gotenon documentation](https://pkg.go.dev/github.com/kmoneil/tenon/gotenon)
has the whole mapping.

## What it is for

| What it does | The example that builds it |
| --- | --- |
| **Plan and diff engines** carry values that are not known yet through a plan, each with a range of what it may turn out to be, and show what changes without showing what is secret. | `Example_planAndApply` |
| **Plugin protocols** pass values, unknown and marked ones included, across a process boundary in one canonical encoding, and tell a receiver about a mark it does not know rather than dropping it. | `Example_pluginProtocol` |
| **Validation layers** check values against constraints and report every failure with a stable code and the path to it. | `Example_validation` |
| **Configuration languages** evaluate expressions over values some of which are not settled, unify the branches of a conditional, and locate what is wrong in the file. | `Example_configLanguage` |
| **Go programs with types already** encode their structs and decode them back, keeping in a `tenon.Value` field whatever Go has no type for. | gotenon's `Example_quickStart` |
| **Go programs without them** take data whose types they do not know, the `map[string]any` that `encoding/json` gives, by what each value holds, with the document's own numbers kept exactly. | gotenon's `Example_json` |

## Coming from go-cty

tenon answers the problems that [go-cty](https://github.com/zclconf/go-cty)
answers for HCL and Terraform, and differs where cty's answers leave a
program exposed. Its API is its own, but a program need not move across all
at once: [ctytenon](ctytenon), a module of its own in this repository,
carries values, types, type constraints, paths and errors between the two,
so that a program built on cty can hand what it has to tenon and take it
back, one piece at a time. What is not known yet, and what is sensitive,
crosses with it:

```go
config := cty.ObjectVal(map[string]cty.Value{
	"name":     cty.StringVal("web"),
	"replicas": cty.UnknownVal(cty.Number).Refine().NotNull().NumberRangeLowerBound(cty.NumberIntVal(1), true).NewValue(),
	"token":    cty.StringVal("s3cr3t").Mark("sensitive"),
})

v, err := b.FromCty(config)
```

```
{"name": "web", "replicas": unknown(number, not null, >= 1), "token": redacted("sensitive")}
```

The module's documentation says what crosses, what does not, and where the
two differ, and its own `CHANGELOG.md` what each release holds:

    go get github.com/kmoneil/tenon/ctytenon@v0.1.0

| | go-cty v1.19 | tenon |
| --- | --- | --- |
| Numbers | 512-bit binary floats: a 150-digit integer comes back with its last digits changed, and `1/0` is infinity | Exact decimals, never rounded; dividing by zero is an error value |
| Number text | `Inf`, `+5` and `1p4` parse as numbers | Each is refused, with a diagnostic |
| Strings | Text that is not UTF-8 passes through | An error value, where the string is made |
| Secrets | A mark is any Go value, and nothing withholds what it marks: a marked value's Go syntax shows it | A redacting mark keeps the contents and structure of what it marks out of display forms, Go syntax, messages and projections, and follows whatever is derived from it |
| Unknown values | Refinements: not null, a string prefix, number bounds, collection lengths | Ranges: the same facts, and the members a set is known to hold |
| Types | One `Type` serves as a type and as a constraint, `DynamicPseudoType` standing for any | Types and constraints are distinct, and a value whose type is not settled yet carries a constraint in its place |
| Diffs | None: each program writes its own | `Diff`, which never looks inside what a redacting mark withholds |
| Determinism | `Equals` on objects and maps holding an unknown answers by Go's map order, and keys that are one after normalization merge at random | The same answer every time, and such keys are refused, naming both spellings |
| Reading JSON | `ctyjson.Unmarshal` keeps the last of two members of one name, ignores text after the value, reads numbers as 512-bit floats and stops at the first failure | `ParseJSON` refuses a name given twice and anything after the value, reads numbers exactly, and reports every failure at its path |

The bench module holds a test for each of go-cty's open issues whose defect
tenon could share, asserting what go-cty v1.19.0 does with the issue's case
and what tenon does with its counterpart.

## Performance

tenon does more for each value than a Go map does: it parses every number
into an exact decimal, normalizes every string, and records what is known
about each value. Beside go-cty, the value system it answers, it is faster at
everything measured here, at every size, and uses less memory at all of it
but converting to a schema, where the two are about even.
[`BENCHMARKS.md`](BENCHMARKS.md) has every size, the memory each operation
takes, and what each library does per value.

<!-- benchmarks:begin -->
For a configuration of 32 KB, measured on Apple M5 Max with go1.26.4:

| | encoding/json | tenon | go-cty |
| --- | --- | --- | --- |
| Parse JSON into a value | 270 µs | 553 µs | 5.10 ms |
| Convert to a schema | – | 602 µs | 1.97 ms |
| Encode and decode | 473 µs | 430 µs | 3.52 ms |
| Compare two copies | 487 µs | 10.5 µs | 8.22 ms |
| Read a nested value | 16.0 ns | 58.6 ns | 94.0 ns |
| Diff one change | – | 194 µs | – |
<!-- benchmarks:end -->

## Stability

tenon is before 1.0. Version 0.12.0 holds the API that 1.0 is to keep, and
1.0 will freeze the API, the encoding and the specification: after it, a
change that breaks a program waits for a new major version. Until then a
minor version may break a program, and its release notes say how to upgrade.
tenon needs Go 1.26 or later; CI tests it on Go 1.26 and 1.27.

## Immutability and concurrent use

Values, types, constraints, paths and diagnostics are immutable. Every
operation returns a new value, and nothing a caller holds is written to again,
so they may be read from any number of goroutines at once without
synchronization. The operations a caller supplies, which are the operations of
a capsule type, the methods of a mark and the decoders given to `Deserialize`,
may be called from any goroutine that uses the value carrying them.

## Unicode

tenon holds strings in Normalization Form C and measures their length in
grapheme clusters, both under Unicode 15.0.0, and its display form escapes text
by the same version. Which Unicode version is in use decides which strings are
equal and how long they are, so changing it is a breaking change.

The version is held inside the module, and follows neither the Go toolchain you
build with nor any module your build requires: tenon requires no other module.
The normalization, general-category and grapheme-segmentation data live in
`internal/uni`, generated by `tools/unigen` and committed. Two builds of one
version of tenon therefore agree on every string, and on every encoding of
one, whatever toolchain made them and whatever else they require. On every
toolchain, tests hold that data to Unicode's own conformance tests for the
version, `NormalizationTest.txt` and `GraphemeBreakTest.txt`, and to fixtures
answered by Python's `unicodedata`; where the toolchain still carries the same
Unicode version, they hold it to `golang.org/x/text` and to Go's own
`unicode` as well, code point by code point.

Normalization is plain UAX #15. tenon does not apply the Stream-Safe Text
Process, which inserts U+034F into a run of more than thirty non-starters: a
string that gained a code point on the way in would not be the string that was
given.

To regenerate the data for another Unicode version, put that version's files in
`tools/unigen/ucd`, run `tools/unigen` on a toolchain that carries it, change
`UnicodeVersion`, and treat it as the breaking change it is.

## Why this exists

When I need something that I need and don't want to modify an existing library, I usually write it myself. If you find it
useful, awesome. If you find where it might be lacking, let me know. Find a bug, post an issue.

## Development

`make check` is the gate: a change is not done until it passes.
`CONTRIBUTING.md` says how to propose a change, and what each of the other
`make` targets does.

## License

tenon is licensed under the Apache License, Version 2.0. See `LICENSE`.
