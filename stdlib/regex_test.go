package stdlib_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// regex calls Regex with the pattern and the string.
func regex(pattern, s string) tenon.Value {
	return call(stdlib.RegexFunc, tenon.String(pattern), tenon.String(s))
}

func TestConformance_LR004_Matching(t *testing.T) {
	conformance.Covers(t, "LR-004")
	for _, tt := range []struct{ pattern, s, want string }{
		{"a|ab", "ab", "a"},
		{"b+", "abbbc", "bbb"},
		{"", "abc", ""},
		// Over scalar values: a dot takes the q and leaves its accent.
		{"^.", "q\U00000301", "q"},
		{`\d+`, "x42y7", "42"},
	} {
		if got := regex(tt.pattern, tt.s); got.IsError() || got.AsString() != tt.want {
			t.Errorf("Regex(%q, %+q) = %v, want %+q", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestConformance_LR005_Shapes(t *testing.T) {
	conformance.Covers(t, "LR-005")
	if got := regex(`(\w+)-(\d+)`, "web-42"); !got.Equal(tenon.Tuple(tenon.String("web"), tenon.String("42"))) {
		t.Errorf("Regex of unnamed groups = %v", got)
	}
	got := regex(`(?P<name>\w+)-(?<n>\d+)`, "web-42")
	if !got.Equal(tenon.Object(map[string]tenon.Value{"name": tenon.String("web"), "n": tenon.String("42")})) {
		t.Errorf("Regex of named groups = %v", got)
	}
	// A group that took no part is null.
	got = regex(`(?P<x>a)|(?P<y>b)`, "b")
	if !got.Equal(tenon.Object(map[string]tenon.Value{"x": tenon.Null(tenon.StringType()), "y": tenon.String("b")})) {
		t.Errorf("Regex of an alternation of groups = %v, want x null", got)
	}
	failsWith(t, "Regex of mixed groups", regex(`(?P<x>a)(b)`, "ab"), tenon.CodeRegexMixedGroups, at(0))
	failsWith(t, "Regex naming a group twice", regex(`(?P<x>a)(?P<x>b)`, "ab"), tenon.CodeRegexDuplicateGroup, at(0))
	failsWith(t, "Regex of no pattern", regex(`(`, "ab"), tenon.CodeRegexInvalidSyntax, at(0))
}

func TestConformance_LR006_NoMatch(t *testing.T) {
	conformance.Covers(t, "LR-006")
	failsWith(t, "Regex(x, abc)", regex("x", "abc"), tenon.CodeRegexNoMatch, at(1))
}

func TestConformance_LR007_RegexAll(t *testing.T) {
	conformance.Covers(t, "LR-007")
	for _, tt := range []struct {
		pattern, s string
		want       tenon.Value
	}{
		{`\d+`, "a1b22c333", strs("1", "22", "333")},
		{"a*", "baaab", strs("", "aaa", "")},
		{"", "abc", strs("", "", "", "")},
		{"x", "abc", strs()},
	} {
		if got := call(stdlib.RegexAllFunc, tenon.String(tt.pattern), tenon.String(tt.s)); !got.Equal(tt.want) {
			t.Errorf("RegexAll(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
	got := call(stdlib.RegexAllFunc, tenon.String(`(\w)=(\d)`), tenon.String("a=1 b=2"))
	want := tenon.List(tenon.TupleType(tenon.StringType(), tenon.StringType()),
		tenon.Tuple(tenon.String("a"), tenon.String("1")), tenon.Tuple(tenon.String("b"), tenon.String("2")))
	if !got.Equal(want) {
		t.Errorf("RegexAll of groups = %v, want %v", got, want)
	}
}

func TestConformance_LR008_PatternNotKnown(t *testing.T) {
	conformance.Covers(t, "LR-008")
	str := tenon.Unknown(tenon.StringType())
	if got := call(stdlib.RegexFunc, str, tenon.String("x")); !got.IsPending() || !notNull(got) {
		t.Errorf("Regex(unknown, x) = %v, want pending, not null", got)
	}
	if got := call(stdlib.RegexAllFunc, str, tenon.String("x")); !got.IsPending() || got.Constraint().Kind() != tenon.ConstraintListOf {
		t.Errorf("RegexAll(unknown, x) = %v, want a pending list", got)
	}
	// A pattern known with a string not known yet: the answer's type, and
	// what the pattern settles fails now.
	got := call(stdlib.RegexFunc, tenon.String(`(\w+)-(\d+)`), str)
	if got.IsPending() || !got.Type().Equal(tenon.TupleType(tenon.StringType(), tenon.StringType())) || !notNull(got) {
		t.Errorf("Regex(groups, unknown) = %v, want an unknown tuple of two strings", got)
	}
	failsWith(t, "Regex of mixed groups and unknown", call(stdlib.RegexFunc, tenon.String(`(?P<x>a)(b)`), str), tenon.CodeRegexMixedGroups, at(0))
}

func TestConformance_LR009_Bound(t *testing.T) {
	conformance.Covers(t, "LR-009")
	// A hundred nested groups each capture a string of n bytes: the answer
	// counts 100 times n+1, and the bound is 64 times the 202 bytes of the
	// pattern and n, and 65,536. At 2,176 bytes it holds 217,700 against
	// 217,728; at 2,177, 217,800 against 217,792.
	nested := strings.Repeat("(", 100) + ".*" + strings.Repeat(")", 100)
	if got := call(stdlib.RegexFunc, tenon.String(nested), tenon.String(strings.Repeat("a", 2176))); got.IsError() {
		t.Errorf("Regex under the bound = %v", got)
	}
	failsWith(t, "Regex past the bound", call(stdlib.RegexFunc, tenon.String(nested), tenon.String(strings.Repeat("a", 2177))), tenon.CodeFunctionTooLarge, at(0))
	// A hundred empty groups match at each of n+1 positions, a hundred
	// captures of no bytes each: 100 times n+1 again, against 64 times the
	// 200 bytes of the pattern and n, and 65,536.
	empty := strings.Repeat("()", 100)
	if got := call(stdlib.RegexAllFunc, tenon.String(empty), tenon.String(strings.Repeat("a", 2173))); got.IsError() || got.Len() != 2174 {
		t.Errorf("RegexAll under the bound = %v", got)
	}
	failsWith(t, "RegexAll past the bound", call(stdlib.RegexAllFunc, tenon.String(empty), tenon.String(strings.Repeat("a", 2174))), tenon.CodeFunctionTooLarge, at(0))
	// Matches of no group count their bytes and one more: the empty
	// pattern over a long string stays far under.
	if got := call(stdlib.RegexAllFunc, tenon.String(""), tenon.String(strings.Repeat("a", 100000))); got.IsError() {
		t.Errorf("RegexAll of the empty pattern = %v", got)
	}
}
