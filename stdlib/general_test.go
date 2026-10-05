package stdlib_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// secret is a redacting mark.
type secret struct{}

func (secret) MarkID() string                 { return "secret" }
func (secret) Propagation() tenon.Propagation { return tenon.Propagate }
func (secret) Redacting() bool                { return true }

// stays is a mark that stays on the value it is attached to.
type stays struct{}

func (stays) MarkID() string                 { return "stays" }
func (stays) Propagation() tenon.Propagation { return tenon.Isolate }
func (stays) Redacting() bool                { return false }

// call calls f under the safe policy.
func call(f tenon.Function, args ...tenon.Value) tenon.Value {
	return tenon.Call(f, args, tenon.Safe)
}

// notNull reports whether v's null check is settled false.
func notNull(v tenon.Value) bool {
	n := tenon.IsNull(v)
	return n.IsKnown() && !n.AsBool()
}

func TestConformance_LN083_AssertNotNull(t *testing.T) {
	conformance.Covers(t, "LN-083")
	f := stdlib.AssertNotNullFunc
	str := tenon.StringType()

	// Any value but null is the answer as it is.
	for _, v := range []tenon.Value{
		tenon.String("x"),
		tenon.NumberFromInt(1),
		tenon.List(str, tenon.String("a")),
		tenon.Tuple(tenon.Null(str), tenon.NumberFromInt(1)),
	} {
		if got := call(f, v); !got.Equal(v) {
			t.Errorf("AssertNotNull(%v) = %v, want the argument", v, got)
		}
	}

	// A null fails at the call, a pending value known to be null among them.
	for _, v := range []tenon.Value{tenon.Null(str), tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())} {
		got := call(f, v)
		if !got.IsError() {
			t.Errorf("AssertNotNull(%v) = %v, want a failure", v, got)
			continue
		}
		if d := got.Diagnostics(); len(d) != 1 || d[0].Code != tenon.CodeOperationNullOperand || !d[0].Path.Equal(tenon.Path{}.Index(tenon.NumberFromInt(0))) {
			t.Errorf("AssertNotNull(%v) failed with %+v, want %s at [0]", v, d, tenon.CodeOperationNullOperand)
		}
	}

	// What is not known yet answers with itself, not null, of its type or
	// its constraint.
	got := call(f, tenon.Unknown(str))
	if got.IsKnown() || !got.Type().Equal(str) || !notNull(got) {
		t.Errorf("AssertNotNull(unknown) = %v, want the unknown String, not null", got)
	}
	prefixed := tenon.Narrow(tenon.Unknown(str), tenon.StringPrefix("ab"))
	if got := call(f, prefixed); got.Range().StringPrefix() != prefixed.Range().StringPrefix() || !notNull(got) {
		t.Errorf("AssertNotNull(%v) = %v, want its prefix kept and null excluded", prefixed, got)
	}
	pending := tenon.Pending(tenon.ListOf(tenon.Any()))
	got = call(f, pending)
	if !got.IsPending() || !got.Constraint().Equal(pending.Constraint()) || !notNull(got) {
		t.Errorf("AssertNotNull(%v) = %v, want it pending, not null", pending, got)
	}
	rc, err := tenon.ResultConstraint(f, []tenon.Value{tenon.Unknown(str)}, tenon.Safe)
	if err != nil || !rc.Equal(tenon.Exactly(str)) {
		t.Errorf("the result constraint of AssertNotNull(unknown string) is %v (%v), want exactly(string)", rc, err)
	}
}

func TestConformance_LB020_MarksOfWhatIsRead(t *testing.T) {
	conformance.Covers(t, "LB-020")
	// AssertNotNull reads its whole argument and answers with it, so the
	// answer carries every mark that propagates from it; one that stays with
	// the value it was attached to stays behind, as the call leaves it
	// (FN-016).
	v := tenon.WithMarks(tenon.String("hunter2"), secret{}, stays{})
	got := call(stdlib.AssertNotNullFunc, v)
	if want := tenon.WithMarks(tenon.String("hunter2"), secret{}); !got.Equal(want) {
		t.Errorf("AssertNotNull(%v) = %v, want %v", v, got, want)
	}
	// A redacted null fails without showing that it was null.
	got = call(stdlib.AssertNotNullFunc, tenon.WithMarks(tenon.Null(tenon.StringType()), secret{}))
	if !got.IsError() || !tenon.HasMark(got, secret{}) {
		t.Fatalf("AssertNotNull(a redacted null) = %v, want a failure carrying the mark", got)
	}
	if msg := got.Diagnostics()[0].Message; strings.Contains(msg, "null,") || !strings.Contains(msg, `redacted("secret")`) {
		t.Errorf("AssertNotNull(a redacted null) says %q, want the placeholder in place of null", msg)
	}
}

