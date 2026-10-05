package tenon

import "testing"

// TestUnchangedIsConvert holds Convert's shortcut to the operation it
// spares: wherever unchanged says a conversion gives the value itself, the
// operation gives a value identical to it, under either policy, and it says
// so of the commonest conversions.
func TestUnchangedIsConvert(t *testing.T) {
	str, num, boo := StringType(), NumberType(), BoolType()
	secret := probe{id: "secret"}
	values := []Value{
		String(""), String("web"), NumberFromInt(0), NumberFromText("0.5"), NumberFromText("1e400"), Bool(true),
		List(str, String("a"), String("b")), List(str),
		Set(num, NumberFromInt(1), NumberFromInt(2)),
		Map(str, map[string]Value{"k": String("v")}),
		Tuple(String("a"), NumberFromInt(1), Bool(false)),
		Object(map[string]Value{"a": List(num, NumberFromInt(1)), "b": Null(str)}),
		Null(str), Unknown(str), Pending(Any()),
		List(str, String("a"), Unknown(str)),
		WithMarks(String("s"), secret),
		List(str, WithMarks(String("s"), secret)),
	}
	constraints := []Constraint{
		Any(), Exactly(str), Exactly(num), Exactly(boo), Exactly(ListType(str)), ListOf(Any()),
		SetOf(Exactly(num)), MapOf(Exactly(str)), Exactly(TupleType(str, num, boo)), OneOf(Exactly(num), Exactly(str)),
	}
	shortcut := 0
	for _, v := range values {
		for _, c := range constraints {
			if !unchanged(v, c) {
				continue
			}
			shortcut++
			for _, p := range []Policy{Safe, Unsafe} {
				if got := convertOp.with(conversion{target: c, policy: p}).apply(v); !Identical(got, v) {
					t.Errorf("converting %v to %s under %s gives %v, and unchanged says it gives the value itself", v, c, p, got)
				}
			}
		}
	}
	if shortcut != 18 {
		t.Errorf("unchanged holds for %d conversions here, want 18", shortcut)
	}
	for _, tt := range []struct {
		v Value
		c Constraint
	}{
		{String("web"), Exactly(str)}, {NumberFromInt(3), Exactly(num)}, {Bool(true), Exactly(boo)}, {String("web"), Any()},
	} {
		if !unchanged(tt.v, tt.c) {
			t.Errorf("unchanged(%v, %s) = false", tt.v, tt.c)
		}
	}
}
