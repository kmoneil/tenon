package tenon

import "testing"

// rangeOf returns the range description of an unknown value.
func rangeOf(t *testing.T, v Value) *rangeData {
	t.Helper()
	r, ok := v.n.data.(*rangeData)
	if !ok {
		t.Fatalf("%v is not an unknown value", v)
	}
	return r
}

func TestRangesAreCanonical(t *testing.T) {
	str, num := StringType(), NumberType()
	one := NumberFromInt(1)
	// The same set of possible values, reached by different narrowings in
	// different orders, is the same range field for field. Comparing two
	// ranges never has to reason about what a narrowing implies, because
	// every implication is recorded when the narrowing is applied.
	for _, tt := range []struct {
		name string
		a, b Value
	}{
		{
			"a prefix implies a least length",
			Narrow(Unknown(str), StringPrefix("ab"), LengthMax(5)),
			Narrow(Unknown(str), LengthMax(5), LengthMin(2), StringPrefix("ab")),
		},
		{
			"the tighter of two bounds at the same value",
			Narrow(Unknown(num), NumberMin(one, false)),
			Narrow(Unknown(num), NumberMin(one, true), NumberMin(one, false)),
		},
		{
			"the same bound written two ways",
			Narrow(Unknown(num), NumberMin(one, true)),
			Narrow(Unknown(num), NumberMin(NumberFromText("1.00"), true)),
		},
		{
			"a weaker narrowing leaves no trace",
			Narrow(Unknown(str), LengthMax(3)),
			Narrow(Unknown(str), LengthMax(3), LengthMax(9), LengthMin(0)),
		},
	} {
		if a, b := rangeOf(t, tt.a), rangeOf(t, tt.b); !a.equal(b) {
			t.Errorf("%s: %+v and %+v are not the same range", tt.name, *a, *b)
		}
	}
	// The zero range is the whole domain, so a fresh unknown records nothing.
	if r := rangeOf(t, Unknown(str)); !r.equal(&rangeData{}) {
		t.Errorf("a fresh unknown has range %+v, want the zero range", *r)
	}
	// A narrowing that says nothing new returns the value itself rather than
	// a copy of it.
	v := Narrow(Unknown(str), LengthMax(3))
	if got := Narrow(v, LengthMax(9)); got != v {
		t.Error("a narrowing that says nothing new produced a new value")
	}
	if got := Narrow(v); got != v {
		t.Error("narrowing by nothing produced a new value")
	}
}
