package tenon_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
)

// TestConformance_UN024_APendingCollectionHasALength holds a pending value
// whose constraint admits lists, sets and maps alone to the lengths it is
// narrowed to: it records the least and the greatest, which its display
// shows; a constraint that admits another type takes none; bounds that leave
// no length leave it null where it may be null and are a contradiction where
// it may not; a value known to be null records none; Length is the number
// within them; and resolving it narrows the unknown value by them.
func TestConformance_UN024_APendingCollectionHasALength(t *testing.T) {
	conformance.Covers(t, "UN-024", "UN-002", "UN-004", "UN-023")
	lists := tenon.ListOf(tenon.Any())
	p := tenon.Pending(lists)
	two := tenon.Narrow(p, tenon.LengthMin(2))
	if got, want := two.String(), "pending(list_of(any), length >= 2)"; got != want {
		t.Errorf("a pending list of at least two reads as %s, want %s", got, want)
	}
	bounded := tenon.Narrow(two, tenon.NotNull(), tenon.LengthMax(5))
	if got, want := bounded.String(), "pending(list_of(any), not null, length >= 2, length <= 5)"; got != want {
		t.Errorf("a pending list of two to five reads as %s, want %s", got, want)
	}
	if again := tenon.Narrow(two, tenon.LengthMin(1)); !tenon.SameNode(again, two) {
		t.Errorf("a least length below the one recorded made a new value: %v", again)
	}

	for _, c := range []tenon.Constraint{tenon.SetOf(tenon.Any()), tenon.MapOf(is(num)), is(tenon.ListType(num)),
		tenon.OneOf(lists, tenon.SetOf(tenon.Any()))} {
		if r := tenon.Narrow(tenon.Pending(c), tenon.LengthMax(3)); r.IsError() || !strings.Contains(r.String(), "length <= 3") {
			t.Errorf("a pending value of %v narrowed to at most three: %v", c, r)
		}
	}
	for _, c := range []tenon.Constraint{tenon.Any(), tenon.OneOf(lists, is(str)), tenon.TupleOf(tenon.Any()),
		tenon.ObjectWith(nil, false), is(str)} {
		mustPanicUsage(t, "applies to a pending value only where every type its constraint admits is a list, a set or a map", func() {
			tenon.Narrow(tenon.Pending(c), tenon.LengthMin(1))
		})
	}

	nullP := tenon.Narrow(p, tenon.NullOnly())
	if got := tenon.Narrow(two, tenon.LengthMax(1)); !tenon.Identical(got, nullP) {
		t.Errorf("a pending list that may be null, of at least two and at most one, is %v, want %v", got, nullP)
	}
	if got := tenon.Narrow(bounded, tenon.LengthMax(1)); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeRangeContradiction {
		t.Errorf("a pending list that is not null, of at least two and at most one, is %v, want a contradiction", got)
	}
	if got := tenon.Narrow(nullP, tenon.LengthMin(2)); !tenon.Identical(got, nullP) {
		t.Errorf("a pending null narrowed by a length is %v, want %v", got, nullP)
	}

	if got, want := tenon.Length(bounded).String(), "unknown(number, not null, >= 2, <= 5)"; got != want {
		t.Errorf("the length of %v is %s, want %s", bounded, got, want)
	}
	strs := tenon.ListType(str)
	if got, want := tenon.Resolve(bounded, strs), tenon.Narrow(tenon.Unknown(strs), tenon.NotNull(), tenon.LengthMin(2), tenon.LengthMax(5)); !tenon.Identical(got, want) {
		t.Errorf("resolving %v to %v gave %v, want %v", bounded, strs, got, want)
	}
	// A set of an element type with few values may have fewer members than
	// the pending value said it would.
	bools := tenon.SetType(tenon.BoolType())
	if got := tenon.Resolve(tenon.Narrow(tenon.Pending(tenon.SetOf(tenon.Any())), tenon.NotNull(), tenon.LengthMin(4)), bools); !got.IsError() {
		t.Errorf("resolving a pending set of at least four to %v gave %v, want a contradiction", bools, got)
	}
}

