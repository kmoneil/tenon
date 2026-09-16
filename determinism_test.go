package tenon_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/conformance"
	"github.com/kmoneil/tenon/conformance/values"
)

// TestCanonicalOutputs emits what the value system writes that must not vary
// from one run to the next, over the generator's values: display forms,
// encodings and JSON projections, the canonical order, and the results of
// equality, conversion and diff for every pair or constraint. make determinism
// runs the tests twice, in different orders and on different numbers of
// processors, and compares what they emit. Without TENON_EMIT_DIR this test
// computes the outputs and writes nothing.
func TestCanonicalOutputs(t *testing.T) {
	all := values.All()
	var display, encoded, projected strings.Builder
	for _, v := range all {
		display.WriteString(v.String() + "\n")
		if b, failure, ok := tenon.Serialize(v); ok {
			fmt.Fprintf(&encoded, "%x\n", b)
		} else {
			encoded.WriteString(failure.String() + "\n")
		}
		if b, failure, ok := tenon.ProjectJSON(v); ok {
			projected.WriteString(string(b) + "\n")
		} else {
			projected.WriteString(failure.String() + "\n")
		}
	}

	sorted := slices.Clone(values.Orderable())
	slices.SortStableFunc(sorted, tenon.CanonicalCompare)
	var order strings.Builder
	for _, v := range sorted {
		order.WriteString(v.String() + "\n")
	}

	var equal, diffs strings.Builder
	for _, a := range all {
		for _, b := range all {
			equal.WriteString(tenon.Equals(a, b).String() + "\n")
			diffs.WriteString(tenon.Diff(a, b).String() + "--\n")
		}
	}

	str, num := tenon.StringType(), tenon.NumberType()
	constraints := []tenon.Constraint{
		tenon.Any(), tenon.Exactly(str), tenon.Exactly(num), tenon.ListOf(tenon.Any()), tenon.SetOf(tenon.Exactly(str)),
		tenon.MapOf(tenon.Any()), tenon.OneOf(tenon.Exactly(tenon.BoolType()), tenon.Exactly(str)),
		tenon.ObjectWith(map[string]tenon.Field{"a": {Constraint: tenon.Any()}}, false),
	}
	var converted strings.Builder
	for _, v := range all {
		for _, c := range constraints {
			for _, p := range []tenon.Policy{tenon.Safe, tenon.Unsafe} {
				converted.WriteString(tenon.Convert(v, c, p).String() + "\n")
			}
		}
	}

	for name, text := range map[string]string{
		"display.txt": display.String(), "encoded.txt": encoded.String(), "projected.txt": projected.String(),
		"order.txt": order.String(), "equal.txt": equal.String(), "diffs.txt": diffs.String(),
		"converted.txt": converted.String(),
	} {
		conformance.Emit(t, name, []byte(text))
	}
}
