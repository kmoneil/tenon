package gotenon_test

import (
	"errors"
	"math/big"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/gotenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// server is a struct whose Go type a program might have only when it runs.
type server struct {
	Name  string   `tenon:"name"`
	Ports []int    `tenon:"ports"`
	Tags  []string `tenon:"tags,optional"`
}

// TestConformance_GO001_MappingsForTypesKnownWhenRunning holds the runtime
// forms to what the generic ones do: DecodeInto decodes into the type its
// pointer points to, as Decode does, and leaves it as it was where decoding
// fails; ConstraintFor and TypeFor give the mapping of a reflect.Type in
// each direction.
func TestConformance_GO001_MappingsForTypesKnownWhenRunning(t *testing.T) {
	conformance.Covers(t, "GO-001", "GO-002")
	v := obj(map[string]tenon.Value{"name": s("web"), "ports": tenon.Tuple(n(80), n(443))})
	rt := reflect.TypeFor[server]()

	// A pointer made from a reflect.Type decodes as Decode decodes the type.
	dst := reflect.New(rt)
	if err := gotenon.DecodeInto(v, dst.Interface(), safe); err != nil {
		t.Fatalf("DecodeInto failed: %v", err)
	}
	want, err := gotenon.Decode[server](v, safe)
	if err != nil || !reflect.DeepEqual(dst.Elem().Interface(), want) {
		t.Errorf("DecodeInto gave %+v, and Decode %+v, %v", dst.Elem().Interface(), want, err)
	}
	// A failure leaves the destination as it was.
	kept := server{Name: "kept"}
	err = gotenon.DecodeInto(obj(map[string]tenon.Value{"name": n(1), "ports": tenon.Tuple()}), &kept, safe)
	var failed *tenon.Error
	if !errors.As(err, &failed) || kept.Name != "kept" {
		t.Errorf("a failed DecodeInto gave %v and left %+v, want a *tenon.Error and the value kept", err, kept)
	}

	// ConstraintFor is what Decode converts to, and TypeFor what Encode gives.
	fields := map[string]tenon.Field{
		"name":  tenon.Required(tenon.Exactly(str)),
		"ports": tenon.Required(tenon.ListOf(tenon.Exactly(num))),
		"tags":  tenon.Optional(tenon.ListOf(tenon.Exactly(str))),
	}
	if got, want := gotenon.ConstraintFor(rt), tenon.ObjectWith(fields, true); !got.Equal(want) {
		t.Errorf("ConstraintFor(server) = %v, want %v", got, want)
	}
	attrs := map[string]tenon.Type{"name": str, "ports": tenon.ListType(num), "tags": tenon.ListType(str)}
	if got, ok := gotenon.TypeFor(rt); !ok || got != tenon.ObjectType(attrs) {
		t.Errorf("TypeFor(server) = %v, %t, want %v", got, ok, tenon.ObjectType(attrs))
	}
	if got, ok := gotenon.TypeFor(reflect.TypeFor[server]()); ok && got != encoded(t, want).Type() {
		t.Errorf("TypeFor(server) = %v, but Encode gives %v", got, encoded(t, want).Type())
	}
	for _, rt := range []reflect.Type{reflect.TypeFor[tenon.Value](), reflect.TypeFor[map[string]any](), reflect.TypeFor[[]tenon.Value]()} {
		if got, ok := gotenon.TypeFor(rt); ok || !got.IsZero() {
			t.Errorf("TypeFor(%s) = %v, %t, want no type, since what its values encode as depends on what they hold", rt, got, ok)
		}
	}
	if got := gotenon.ConstraintFor(reflect.TypeFor[tenon.Value]()); !got.Equal(tenon.Any()) {
		t.Errorf("ConstraintFor(tenon.Value) = %v, want any", got)
	}

	// Each panics where its generic form would, and on what is not a type or
	// a pointer.
	var none *server
	mustPanicUsage(t, "not a non-nil pointer", func() { gotenon.DecodeInto(v, server{}, safe) })
	mustPanicUsage(t, "not a non-nil pointer", func() { gotenon.DecodeInto(v, none, safe) })
	mustPanicUsage(t, "not a non-nil pointer", func() { gotenon.DecodeInto(v, nil, safe) })
	mustPanicUsage(t, "is an interface", func() { gotenon.DecodeInto(v, new(any), safe) })
	mustPanicUsage(t, "neither Safe nor Unsafe", func() { gotenon.DecodeInto(v, &kept, tenon.Policy(9)) })
	mustPanicUsage(t, "is an interface", func() { gotenon.ConstraintFor(reflect.TypeFor[any]()) })
	mustPanicUsage(t, "does not map to tenon", func() { gotenon.ConstraintFor(reflect.TypeFor[chan int]()) })
	mustPanicUsage(t, "does not map to tenon", func() { gotenon.TypeFor(reflect.TypeFor[func()]()) })
	mustPanicUsage(t, "nil reflect.Type", func() { gotenon.ConstraintFor(nil) })
	mustPanicUsage(t, "nil reflect.Type", func() { gotenon.TypeFor(nil) })
}

// TestConformance_GO040_UnmarshalersAreGivenThePolicy holds an unmarshaler to
// being told the policy the decoding was given, at the top and within a
// container, so that it converts what it is given as the rest does.
func TestConformance_GO040_UnmarshalersAreGivenThePolicy(t *testing.T) {
	conformance.Covers(t, "GO-040")
	for _, p := range []tenon.Policy{tenon.Safe, tenon.Unsafe} {
		if got := decoded[observer](t, n(1), p); got.policy != p {
			t.Errorf("an unmarshaler decoded under %s was told %s", p, got.policy)
		}
		w := decoded[watched](t, obj(map[string]tenon.Value{"one": n(1), "many": tenon.Tuple(n(1)), "plain": n(1), "plains": tenon.Tuple(n(1)), "raw": n(1)}), p)
		if w.One.policy != p || w.Many[0].policy != p || w.Plain.policy != p || w.Plains[0].policy != p {
			t.Errorf("unmarshalers within a struct decoded under %s were told %s, %s, %s and %s", p, w.One.policy, w.Many[0].policy, w.Plain.policy, w.Plains[0].policy)
		}
	}
}

// badText marshals itself to text that is not UTF-8.
type badText struct{}

func (badText) MarshalText() ([]byte, error) { return []byte{'a', 0xff}, nil }

// bothWays marshals itself to a value and to text; the value comes first.
type bothWays struct{}

func (bothWays) MarshalValue() (tenon.Value, error) { return tenon.NumberFromInt(1), nil }
func (bothWays) MarshalText() ([]byte, error)       { return []byte("text"), nil }

// textOut marshals itself to text, and decodes by its struct mapping.
type textOut struct {
	N int `tenon:"n"`
}

func (t textOut) MarshalText() ([]byte, error) { return []byte("out"), nil }

// event holds values that marshal themselves to text, and a pointer to one.
type event struct {
	At   time.Time  `tenon:"at"`
	From netip.Addr `tenon:"from"`
	Done *time.Time `tenon:"done,optional"`
}

// TestConformance_GO044_TextMarshalers holds a Go type that marshals itself
// to text to encoding as the String of its text, and one whose pointer
// unmarshals itself from text to decoding from a String, in each direction on
// its own, after tenon's own marshalers and the types the mapping names; and
// a struct whose state is all in unexported fields, which would otherwise
// cross as an empty object, to being a usage error.
func TestConformance_GO044_TextMarshalers(t *testing.T) {
	conformance.Covers(t, "GO-044", "GO-010", "GO-011")
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	from := netip.MustParseAddr("10.0.0.1")
	x := event{At: at, From: from}
	v := encoded(t, x)
	want := obj(map[string]tenon.Value{"at": s("2026-09-27T12:00:00Z"), "from": s("10.0.0.1"), "done": tenon.Null(str)})
	wantValue(t, "a struct of text marshalers", v, want)
	if got, ok := gotenon.TypeFor(reflect.TypeFor[event]()); !ok || got != v.Type() {
		t.Errorf("TypeFor(event) = %v, %t, want %v", got, ok, v.Type())
	}
	if back := decoded[event](t, v, safe); !back.At.Equal(at) || back.From != from || back.Done != nil {
		t.Errorf("the event came back as %+v", back)
	}

	// Decoding converts to a string first, under the policy, and hands the
	// method its text; what the method refuses fails where the part is.
	wantDecodeFailures[time.Time](t, "a number, safely", n(1), safe, wantDiag{tenon.CodeConvertUnsafe, "."})
	_, err := gotenon.Decode[event](obj(map[string]tenon.Value{"at": s("noon"), "from": s("10.0.0.1")}), safe)
	var failed *tenon.Error
	var parse *time.ParseError
	if !errors.As(err, &failed) || failed.Diagnostics()[0].Code != tenon.CodeDecodeUnmarshalFailed ||
		failed.Diagnostics()[0].Path.String() != ".at" || !errors.As(err, &parse) {
		t.Errorf("decoding an unreadable time gave %v, want %s at .at with the time.ParseError as a cause", err, tenon.CodeDecodeUnmarshalFailed)
	}
	wantDecodeFailures[time.Time](t, "an unknown string", tenon.Unknown(str), safe, wantDiag{tenon.CodeDecodeNotKnown, "."})
	wantDecodeFailures[time.Time](t, "a null", tenon.Null(str), safe, wantDiag{tenon.CodeDecodeNull, "."})

	// Encoding fails where the method does, or its text is not UTF-8.
	_, err = gotenon.Encode(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC))
	if !errors.As(err, &failed) || failed.Diagnostics()[0].Code != tenon.CodeEncodeMarshalFailed {
		t.Errorf("encoding the year 10000 gave %v, want %s", err, tenon.CodeEncodeMarshalFailed)
	}
	wantEncodeFailure(t, "text that is not UTF-8", badText{}, wantDiag{tenon.CodeStringInvalidUTF8, "."})

	// tenon's own marshalers come first, and each direction maps on its own.
	wantValue(t, "a type marshaling itself both ways", encoded(t, bothWays{}), n(1))
	wantValue(t, "a type marshaling itself to text", encoded(t, textOut{N: 2}), s("out"))
	if got := decoded[textOut](t, obj(map[string]tenon.Value{"n": n(3)}), safe); got.N != 3 {
		t.Errorf("decoding by the struct mapping gave %+v", got)
	}
	// The big numbers marshal themselves to text, and map as numbers all the
	// same.
	if got, _ := gotenon.TypeFor(reflect.TypeFor[big.Int]()); got != num {
		t.Errorf("TypeFor(big.Int) = %v, want number", got)
	}

	// A struct whose state is all unexported is refused, both ways; one with
	// no fields at all holds nothing to lose.
	type opaque struct{ secret int }
	mustPanicUsage(t, "holds its state in unexported fields", func() { gotenon.Encode(opaque{1}) })
	mustPanicUsage(t, "holds its state in unexported fields", func() { gotenon.Decode[opaque](obj(nil), safe) })
	wantValue(t, "an empty struct", encoded(t, struct{}{}), obj(nil))
}

// TestConformance_GO044_TenonHandlesCrossAsText holds a tenon.Type, a
// tenon.Constraint and a tenon.Path held in a Go value to encoding as the
// text of their display forms, which they marshal themselves to; none
// unmarshals itself from text, so decoding into one is refused.
func TestConformance_GO044_TenonHandlesCrossAsText(t *testing.T) {
	conformance.Covers(t, "GO-044", "DI-018")
	type described struct {
		T tenon.Type       `tenon:"t"`
		C tenon.Constraint `tenon:"c"`
		P tenon.Path       `tenon:"p"`
	}
	x := described{tenon.ListType(str), tenon.ListOf(tenon.Any()), tenon.Path{}.Attribute("a")}
	wantValue(t, "a struct of tenon handles", encoded(t, x),
		obj(map[string]tenon.Value{"t": s("list(string)"), "c": s("list_of(any)"), "p": s(".a")}))
	mustPanicUsage(t, "holds its state in unexported fields", func() {
		gotenon.Decode[described](obj(map[string]tenon.Value{"t": s("string"), "c": s("any"), "p": s(".")}), safe)
	})
}