// TestConformance_UN023_EqualsReadsAPendingCollectionsLength holds Equals with
// a pending operand to what the lengths it records settle: known false against
// a value whose length they exclude, unless both operands could still be
// null, and unknown where some length could be both.
func TestConformance_UN023_EqualsReadsAPendingCollectionsLength(t *testing.T) {
	conformance.Covers(t, "UN-023", "EQ-005")
	lists := tenon.ListOf(tenon.Any())
	atMostOne := tenon.Narrow(tenon.Pending(lists), tenon.LengthMax(1))
	atLeastThree := tenon.Narrow(tenon.Pending(lists), tenon.NotNull(), tenon.LengthMin(3))
	nums := tenon.ListType(num)
	for _, tt := range []struct {
		name string
		a, b tenon.Value
		want string
	}{
		{"a known list too long", atMostOne, tenon.List(num, n(1), n(2)), "false"},
		{"a known list short enough", atMostOne, tenon.List(num, n(1)), "unknown(bool, not null)"},
		{"an unknown list too short, not null", atLeastThree, tenon.Narrow(tenon.Unknown(nums), tenon.LengthMax(2)), "false"},
		{"an unknown list too short, both maybe null", tenon.Narrow(tenon.Pending(lists), tenon.LengthMin(3)), tenon.Narrow(tenon.Unknown(nums), tenon.LengthMax(2)), "unknown(bool, not null)"},
		{"two pending lists of lengths apart", atLeastThree, atMostOne, "false"},
		{"a pending list of one type", tenon.Narrow(tenon.Pending(is(nums)), tenon.LengthMax(1)), tenon.List(num, n(1), n(2)), "false"},
		{"an empty map", tenon.Narrow(tenon.Pending(tenon.MapOf(tenon.Any())), tenon.LengthMin(1)), tenon.Map(num, nil), "false"},
		{"a set of two", tenon.Narrow(tenon.Pending(tenon.SetOf(tenon.Any())), tenon.LengthMin(3)), tenon.Set(tenon.BoolType(), tenon.Bool(true), tenon.Bool(false)), "false"},
	} {
		if got := tenon.Equals(tt.a, tt.b).String(); got != tt.want {
			t.Errorf("%s: Equals(%v, %v) = %s, want %s", tt.name, tt.a, tt.b, got, tt.want)
		}
		if got := tenon.Equals(tt.b, tt.a).String(); got != tt.want {
			t.Errorf("%s, the other way: Equals(%v, %v) = %s, want %s", tt.name, tt.b, tt.a, got, tt.want)
		}
	}
}

// TestConformance_EQ010_APendingValuesLengthsAreItsOwn holds Identical to a
// pending value's lengths, and GoString to the Go that builds them.
func TestConformance_EQ010_APendingValuesLengthsAreItsOwn(t *testing.T) {
	conformance.Covers(t, "EQ-010", "UN-024")
	p := tenon.Pending(tenon.ListOf(tenon.Any()))
	two, three := tenon.Narrow(p, tenon.LengthMin(2)), tenon.Narrow(p, tenon.LengthMin(3))
	if tenon.Identical(two, three) || tenon.Identical(two, p) {
		t.Errorf("pending lists of different lengths are identical: %v, %v, %v", p, two, three)
	}
	if !tenon.Identical(two, tenon.Narrow(tenon.Pending(tenon.ListOf(tenon.Any())), tenon.LengthMin(2))) {
		t.Errorf("two pending lists of at least two are not identical")
	}
	bounded := tenon.Narrow(p, tenon.NotNull(), tenon.LengthMin(2), tenon.LengthMax(5))
	if got, want := fmt.Sprintf("%#v", bounded), "tenon.Narrow(tenon.Pending(tenon.ListOf(tenon.Any())), tenon.NotNull(), tenon.LengthMin(2), tenon.LengthMax(5))"; got != want {
		t.Errorf("%%#v is %s, want %s", got, want)
	}
}

