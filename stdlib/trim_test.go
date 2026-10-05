package stdlib_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

func TestConformance_LS019_Trim(t *testing.T) {
	conformance.Covers(t, "LS-019")
	thumb := "\U0001F44D"
	for _, tt := range []struct{ str, cutset, want string }{
		{"xyhixy", "yx", "hi"},
		{"abc", "", "abc"},
		{"  a b  ", " ", "a b"},
		{"aaa", "a", ""},
		// A run ends and begins at a cut position: the thumb with its skin
		// tone is one cluster, and q with an acute another.
		{thumb + "\U0001F3FDx" + thumb, thumb, thumb + "\U0001F3FDx"},
		{"q\U00000301xq", "q", "q\U00000301x"},
		{"xq\U00000301", "\U00000301", "xq\U00000301"},
		{"\U0001F1FA\U0001F1F8\U0001F1EC\U0001F1E7", "\U0001F1FA", "\U0001F1FA\U0001F1F8\U0001F1EC\U0001F1E7"},
		// The cutset is a set of code points: CR and LF each.
		{"\r\na\n\r", "\r\n", "a"},
	} {
		if got := call(stdlib.TrimFunc, tenon.String(tt.str), tenon.String(tt.cutset)); got.AsString() != tenon.String(tt.want).AsString() {
			t.Errorf("Trim(%+q, %+q) = %+q, want %+q", tt.str, tt.cutset, got.AsString(), tt.want)
		}
	}
	// The range records xxabc de, and the answer what it settles, abc de,
	// cut again where a combining mark could follow.
	if got := call(stdlib.TrimFunc, prefixed("xxabc def"), tenon.String("x ")); !promises(got, "abc de") {
		t.Errorf("Trim(unknown beginning xxabc def, x and space) = %v, want an unknown beginning abc de", got)
	}
	promisesHoldFor(t, stdlib.TrimFunc, 0, func(rng *rand.Rand) []tenon.Value {
		return []tenon.Value{{}, tenon.String(randomText(rng, 2))}
	})
}

func TestConformance_LS020_TrimSpace(t *testing.T) {
	conformance.Covers(t, "LS-020")
	for _, tt := range []struct{ in, want string }{
		{"  hello  ", "hello"},
		{"\t\r\n x \U000000A0\U00003000", "x"},
		// Not White_Space: the byte order mark and the zero width space.
		{"\U0000FEFFx\U0000200B", "\U0000FEFFx\U0000200B"},
		// A space carrying a combining mark stays, at either end.
		{" \U00000301x", " \U00000301x"},
		{"x \U00000301", "x \U00000301"},
		{"   ", ""},
	} {
		if got := call(stdlib.TrimSpaceFunc, tenon.String(tt.in)); got.AsString() != tenon.String(tt.want).AsString() {
			t.Errorf("TrimSpace(%+q) = %+q, want %+q", tt.in, got.AsString(), tt.want)
		}
	}
	if got := call(stdlib.TrimSpaceFunc, prefixed("  ab cd")); !promises(got, "ab c") {
		t.Errorf("TrimSpace(unknown beginning   ab cd) = %v, want an unknown beginning ab c", got)
	}
	promisesHoldFor(t, stdlib.TrimSpaceFunc, 0, func(*rand.Rand) []tenon.Value { return []tenon.Value{{}} })
}

func TestConformance_LS021_Chomp(t *testing.T) {
	conformance.Covers(t, "LS-021")
	for _, tt := range []struct{ in, want string }{
		{"a\n", "a"}, {"a\r\n", "a"}, {"a\r", "a"}, {"a\n\r", "a"}, {"a\n\n\n", "a"},
		{"a\r\n\r\n", "a"}, {"\n", ""}, {"a\nb\n", "a\nb"}, {"a\n ", "a\n "},
		// Other line terminators stay.
		{"a\U00002028", "a\U00002028"}, {"a\U00000085", "a\U00000085"}, {"a\v\f", "a\v\f"},
	} {
		if got := call(stdlib.ChompFunc, tenon.String(tt.in)); got.AsString() != tt.want {
			t.Errorf("Chomp(%+q) = %+q, want %+q", tt.in, got.AsString(), tt.want)
		}
	}
	if got := call(stdlib.ChompFunc, prefixed("ab\r\n")); !promises(got, "ab") {
		t.Errorf("Chomp(unknown beginning ab CR LF) = %v, want an unknown beginning a", got)
	}
	promisesHoldFor(t, stdlib.ChompFunc, 0, func(*rand.Rand) []tenon.Value { return []tenon.Value{{}} })
}

func TestConformance_LS022_Indent(t *testing.T) {
	conformance.Covers(t, "LS-022")
	for _, tt := range []struct {
		spaces   string
		in, want string
	}{
		{"2", "a\nb", "a\n  b"},
		{"2", "a\nb\n", "a\n  b\n  "},
		{"2", "\n", "\n  "},
		{"0", "a\nb", "a\nb"},
		// Only LF breaks a line.
		{"2", "a\r\nb", "a\r\n  b"},
		{"2", "a\rb", "a\rb"},
		// A string with no LF is the answer, whatever the number.
		{"1e30", "ab", "ab"},
	} {
		if got := call(stdlib.IndentFunc, num(tt.spaces), tenon.String(tt.in)); got.AsString() != tt.want {
			t.Errorf("Indent(%s, %+q) = %v, want %+q", tt.spaces, tt.in, got, tt.want)
		}
	}
	failsWith(t, "Indent(-1, ab)", call(stdlib.IndentFunc, num("-1"), tenon.String("ab")), tenon.CodeFunctionInvalidArgument, at(0))
	failsWith(t, "Indent(1.5, a LF b)", call(stdlib.IndentFunc, num("1.5"), tenon.String("a\nb")), tenon.CodeFunctionInvalidArgument, at(0))
	// The bound, before anything is made: 2^62 spaces, and a thousand
	// lines a thousand spaces deep.
	failsWith(t, "Indent(2^62, a LF b)", call(stdlib.IndentFunc, num("4611686018427387904"), tenon.String("a\nb")), tenon.CodeFunctionTooLarge, at(0))
	failsWith(t, "Indent(1000, a thousand LF)", call(stdlib.IndentFunc, num("1000"), tenon.String(strings.Repeat("\n", 1000))), tenon.CodeFunctionTooLarge, at(0))
}

func TestConformance_LS023_IndentNotKnown(t *testing.T) {
	conformance.Covers(t, "LS-023")
	if got := call(stdlib.IndentFunc, num("2"), prefixed("a\nb\nc")); !promises(got, "a\n  b\n  c") {
		t.Errorf("Indent(2, unknown beginning a LF b LF c) = %v", got)
	}
	if got := call(stdlib.IndentFunc, unknownNumber, prefixed("a\nb\nc")); !promises(got, "a\n") {
		t.Errorf("Indent(unknown, unknown beginning a LF b LF c) = %v, want an unknown beginning a LF", got)
	}
	if got := call(stdlib.IndentFunc, unknownNumber, tenon.String("ab")); !got.Equal(tenon.String("ab")) {
		t.Errorf("Indent(unknown, ab) = %v, want ab", got)
	}
	failsWith(t, "Indent(-1, unknown)", call(stdlib.IndentFunc, num("-1"), tenon.Unknown(tenon.StringType())), tenon.CodeFunctionInvalidArgument, at(0))
	promisesHoldFor(t, stdlib.IndentFunc, 1, func(rng *rand.Rand) []tenon.Value {
		return []tenon.Value{tenon.NumberFromInt(int64(rng.IntN(4))), {}}
	})
}
