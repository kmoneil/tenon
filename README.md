# tenon

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
config := tenon.ObjectVal(map[string]tenon.Value{
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

    go get github.com/kmoneil/tenon@v0.3.0

Version 0.3.0 implements version 0.2.0 of the tenon specification. A
conformance test covers every one of its 194 rules, as `CONFORMANCE.md`
reports; `CHANGELOG.md` says what each release holds, and which rules the
report still states more widely than its test exercises.

Every example below is a program in the test suite, run by `make check`, so
nothing here is code that has never been compiled. They are on pkg.go.dev
under [Examples](https://pkg.go.dev/github.com/kmoneil/tenon#pkg-examples).

# What is not known yet

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
See `ExampleUnknown` and `ExampleNarrow`.

# What must not be shown

A mark is a label that travels with a value. A redacting mark keeps the value's
contents out of display forms, diagnostic messages and JSON projections, and
follows into whatever is derived from it, so a secret cannot reach a log by a
route nobody thought about.

```go
fmt.Println("200", checked)

// What comes back out is what a log or a response may hold. The
// projection refuses the secret rather than printing it.
if _, failure, ok := tenon.ProjectJSON(checked); !ok {
	fmt.Println("   not loggable:", failure.Diagnostics()[0].Code, "at", failure.Diagnostics()[0].Path)
}
```

```
200 {"name": "web", "password": redacted("password"), "port": 8080}
   not loggable: serialize.redacted at .password
```

See `Example_validation` for the whole program, and `Example_planAndApply` for
a diff that reports a secret changing without showing either secret.

# What is wrong

Data that is wrong becomes an error value carrying a diagnostic for each
problem: a stable code for programs, a message for people, and the path to the
part that failed. An operation over an error value gives an error value, so a
failure travels to where it is handled instead of being checked at every step.

```go
ports := tenon.ListVal(tenon.StringType(), tenon.String("80"), tenon.String("http"), tenon.String("443"))
converted := tenon.Convert(ports, tenon.ListOf(tenon.Exactly(tenon.NumberType())), tenon.Unsafe)
for _, d := range converted.Diagnostics() {
	fmt.Printf("%s at %s: %s\n", d.Code, d.Path, d.Message)
}
// Output:
// number.invalid_syntax at .[1]: "http" is not a number
```

Passing a value of the wrong type to an operation is not a diagnostic but a
panic: that is a mistake in the program, not in the data.

# One value, one encoding

However a value was built, two values that say the same thing are one value:
they are `Identical`, they hash alike, and they serialize to the same bytes.
Numbers are exact and never rounded, strings are compared in Normalization Form
C, and a set holds each value once by what its members are rather than by how
they were written. `Serialize` writes a CBOR document that carries unknown
values, their bounds, marks and diagnostics, and `Deserialize` reads it back
into a value identical to the one that was sent.

# What it is for

| What it does | The example that builds it |
| --- | --- |
| **Plan and diff engines** carry values that are not known yet through a plan, each with a range of what it may turn out to be, and show what changes without showing what is secret. | `Example_planAndApply` |
| **Plugin protocols** pass values, unknown and marked ones included, across a process boundary in one canonical encoding, and tell a receiver about a mark it does not know rather than dropping it. | `Example_pluginProtocol` |
| **Validation layers** check values against constraints and report every failure with a stable code and the path to it. | `Example_validation` |
| **Configuration languages** evaluate expressions over values some of which are not settled, unify the branches of a conditional, and locate what is wrong in the file. | `Example_configLanguage` |
| **Go programs with types already** encode their structs and decode them back, keeping in a `tenon.Value` field whatever Go has no type for. | `gotenon` |
| **Go programs without them** take data whose types they do not know, the `map[string]any` that `encoding/json` gives, by what each value holds, with the document's own numbers kept exactly. | `gotenon` |

# Immutability and concurrent use

Values, types, constraints, paths and diagnostics are immutable. Every
operation returns a new value, and nothing a caller holds is written to again,
so they may be read from any number of goroutines at once without
synchronization. The operations a caller supplies, which are the operations of
a capsule type, the methods of a mark and the decoders given to `Deserialize`,
may be called from any goroutine that uses the value carrying them.

# Unicode

tenon holds strings in Normalization Form C and measures their length in
grapheme clusters, both under Unicode 15.0.0, and its display form escapes text
by the same version. Which Unicode version is in use decides which strings are
equal and how long they are, so changing it is a breaking change.

The version is held inside the module and does not follow the Go toolchain you
build with. The normalization and general-category data live in
`internal/uni`, generated by `tools/unigen` and committed; grapheme
segmentation comes from a pinned module version. Two builds of one version of
tenon therefore agree on every string, and on every encoding of one, whatever
toolchain made them. Where the toolchain still carries the same Unicode
version, tests hold that data to `golang.org/x/text` and to Go's own
`unicode`, code point by code point; everywhere, fixtures answered by an
oracle outside both hold it to the version this module states.

Normalization is plain UAX #15. tenon does not apply the Stream-Safe Text
Process, which inserts U+034F into a run of more than thirty non-starters: a
string that gained a code point on the way in would not be the string that was
given.

To regenerate the data for another Unicode version, run `tools/unigen` on a
toolchain that carries it, change `UnicodeVersion`, and treat it as the
breaking change it is.

# Why this exists

When I need something that I need and don't want to modify an existing library, I usually write it myself. If you find
useful, awesome. If you find where it might be lacking, let me know. Find a bug, post an issue.

# Development

`make check` is the gate: a change is not done until it passes. It checks
formatting, runs `go vet` and the tests under the race detector, and then
checks the conformance suite against the specification's rules: every rule has
a passing conformance test, the diagnostic codes agree with the specification,
and `CONFORMANCE.md` is current.

The other targets:

| Target | What it does |
| ------ | ------------ |
| `make check-slow` | `make check`, then every property test at twenty times its cases (`TENON_SLOW=20`), then `make determinism`. Run it before a release, and after changing how values are stored, ordered or encoded. |
| `make determinism` | Runs the tests twice, in shuffled orders and on different numbers of processors, writing the canonical output they emit (encodings, display forms, diffs, conversions) to `.emit/`, and fails unless both runs wrote the same bytes. |
| `make fuzz` | Runs each fuzz target (the number parser, string construction, and decoding) for `FUZZTIME`, 30 minutes by default. An input that fails is saved under the package's `testdata/fuzz`, where it runs with the tests from then on. |
| `make report` | Runs the tests, recording the rules they cover, and regenerates `CONFORMANCE.md`. |
| `make rules`, `make codes` | Regenerate `conformance/rules.json` and the specification's appendix of diagnostic codes from the specification that `TENON_SPEC` names. |

# License

tenon is licensed under the Apache License, Version 2.0. See `LICENSE`.