// TestConformance_SE010_APendingValueCarriesItsLengths pins a pending value's
// lengths in its item, a range holding keys 4 and 5 alone and present only
// where it records one, read back as itself; and the input that describes no
// value refused: lengths on a value that cannot take them, on one known to be
// null, another key, and bounds that leave no length.
func TestConformance_SE010_APendingValueCarriesItsLengths(t *testing.T) {
	conformance.Covers(t, "SE-010", "SE-034", "SE-002")
	p := tenon.Pending(tenon.ListOf(tenon.Any()))
	for _, tt := range []struct {
		name string
		v    tenon.Value
		item string
	}{
		{"no length", p, "83 01 82 03 81 02 00"},
		{"a least length", tenon.Narrow(p, tenon.LengthMin(2)), "84 01 82 03 81 02 00 a1 04 02"},
		{"both, not null", tenon.Narrow(p, tenon.NotNull(), tenon.LengthMin(2), tenon.LengthMax(5)), "84 01 82 03 81 02 01 a2 04 02 05 05"},
		{"a greatest length of none", tenon.Narrow(p, tenon.LengthMax(0)), "84 01 82 03 81 02 00 a1 05 00"},
	} {
		wantEncoding(t, tt.name, tt.v, tt.item)
		if got, _, ok := tryDeserialize(fromHex(t, document+tt.item), decoders); !ok || !tenon.Identical(got, tt.v) {
			t.Errorf("%s came back as %v", tt.name, got)
		}
	}
	for _, tt := range []struct {
		name, item string
		code       tenon.Code
	}{
		{"lengths on a pending value of any", "84 01 81 02 00 a1 04 02", tenon.CodeSerializeMalformed},
		{"lengths on a pending null", "84 01 82 03 81 02 02 a1 04 02", tenon.CodeSerializeMalformed},
		{"a least length above the greatest", "84 01 82 03 81 02 00 a2 04 03 05 01", tenon.CodeSerializeMalformed},
		{"another range key", "84 01 82 03 81 02 00 a1 00 f5", tenon.CodeSerializeMalformed},
		{"an empty range", "84 01 82 03 81 02 00 a0", tenon.CodeSerializeNotCanonical},
		{"a least length of none", "84 01 82 03 81 02 00 a1 04 00", tenon.CodeSerializeNotCanonical},
	} {
		wantDecodeFailure(t, tt.name, document+tt.item, tt.code)
	}
}

// TestConformance_CV032_APendingCollectionKeepsItsLengths holds the
// conversion of a pending list, set or map to the lengths it keeps, as an
// unknown collection's are kept: both into a list or a map, and into a set
// the greatest and a least of one, members that convert to one value merging;
// in the unknown value a constraint that gives a type converts it to, in the
// pending value of one that does not, and in the unknown value that a pending
// value of one type converts to; and kept whole converted to Any.
func TestConformance_CV032_APendingCollectionKeepsItsLengths(t *testing.T) {
	conformance.Covers(t, "CV-032", "UN-024")
	lists, sets := tenon.ListOf(tenon.Any()), tenon.SetOf(tenon.Any())
	two := tenon.Narrow(tenon.Pending(lists), tenon.LengthMin(2), tenon.LengthMax(5))
	nums := tenon.ListType(num)
	for _, tt := range []struct {
		name string
		v    tenon.Value
		c    tenon.Constraint
		p    tenon.Policy
		want tenon.Value
	}{
		{"to a list of one type", two, tenon.ListOf(is(num)), safe, tenon.Narrow(tenon.Unknown(nums), tenon.LengthMin(2), tenon.LengthMax(5))},
		{"to a set of one type", two, tenon.SetOf(is(num)), uns, tenon.Narrow(tenon.Unknown(tenon.SetType(num)), tenon.LengthMin(1), tenon.LengthMax(5))},
		{"to a pending set", two, sets, uns, tenon.Narrow(tenon.Pending(sets), tenon.LengthMin(1), tenon.LengthMax(5))},
		{"to a pending list", two, tenon.ListOf(tenon.OneOf(is(num), is(str))), safe, tenon.Narrow(tenon.Pending(tenon.ListOf(tenon.OneOf(is(num), is(str)))), tenon.LengthMin(2), tenon.LengthMax(5))},
		{"to any", two, tenon.Any(), safe, two},
		{"a pending set to a list", tenon.Narrow(tenon.Pending(sets), tenon.NotNull(), tenon.LengthMin(3)), lists, safe,
			tenon.Narrow(tenon.Pending(lists), tenon.NotNull(), tenon.LengthMin(3))},
		{"a pending list of one type to strings", tenon.Narrow(tenon.Pending(is(nums)), tenon.LengthMax(1)), tenon.ListOf(is(str)), uns,
			tenon.Narrow(tenon.Unknown(tenon.ListType(str)), tenon.LengthMax(1))},
	} {
		wantValue(t, tt.name, tenon.Convert(tt.v, tt.c, tt.p), tt.want)
	}
}
