package gotenon_test

import (
	"errors"
	"reflect"
	"testing"

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
	v := obj(map[string]tenon.Value{"name": s("web"), "ports": tenon.TupleVal(n(80), n(443))})
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
	err = gotenon.DecodeInto(obj(map[string]tenon.Value{"name": n(1), "ports": tenon.TupleVal()}), &kept, safe)
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
	attrs := map[string]tenon.Type{"name": str, "ports": tenon.List(num), "tags": tenon.List(str)}
	if got, ok := gotenon.TypeFor(rt); !ok || got != tenon.Object(attrs) {
		t.Errorf("TypeFor(server) = %v, %t, want %v", got, ok, tenon.Object(attrs))
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
		w := decoded[watched](t, obj(map[string]tenon.Value{"one": n(1), "many": tenon.TupleVal(n(1)), "plain": n(1), "plains": tenon.TupleVal(n(1)), "raw": n(1)}), p)
		if w.One.policy != p || w.Many[0].policy != p || w.Plain.policy != p || w.Plains[0].policy != p {
			t.Errorf("unmarshalers within a struct decoded under %s were told %s, %s, %s and %s", p, w.One.policy, w.Many[0].policy, w.Plain.policy, w.Plains[0].policy)
		}
	}
}