func TestConformance_LB003_FailuresCarryCodes(t *testing.T) {
	conformance.Covers(t, "LB-003")
	// Every failure the library gives carries a code the specification
	// defines; function.failed is for functions a host defines.
	for name, f := range library {
		args := make([]tenon.Value, len(f.Params()))
		for i := range args {
			args[i] = tenon.Null(tenon.StringType())
		}
		got := call(f, args...)
		if !got.IsError() {
			continue
		}
		for _, d := range got.Diagnostics() {
			if d.Code == tenon.CodeFunctionFailed || !strings.Contains(string(d.Code), ".") {
				t.Errorf("%s failed with %+v, want a code of the specification", name, d)
			}
		}
	}
}

func TestConformance_LB010_UnknownAnswersSayTheLeast(t *testing.T) {
	conformance.Covers(t, "LB-010")
	// What a rule states of an unknown answer is the least it says: every
	// function declaring its result never null answers an argument not yet
	// known with a value whose null check is settled.
	for name, f := range library {
		if !f.NotNull() {
			continue
		}
		args := make([]tenon.Value, len(f.Params()))
		for i := range args {
			args[i] = tenon.Unknown(tenon.StringType())
		}
		if got := call(f, args...); !got.IsError() && !notNull(got) {
			t.Errorf("%s answered unknown arguments with %v, which may be null", name, got)
		}
	}
}

func TestConformance_LN011_Equality(t *testing.T) {
	conformance.Covers(t, "LN-011")
	str, num := tenon.StringType(), tenon.NumberType()
	untyped := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	tests := []struct {
		a, b tenon.Value
		want tenon.Value
	}{
		// A language's untyped null against a null of any type, a value,
		// itself, and what is not known yet.
		{untyped, tenon.Null(str), tenon.Bool(true)},
		{tenon.Null(num), untyped, tenon.Bool(true)},
		{untyped, tenon.String("x"), tenon.Bool(false)},
		{untyped, untyped, tenon.Bool(true)},
		{untyped, tenon.Unknown(str), tenon.Unknown(tenon.BoolType())},
		{untyped, tenon.Narrow(tenon.Unknown(str), tenon.NotNull()), tenon.Bool(false)},
		// A pending value that is not null resolves as well, and says no
		// more than it did.
		{tenon.Pending(tenon.Any()), tenon.String("x"), tenon.Unknown(tenon.BoolType())},
		// Typed values compare as Equals compares them.
		{tenon.Null(str), tenon.Null(num), tenon.Bool(false)},
		{tenon.NumberFromInt(1), tenon.String("1"), tenon.Bool(false)},
		{tenon.NumberFromText("1.50"), tenon.NumberFromText("1.5"), tenon.Bool(true)},
	}
	for _, tt := range tests {
		got := call(stdlib.EqualFunc, tt.a, tt.b)
		if !tenon.Identical(got, tt.want) && !(got.IsResolved() && tt.want.IsResolved() && !got.IsKnown() && !tt.want.IsKnown() && got.Type().Equal(tt.want.Type())) {
			t.Errorf("Equal(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
		if got, want := call(stdlib.NotEqualFunc, tt.a, tt.b), tenon.Not(got); !tenon.Identical(got, want) {
			t.Errorf("NotEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, want)
		}
	}
	// Typed operands answer as Equals answers them, marks and all.
	a, b := tenon.WithMarks(tenon.String("x"), secret{}), tenon.String("x")
	if got, want := call(stdlib.EqualFunc, a, b), tenon.Equals(a, b); !tenon.Identical(got, want) {
		t.Errorf("Equal(%v, %v) = %v, want %v", a, b, got, want)
	}
	if got := call(stdlib.EqualFunc, tenon.WithMarks(untyped, secret{}), tenon.Null(str)); !got.Equal(tenon.WithMarks(tenon.Bool(true), secret{})) {
		t.Errorf("Equal(a redacted untyped null, null) = %v, want true carrying the mark", got)
	}
}
