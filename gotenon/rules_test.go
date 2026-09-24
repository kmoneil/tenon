package gotenon_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/gotenon"
)

// TestConformance_GO001_AMappingHasTwoDirections pins the rule's two verbs:
// encoding makes a value from a Go value, decoding makes a Go value back,
// through the mapping the Go type has.
func TestConformance_GO001_AMappingHasTwoDirections(t *testing.T) {
	conformance.Covers(t, "GO-001")
	wantValue(t, "Encode(42)", encoded(t, 42), n(42))
	if got := decoded[int](t, n(42), safe); got != 42 {
		t.Errorf("Decode[int](42) = %d", got)
	}
}

// TestConformance_GO014_ObjectsIntoMapsAndMapsIntoStructs pins the two
// crossings the rule allows: an object decodes into a Go map under either
// policy, and a map decodes into a struct under the unsafe policy only,
// since decoding is conversion and invents no second answer.
func TestConformance_GO014_ObjectsIntoMapsAndMapsIntoStructs(t *testing.T) {
	conformance.Covers(t, "GO-014")
	o := obj(map[string]tenon.Value{"a": n(1)})
	for _, p := range []tenon.Policy{safe, uns} {
		if got := decoded[map[string]int](t, o, p); len(got) != 1 || got["a"] != 1 {
			t.Errorf("an object under %v decoded to %v", p, got)
		}
	}
	type onlyA struct {
		A int `tenon:"a"`
	}
	mv := tenon.MapVal(num, map[string]tenon.Value{"a": n(1)})
	if got, err := gotenon.Decode[onlyA](mv, safe); err == nil {
		t.Errorf("a map decoded into a struct safely: %v", got)
	}
	if got := decoded[onlyA](t, mv, uns); got.A != 1 {
		t.Errorf("a map under the unsafe policy decoded to %+v", got)
	}
}

// TestConformance_GO020_FieldsAreNamedByTagOrName pins the attribute names a
// struct maps to: the tenon tag where there is one, the field's own name
// where there is none, a "-" tag and an unexported field left out.
func TestConformance_GO020_FieldsAreNamedByTagOrName(t *testing.T) {
	conformance.Covers(t, "GO-020")
	type tagged struct {
		A       int
		B       int `tenon:"b"`
		Skipped int `tenon:"-"`
		hidden  int
	}
	v := encoded(t, tagged{A: 1, B: 2, Skipped: 3, hidden: 4})
	if got := v.Type().AttributeNames(); len(got) != 2 || got[0] != "A" || got[1] != "b" {
		t.Errorf("the struct maps to attributes %v, want [A b]", got)
	}
}

// TestConformance_GO021_MalformedTagsAreUsageErrors pins the panics: an
// option that is not optional, and two fields mapped to one attribute.
func TestConformance_GO021_MalformedTagsAreUsageErrors(t *testing.T) {
	conformance.Covers(t, "GO-021")
	type badOption struct {
		A int `tenon:"a,weird"`
	}
	mustPanicUsage(t, "is not optional", func() { gotenon.Encode(badOption{}) })
	type oneName struct {
		A int `tenon:"x"`
		B int `tenon:"x"`
	}
	mustPanicUsage(t, "both map to the attribute", func() { gotenon.Encode(oneName{}) })
}

// TestConformance_GO022_OptionalFieldsCrossBothWays pins the rule's halves:
// a struct encodes every mapped field, optional ones included, except an
// optional tenon.Value holding the zero Value, which is left out; a null
// decodes into an optional field as its zero value, and an absent optional
// attribute leaves the field at its zero value.
func TestConformance_GO022_OptionalFieldsCrossBothWays(t *testing.T) {
	conformance.Covers(t, "GO-022")
	type withOpt struct {
		P int         `tenon:"p,optional"`
		V tenon.Value `tenon:"v,optional"`
	}
	v := encoded(t, withOpt{})
	if got := v.Type().AttributeNames(); len(got) != 1 || got[0] != "p" {
		t.Errorf("the zero optional Value encoded among %v, want [p] alone", got)
	}
	got := decoded[withOpt](t, obj(map[string]tenon.Value{"p": tenon.NullVal(num)}), uns)
	if got.P != 0 || got.V != (tenon.Value{}) {
		t.Errorf("a null and an absence decoded to %+v, want the zero fields", got)
	}
}

