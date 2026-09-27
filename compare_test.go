package tenon_test

import (
	"reflect"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// TestHandlesAreNotComparable holds Value, Constraint and Path, and what holds
// them, to refusing ==, which would compare how each is held rather than what
// it says: String("a") built twice would be two map keys. Type stays
// comparable, since it is interned, and refusing == costs no space.
func TestHandlesAreNotComparable(t *testing.T) {
	for _, rt := range []reflect.Type{
		reflect.TypeFor[tenon.Value](),
		reflect.TypeFor[tenon.Constraint](),
		reflect.TypeFor[tenon.Path](),
		reflect.TypeFor[tenon.Step](),
		reflect.TypeFor[tenon.Diagnostic](),
		reflect.TypeFor[tenon.Change](),
	} {
		if rt.Comparable() {
			t.Errorf("%s is comparable", rt)
		}
	}
	if !reflect.TypeFor[tenon.Type]().Comparable() {
		t.Error("Type is not comparable")
	}
	pointer := reflect.TypeFor[*int]().Size()
	for _, rt := range []reflect.Type{
		reflect.TypeFor[tenon.Value](),
		reflect.TypeFor[tenon.Constraint](),
		reflect.TypeFor[tenon.Path](),
	} {
		if rt.Size() != pointer {
			t.Errorf("%s takes %d bytes, want %d", rt, rt.Size(), pointer)
		}
	}
}

// TestEqualMethods holds each handle's Equal method to the form go-cmp's
// cmp.Equal calls, (T) Equal(T) bool, so that a struct holding one compares
// by what it says without an option.
func TestEqualMethods(t *testing.T) {
	for _, rt := range []reflect.Type{
		reflect.TypeFor[tenon.Value](),
		reflect.TypeFor[tenon.Type](),
		reflect.TypeFor[tenon.Constraint](),
		reflect.TypeFor[tenon.Path](),
		reflect.TypeFor[tenon.Step](),
		reflect.TypeFor[tenon.Diagnostic](),
	} {
		m, ok := rt.MethodByName("Equal")
		if !ok {
			t.Errorf("%s has no Equal method", rt)
			continue
		}
		if ft := m.Type; ft.NumIn() != 2 || ft.In(1) != rt || ft.NumOut() != 1 || ft.Out(0) != reflect.TypeFor[bool]() {
			t.Errorf("%s.Equal is %s, want func(%s) bool", rt, ft, rt)
		}
	}
}

// TestValueEqual holds Value.Equal to Identical, over the values of the
// conformance corpus, and to answering for the zero Value, which is equal
// only to itself.
func TestValueEqual(t *testing.T) {
	all := values.All()
	for _, v := range all {
		for _, w := range all {
			if got, want := v.Equal(w), tenon.Identical(v, w); got != want {
				t.Errorf("%v.Equal(%v) = %t, want %t as Identical", v, w, got, want)
			}
		}
	}
	one := tenon.NumberFromInt(1)
	if !one.Equal(tenon.NumberFromText("1.0")) {
		t.Error("1 and 1.0, built apart, are not Equal")
	}
	if one.Equal(tenon.WithMarks(one, bare("m"))) {
		t.Error("1 and a marked 1 are Equal, though Identical tells them apart")
	}
	var zero tenon.Value
	if !zero.IsZero() || one.IsZero() {
		t.Error("IsZero does not tell the zero Value from a value")
	}
	if zero.Equal(one) || one.Equal(zero) || !zero.Equal(tenon.Value{}) {
		t.Error("the zero Value is Equal to a value, or not to itself")
	}
}

// TestZeroHandles holds IsZero and Equal on Constraint, Path and Step to
// answering for the zero value, which is equal only to itself.
func TestZeroHandles(t *testing.T) {
	var c tenon.Constraint
	if !c.IsZero() || tenon.Any().IsZero() {
		t.Error("IsZero does not tell the zero Constraint from a constraint")
	}
	if c.Equal(tenon.Any()) || tenon.Any().Equal(c) || !c.Equal(tenon.Constraint{}) || !tenon.Any().Equal(tenon.Any()) {
		t.Error("Constraint.Equal is wrong about the zero Constraint")
	}

	var p tenon.Path
	a := p.Attribute("a")
	if !p.IsZero() || a.IsZero() || !a.Steps()[0].Equal(tenon.Path{}.Attribute("a").Steps()[0]) {
		t.Error("IsZero does not tell the empty path from a longer one")
	}
	if p.Equal(a) || a.Equal(p) || !p.Equal(tenon.Path{}) || !a.Equal(tenon.Path{}.Attribute("a")) {
		t.Error("Path.Equal is wrong about the empty path")
	}

	var s tenon.Step
	attr := a.Steps()[0]
	index := p.Index(tenon.NumberFromInt(0)).Steps()[0]
	if !s.IsZero() || attr.IsZero() || index.IsZero() {
		t.Error("IsZero does not tell the zero Step from a step")
	}
	for _, step := range []tenon.Step{attr, index} {
		if s.Equal(step) || step.Equal(s) || !step.Equal(step) {
			t.Errorf("Step.Equal is wrong about the zero Step and %s", step)
		}
	}
	if !s.Equal(tenon.Step{}) || attr.Equal(index) {
		t.Error("Step.Equal is wrong about the zero Step, or two steps of two kinds")
	}
}
