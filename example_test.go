package tenon_test

import (
	"errors"
	"fmt"
	"hash/maphash"
	"net/netip"

	"github.com/kmoneil/tenon"
)

// A value holds what a program was given, whatever shape that turned out to
// be, and says honestly when part of it is not known yet.
func Example() {
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
}

// Unknown is a value of a known type whose content is not settled yet. What
// is known about it is its range, which narrowing adds to.
func ExampleUnknown() {
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
}

// Narrow records what has been learned about a value that is not known. It is
// monotone: the result says everything the value said and everything the
// narrowing says.
func ExampleNarrow() {
	port := tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull())
	only := func(i int64) []tenon.Narrowing {
		return []tenon.Narrowing{tenon.NumberMin(tenon.NumberFromInt(i), true), tenon.NumberMax(tenon.NumberFromInt(i), true)}
	}

	// A narrowing that leaves one value gives that value, known: an unknown
	// that nothing more could ever say is not unknown.
	settled := tenon.Narrow(port, only(443)...)
	fmt.Println(settled, settled.IsKnown())

	// One that leaves nothing is a contradiction, and gives an error value.
	empty := tenon.Narrow(tenon.Narrow(port, tenon.NumberMin(tenon.NumberFromInt(1024), true)), tenon.NumberMax(tenon.NumberFromInt(80), true))
	fmt.Println(empty.IsError(), empty.Diagnostics()[0].Code)

	// Bounds say nothing about null, so the same bounds on a port that may
	// be absent leave it absent: null is the one value left.
	optional := tenon.Unknown(tenon.NumberType())
	absent := tenon.Narrow(tenon.Narrow(optional, tenon.NumberMin(tenon.NumberFromInt(1024), true)), tenon.NumberMax(tenon.NumberFromInt(80), true))
	fmt.Println(absent)
	// Output:
	// 443 true
	// true range.contradiction
	// null(number)
}

// A pending value has no type yet, only a constraint on what its type will be,
// which is how a program holds a value whose shape depends on data it has not
// read yet.
func ExamplePending() {
	// Whatever this turns out to be, it will be a list of something.
	items := tenon.Pending(tenon.ListOf(tenon.Any()))
	fmt.Println(items, items.IsPending())

	// Its length is a number, however little else is settled.
	fmt.Println(tenon.Length(items))

	// Resolve settles the type once it is known.
	fmt.Println(tenon.Resolve(items, tenon.ListType(tenon.StringType())))
	// Output:
	// pending(list_of(any)) true
	// unknown(number, not null, >= 0)
	// unknown(list(string))
}

// Numbers are exact. Arithmetic is never rounded, and a result that cannot be
// held is an error value rather than a rounded one.
func ExampleAdd() {
	tenth := tenon.NumberFromText("0.1")
	fmt.Println(tenon.Add(tenon.Add(tenth, tenth), tenth))

	// Bounds carry through: the sum of two numbers that are not known is
	// bounded by what their bounds allow.
	atLeastTwo := tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull(), tenon.NumberMin(tenon.NumberFromInt(2), true))
	fmt.Println(tenon.Add(atLeastTwo, tenon.NumberFromInt(40)))
	// Output:
	// 0.3
	// unknown(number, not null, >= 42)
}

// Equals answers what is known, and says it does not know where the values
// could still turn out either way.
func ExampleEquals() {
	// Values equal by what they are, not by how they were written.
	fmt.Println(tenon.Equals(tenon.NumberFromText("2.00"), tenon.NumberFromInt(2)))

	// A value that is not known could still be anything its range allows.
	small := tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull(), tenon.NumberMax(tenon.NumberFromInt(10), true))
	fmt.Println(tenon.Equals(small, tenon.NumberFromInt(5)))

	// Unless the ranges settle it: nothing at most ten is a hundred.
	fmt.Println(tenon.Equals(small, tenon.NumberFromInt(100)))
	// Output:
	// true
	// unknown(bool, not null)
	// false
}

// Length counts a string in grapheme clusters, which is what a reader would
// call a character, and a collection in members.
func ExampleLength() {
	// One e with an acute accent, written as two code points.
	fmt.Println(tenon.Length(tenon.String("e\U00000301")))
	fmt.Println(tenon.Length(tenon.List(tenon.NumberType(), tenon.NumberFromInt(1), tenon.NumberFromInt(2))))
	// Output:
	// 1
	// 2
}

// A set holds each value once, by what its members are rather than by how they
// were written.
func ExampleSet() {
	ports := tenon.Set(tenon.NumberType(),
		tenon.NumberFromInt(80),
		tenon.NumberFromText("80.0"),
		tenon.NumberFromInt(443),
	)
	fmt.Println(ports, tenon.Length(ports))
	fmt.Println(tenon.Contains(ports, tenon.NumberFromInt(443)))
	// Output:
	// set(number)[80, 443] 2
	// true
}

// Convert converts a value to a constraint under a policy the caller chooses,
// since whether text may stand where a number is expected is a question about
// a language rather than about values.
func ExampleConvert() {
	// Safe conversions change how a value is held, not what it is.
	tuple := tenon.Tuple(tenon.String("a"), tenon.String("b"))
	fmt.Println(tenon.Convert(tuple, tenon.ListOf(tenon.Exactly(tenon.StringType())), tenon.Safe))

	// Unsafe conversions change what a value is, and can fail on some values.
	fmt.Println(tenon.Convert(tenon.String("42"), tenon.Exactly(tenon.NumberType()), tenon.Unsafe))
	fmt.Println(tenon.Convert(tenon.String("42"), tenon.Exactly(tenon.NumberType()), tenon.Safe).IsError())
	// Output:
	// list(string)["a", "b"]
	// 42
	// true
}

