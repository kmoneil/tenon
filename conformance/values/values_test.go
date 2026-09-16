package values_test

import (
	"testing"

	"tenon"
	"tenon/conformance/values"
)

// TestEveryShapeIsThere holds the generator to its promise: a property test
// over it is only as good as what it covers.
func TestEveryShapeIsThere(t *testing.T) {
	states := map[string]bool{}
	kinds := map[tenon.Kind]bool{}
	partial, bounded := false, false
	for _, v := range values.All() {
		switch {
		case v.IsError():
			states["error"] = true
		case v.IsPending():
			states["pending"] = true
		default:
			kinds[v.Type().Kind()] = true
			switch {
			case !v.IsKnown() && v.Type().Kind() != tenon.KindList:
				states["unknown"] = true
			case !v.IsKnown():
				partial = true
			case tenon.IsNull(v).String() == "true":
				states["null"] = true
			default:
				states["known"] = true
			}
		}
		if v.IsResolved() && !v.IsKnown() && v.Range().String() != v.Type().String() {
			bounded = true
		}
	}
	for _, want := range []string{"error", "pending", "null", "unknown", "known"} {
		if !states[want] {
			t.Errorf("no %s value in the generator", want)
		}
	}
	for k := tenon.KindBool; k <= tenon.KindCapsule; k++ {
		if !kinds[k] {
			t.Errorf("no value of kind %v in the generator", k)
		}
	}
	if !partial {
		t.Error("no container holding a member that is not known")
	}
	if !bounded {
		t.Error("no unknown value with a narrowed range")
	}
	if got := len(values.Orderable()); got == 0 || got == len(values.All()) {
		t.Errorf("Orderable returned %d of %d values", got, len(values.All()))
	}
}

// TestMarkedValuesAreThere holds the generator to covering marks: a value
// carrying one in every state, and a container that holds one without
// carrying one, known and not.
func TestMarkedValuesAreThere(t *testing.T) {
	carries := map[string]bool{}
	holdsKnown, holdsPartial := false, false
	for _, v := range values.All() {
		_, own := tenon.Unmark(v)
		_, all := tenon.UnmarkDeep(v)
		switch {
		case len(own) > 0 && v.IsError():
			carries["error"] = true
		case len(own) > 0 && v.IsPending():
			carries["pending"] = true
		case len(own) > 0 && v.IsKnown() && !v.HasContent():
			carries["null"] = true
		case len(own) > 0 && !v.IsKnown():
			carries["unknown"] = true
		case len(own) > 0:
			carries["known"] = true
		case len(all) > 0 && v.IsKnown():
			holdsKnown = true
		case len(all) > 0:
			holdsPartial = true
		}
	}
	for _, want := range []string{"error", "pending", "null", "unknown", "known"} {
		if !carries[want] {
			t.Errorf("no marked %s value in the generator", want)
		}
	}
	if !holdsKnown {
		t.Error("no known container holding a marked member")
	}
	if !holdsPartial {
		t.Error("no container holding a marked member that is not known")
	}
}

// TestDeepMarkedValuesAreThere holds the generator to covering deep marks on a
// set, whose members carry the mark only once read, and on the other kinds of
// container, whose members carry it in storage.
func TestDeepMarkedValuesAreThere(t *testing.T) {
	kinds := map[tenon.Kind]bool{}
	for _, v := range values.All() {
		_, own := tenon.Unmark(v)
		for _, m := range own {
			if d, ok := m.(tenon.DeepMark); ok && d.Deep() && v.IsResolved() {
				kinds[v.Type().Kind()] = true
			}
		}
	}
	for _, want := range []tenon.Kind{tenon.KindList, tenon.KindSet, tenon.KindMap, tenon.KindObject} {
		if !kinds[want] {
			t.Errorf("no deep-marked value of kind %v in the generator", want)
		}
	}
}