// TestConformance_GO031_NaNAndInfinityFailWithoutPanicking pins the code and
// the calm: a NaN and an infinity, float64 or big.Float, fail with
// encode.not_a_number and nothing panics.
func TestConformance_GO031_NaNAndInfinityFailWithoutPanicking(t *testing.T) {
	conformance.Covers(t, "GO-031")
	wantEncodeFailure(t, "a NaN", math.NaN(), wantDiag{tenon.CodeEncodeNotANumber, "."})
	wantEncodeFailure(t, "an infinity", math.Inf(1), wantDiag{tenon.CodeEncodeNotANumber, "."})
	wantEncodeFailure(t, "a big.Float infinity", new(big.Float).SetInf(true), wantDiag{tenon.CodeEncodeNotANumber, "."})
}

// TestConformance_GO033_StringsCrossNormalized pins both halves: a Go string
// encodes as the String of its text in Normalization Form C, and text that
// is not well-formed UTF-8 fails with string.invalid_utf8.
func TestConformance_GO033_StringsCrossNormalized(t *testing.T) {
	conformance.Covers(t, "GO-033")
	wantValue(t, `Encode("cafe" with a combining acute)`, encoded(t, "cafe\U00000301"), s("caf\U000000e9"))
	wantEncodeFailure(t, "ill-formed text", "a\xffb", wantDiag{tenon.CodeStringInvalidUTF8, "."})
}

// TestConformance_GO042_TheBoundaryFailsWhereThePartIs pins what decoding
// refuses and where: a part that is not known or carries a mark fails at
// its own path, and an error value fails the decoding with its own
// diagnostics.
func TestConformance_GO042_TheBoundaryFailsWhereThePartIs(t *testing.T) {
	conformance.Covers(t, "GO-042")
	type holder struct {
		A int `tenon:"a"`
	}
	wantDecodeFailures[holder](t, "an unknown part, located",
		obj(map[string]tenon.Value{"a": tenon.Unknown(num)}), uns,
		wantDiag{tenon.CodeDecodeNotKnown, ".a"})
	wantDecodeFailures[int](t, "a marked value",
		tenon.WithMarks(n(1), stamp{id: "m"}), uns,
		wantDiag{tenon.CodeDecodeMarked, "."})
	wantDecodeFailures[int](t, "an error value, its own diagnostics",
		tenon.ErrorVal(tenon.Diagnostic{Code: "app.x", Message: "m"}), uns,
		wantDiag{"app.x", "."})
}

// TestConformance_GO050_TheCodesOfTheBoundary produces each code the section
// names, one row a code.
func TestConformance_GO050_TheCodesOfTheBoundary(t *testing.T) {
	conformance.Covers(t, "GO-050")
	wantDecodeFailures[[2]int](t, "decode.length_mismatch",
		tenon.ListVal(num, n(1)), uns, wantDiag{tenon.CodeDecodeLengthMismatch, "."})
	wantDecodeFailures[moment](t, "decode.unmarshal_failed",
		s("not a time"), uns, wantDiag{tenon.CodeDecodeUnmarshalFailed, "."})
	wantDecodeFailures[int](t, "decode.marked",
		tenon.WithMarks(n(1), stamp{id: "m"}), uns, wantDiag{tenon.CodeDecodeMarked, "."})
	wantDecodeFailures[int](t, "decode.not_known",
		tenon.Unknown(num), uns, wantDiag{tenon.CodeDecodeNotKnown, "."})
	wantDecodeFailures[int](t, "decode.null",
		tenon.NullVal(num), uns, wantDiag{tenon.CodeDecodeNull, "."})
	wantDecodeFailures[int8](t, "decode.out_of_range",
		n(1000), uns, wantDiag{tenon.CodeDecodeOutOfRange, "."})
	wantEncodeFailure(t, "encode.inexact", big.NewRat(1, 3), wantDiag{tenon.CodeEncodeInexact, "."})
	wantEncodeFailure(t, "encode.marshal_failed", moment{}, wantDiag{tenon.CodeEncodeMarshalFailed, "."})
	wantEncodeFailure(t, "encode.not_a_number", math.NaN(), wantDiag{tenon.CodeEncodeNotANumber, "."})
	wantEncodeFailure(t, "encode.untyped_nil", []any{nil}, wantDiag{tenon.CodeEncodeUntypedNil, ".[0]"})
}