// Data that is wrong becomes an error value carrying a diagnostic for each
// problem, located by its path, rather than a panic or a zero value.
func ExampleConvert_diagnostics() {
	ports := tenon.List(tenon.StringType(), tenon.String("80"), tenon.String("http"), tenon.String("443"))
	converted := tenon.Convert(ports, tenon.ListOf(tenon.Exactly(tenon.NumberType())), tenon.Unsafe)
	for _, d := range converted.Diagnostics() {
		fmt.Printf("%s at %s: %s\n", d.Code, d.Path, d.Message)
	}
	// Output:
	// number.invalid_syntax at .[1]: "http" is not a number
}

// However a value was built, two values that say the same thing are one value.
func ExampleIdentical() {
	fromParts := tenon.Object(map[string]tenon.Value{
		"ports": tenon.Set(tenon.NumberType(), tenon.NumberFromInt(443), tenon.NumberFromInt(80), tenon.NumberFromInt(443)),
	})
	written := tenon.Object(map[string]tenon.Value{
		"ports": tenon.Set(tenon.NumberType(), tenon.NumberFromText("80"), tenon.NumberFromText("443.0")),
	})
	fmt.Println(tenon.Identical(fromParts, written))
	fmt.Println(tenon.Hash(fromParts) == tenon.Hash(written))

	// Serialization agrees: one value has one encoding.
	a, _ := tenon.Serialize(fromParts)
	b, _ := tenon.Serialize(written)
	fmt.Println(string(a) == string(b))
	// Output:
	// true
	// true
	// true
}

// Serialize encodes a value as a CBOR document: what is not known, what it is
// bounded by, and the diagnostics of a failure all survive the round trip.
func ExampleSerialize() {
	value := tenon.Object(map[string]tenon.Value{
		"name": tenon.String("web"),
		"port": tenon.Narrow(tenon.Unknown(tenon.NumberType()), tenon.NotNull(), tenon.NumberMin(tenon.NumberFromInt(1024), true)),
	})
	encoded, err := tenon.Serialize(value)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(encoded), "bytes")

	back, err := tenon.Deserialize(encoded, tenon.Decoders{})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(back)
	fmt.Println(tenon.Identical(back, value))
	// Output:
	// 45 bytes
	// {"name": "web", "port": unknown(number, not null, >= 1024)}
	// true
}

// ProjectJSON renders a value for a consumer that speaks JSON and nothing
// else. It is one-way and lossy, and it refuses what JSON cannot say.
func ExampleProjectJSON() {
	known := tenon.Object(map[string]tenon.Value{
		"name": tenon.String("web"),
		"port": tenon.NumberFromInt(443),
	})
	text, _ := tenon.ProjectJSON(known)
	fmt.Println(string(text))

	withUnknown := tenon.Object(map[string]tenon.Value{
		"name": tenon.String("web"),
		"port": tenon.Unknown(tenon.NumberType()),
	})
	// A failure is a *tenon.Error, whose diagnostics each say what part
	// failed, and why.
	_, err := tenon.ProjectJSON(withUnknown)
	var failed *tenon.Error
	if errors.As(err, &failed) {
		fmt.Println(failed.Diagnostics()[0].Code, failed.Diagnostics()[0].Path)
	}
	fmt.Println(err)
	// Output:
	// {"name":"web","port":443}
	// serialize.not_known .port
	// serialize.not_known: an unknown value of type number has no content to project at .port
}

// Diff reports what changed between two values, each change located by its
// path, which is what a plan engine shows a person before it acts.
func ExampleDiff() {
	before := tenon.Object(map[string]tenon.Value{
		"name":     tenon.String("web"),
		"replicas": tenon.NumberFromInt(3),
	})
	after := tenon.Object(map[string]tenon.Value{
		"name":     tenon.String("web"),
		"replicas": tenon.NumberFromInt(5),
	})
	for _, change := range tenon.Diff(before, after) {
		fmt.Println(change)
	}
	// Output:
	// ~ .replicas: 3 -> 5
}

// Unify gives the one constraint that several of them come to, which is how a
// language decides what the branches of a conditional have in common.
func ExampleUnify() {
	number := tenon.Exactly(tenon.NumberType())
	text := tenon.Exactly(tenon.StringType())
	common, err := tenon.Unify([]tenon.Constraint{number, text}, tenon.Unsafe)
	fmt.Println(common, err)

	_, err = tenon.Unify([]tenon.Constraint{number, text}, tenon.Safe)
	fmt.Println(err)
	// Output:
	// exactly(string) <nil>
	// unify.no_common_constraint: exactly(number) and exactly(string) have no common constraint under the safe policy
}

// NewCapsule carries a Go type through tenon unchanged. The handle it returns
// builds the type's values and reads them back, the Go type checked where the
// program is compiled, and its Type stands wherever a type does.
func ExampleNewCapsule() {
	seed := maphash.MakeSeed()
	addresses := tenon.NewCapsule("ip", tenon.CapsuleOps[netip.Addr]{
		Equal:   func(a, b *netip.Addr) bool { return *a == *b },
		Hash:    func(a *netip.Addr) uint64 { return maphash.Comparable(seed, *a) },
		Display: func(a *netip.Addr) string { return a.String() },
	})
	gateway := netip.MustParseAddr("10.0.0.1")
	hosts := tenon.List(addresses.Type(), addresses.Value(&gateway))
	fmt.Println(hosts)

	if first, ok := addresses.Of(hosts.Index(0)); ok {
		fmt.Println(first.Is4(), *first == gateway)
	}
	_, ok := addresses.Of(tenon.String("10.0.0.1"))
	fmt.Println(ok)
	// Output:
	// list(capsule("ip"))[capsule("ip", "10.0.0.1")]
	// true true
	// false
}
