package stdlib_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

func TestConformance_LS024_Join(t *testing.T) {
	conformance.Covers(t, "LS-024")
	for _, tt := range []struct {
		sep   string
		lists []tenon.Value
		want  string
	}{
		{",", []tenon.Value{strs("a", "b"), strs("c")}, "a,b,c"},
		{",", []tenon.Value{strs(), strs("a"), strs()}, "a"},
		{"-", []tenon.Value{strs()}, ""},
		// The answer is the string value made: the separator, a combining
		// acute, composes with the e before it.
		{"\U00000301", []tenon.Value{strs("e", "x")}, "\U000000E9x"},
	} {
		args := append([]tenon.Value{tenon.String(tt.sep)}, tt.lists...)
		if got := call(stdlib.JoinFunc, args...); got.AsString() != tt.want {
			t.Errorf("Join(%v) = %v, want %+q", args, got, tt.want)
		}
	}
	// A null element fails at it, whatever else is not known yet.
	withNull := tenon.List(tenon.StringType(), tenon.String("a"), tenon.Null(tenon.StringType()), tenon.Unknown(tenon.StringType()))
	failsWith(t, "Join(\",\", [a], [a, null, unknown])", call(stdlib.JoinFunc, tenon.String(","), strs("a"), withNull), tenon.CodeOperationNullOperand, at(2).Index(tenon.NumberFromInt(1)))
	// No list: the call's arity.
	if got := call(stdlib.JoinFunc, tenon.String(",")); !got.IsError() || got.Diagnostics()[0].Code != tenon.CodeFunctionArity {
		t.Errorf("Join(\",\") = %v, want %s", got, tenon.CodeFunctionArity)
	}
}

func TestConformance_LS025_JoinNotKnown(t *testing.T) {
	conformance.Covers(t, "LS-025")
	partly := tenon.List(tenon.StringType(), tenon.String("x"), prefixed("ab-"))
	if got := call(stdlib.JoinFunc, tenon.String(","), partly); !promises(got, "x,ab-") {
		t.Errorf("Join(\",\", [x, unknown beginning ab-]) = %v, want an unknown beginning x,ab-", got)
	}
	// A list not known yet may be empty: no separator before it.
	if got := call(stdlib.JoinFunc, tenon.String(","), strs("a", "b"), tenon.Unknown(tenon.ListType(tenon.StringType()))); !promises(got, "a,b") {
		t.Errorf("Join(\",\", [a, b], unknown) = %v, want an unknown beginning a,b", got)
	}
	// A separator not known yet is not read for one element.
	if got := call(stdlib.JoinFunc, tenon.Unknown(tenon.StringType()), strs("only")); !got.Equal(tenon.String("only")) {
		t.Errorf("Join(unknown, [only]) = %v, want only", got)
	}
	// For random lists ending in an element not known yet, the answer for
	// the element known begins with what was promised.
	rng := rand.New(rand.NewPCG(20261005, 5))
	for range 3000 {
		known := randomText(rng, 2)
		p, rest := randomText(rng, 4), randomText(rng, 3)
		sep := tenon.String(randomText(rng, 1))
		promised := call(stdlib.JoinFunc, sep, tenon.List(tenon.StringType(), tenon.String(known), prefixed(p)))
		got := call(stdlib.JoinFunc, sep, strs(known, p+rest))
		if !promised.IsKnown() && !strings.HasPrefix(got.AsString(), promised.Range().StringPrefix()) {
			t.Fatalf("Join(%v, [%+q, %+q]) = %v, not beginning with %v", sep, known, p+rest, got, promised)
		}
	}
}

func TestConformance_LS026_Sort(t *testing.T) {
	conformance.Covers(t, "LS-026")
	for _, tt := range []struct{ in, want tenon.Value }{
		{strs("b", "a", "B", "\U000000E9", "z"), strs("B", "a", "b", "z", "\U000000E9")},
		{strs("10", "9", "1"), strs("1", "10", "9")},
		{strs("\U00010000", "\U0000FF61"), strs("\U0000FF61", "\U00010000")},
		{strs("b", "a", "b"), strs("a", "b", "b")},
		{strs(), strs()},
	} {
		if got := call(stdlib.SortFunc, tt.in); !got.Equal(tt.want) {
			t.Errorf("Sort(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	withNull := tenon.List(tenon.StringType(), tenon.String("a"), tenon.Null(tenon.StringType()), tenon.Unknown(tenon.StringType()))
	failsWith(t, "Sort([a, null, unknown])", call(stdlib.SortFunc, withNull), tenon.CodeOperationNullOperand, at(0).Index(tenon.NumberFromInt(1)))
}

func TestConformance_LS027_SortNotKnown(t *testing.T) {
	conformance.Covers(t, "LS-027")
	got := call(stdlib.SortFunc, tenon.List(tenon.StringType(), tenon.String("a"), tenon.Unknown(tenon.StringType())))
	if !got.HasMembers() || got.Len() != 2 {
		t.Fatalf("Sort([a, unknown]) = %v, want a list of two", got)
	}
	for _, e := range got.Elements() {
		if e.IsKnown() || !notNull(e) {
			t.Errorf("Sort([a, unknown]) = %v, want each element unknown and not null", got)
		}
	}
	one := prefixed("ab-")
	if got := call(stdlib.SortFunc, tenon.List(tenon.StringType(), one)); got.Len() != 1 || !promises(got.Index(0), "ab-") {
		t.Errorf("Sort([unknown beginning ab-]) = %v, want itself, not null", got)
	}
	unknownList := tenon.Narrow(tenon.Unknown(tenon.ListType(tenon.StringType())), tenon.LengthMin(2), tenon.LengthMax(4))
	if got := call(stdlib.SortFunc, unknownList); !lengthBetween(got, 2, 4) {
		t.Errorf("Sort(unknown of 2 to 4) = %v, want an unknown list of 2 to 4", got)
	}
}
