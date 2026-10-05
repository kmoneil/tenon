package stdlib_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// strs returns the list of the strings given.
func strs(ss ...string) tenon.Value {
	vs := make([]tenon.Value, len(ss))
	for i, s := range ss {
		vs[i] = tenon.String(s)
	}
	return tenon.List(tenon.StringType(), vs...)
}

// texts are what the searches' property tests build strings and the texts
// they look for from: ASCII, a combining acute, regional indicators, CR
// and LF, a ZWJ and a pictograph.
var texts = []string{"a", "b", "ab", ",", " ", "\r", "\n", "\U00000301", "\U0001F1FA", "\U0001F1F8", "\U0000200D", "\U0001F468"}

// randomText returns a random text of up to n pieces of texts.
func randomText(rng *rand.Rand, n int) string {
	var b strings.Builder
	for range rng.IntN(n + 1) {
		b.WriteString(texts[rng.IntN(len(texts))])
	}
	return tenon.String(b.String()).AsString()
}

// promisesHoldFor holds f to the promise its answer for a string not known
// yet makes, the string being the argument at index str of what args
// gives: for random prefixes and continuations, the answer for the whole
// is what was answered for the prefix where that was known, begins with
// what it promised where that was a string, and is at least as long as it
// promised where that was a list.
func promisesHoldFor(t *testing.T, f tenon.Function, str int, args func(rng *rand.Rand) []tenon.Value) {
	t.Helper()
	rng := rand.New(rand.NewPCG(20261005, 4))
	for range 5000 {
		p, rest := randomText(rng, 6), randomText(rng, 4)
		given := args(rng)
		withPrefix := append([]tenon.Value(nil), given...)
		withPrefix[str] = prefixed(p)
		whole := append([]tenon.Value(nil), given...)
		whole[str] = tenon.String(p + rest)
		promised, got := call(f, withPrefix...), call(f, whole...)
		switch {
		case got.IsError():
			continue
		case promised.IsKnown():
			if !promised.Equal(got) {
				t.Fatalf("%s(%v) = %v, and %v was answered for the prefix %+q", f.Name(), whole, got, promised, p)
			}
		case got.Type().Kind() == tenon.KindList:
			if int64(got.Len()) < promised.Range().LengthMin() {
				t.Fatalf("%s(%v) = %v, shorter than %d, promised for the prefix %+q", f.Name(), whole, got, promised.Range().LengthMin(), p)
			}
		case !strings.HasPrefix(got.AsString(), promised.Range().StringPrefix()):
			t.Fatalf("%s(%v) = %+q, which does not begin with %+q, promised for the prefix %+q", f.Name(), whole, got.AsString(), promised.Range().StringPrefix(), p)
		}
	}
}

func TestConformance_LS012_Matching(t *testing.T) {
	conformance.Covers(t, "LS-012")
	us, sg, gb := "\U0001F1FA\U0001F1F8", "\U0001F1F8\U0001F1EC", "\U0001F1EC\U0001F1E7"
	for _, tt := range []struct {
		sep, str string
		want     tenon.Value
	}{
		// Leftmost, without overlap.
		{"aa", "aaaaa", strs("", "", "a")},
		{",", "a,b,,c", strs("a", "b", "", "c")},
		// Every position of ASCII text is a cut position, inside CR LF too.
		{"\n", "a\r\nb\r\n", strs("a\r", "b\r", "")},
		// No match splits a cluster: the Singapore flag is not in the US
		// flag followed by the British one, and an e with an acute is no e.
		{sg, us + gb, strs(us + gb)},
		{"\U00000119", "a\U00000119\U00000301b", strs("a\U00000119\U00000301b")},
		{"q", "q\U00000301x", strs("q\U00000301x")},
	} {
		if got := call(stdlib.SplitFunc, tenon.String(tt.sep), tenon.String(tt.str)); !got.Equal(tt.want) {
			t.Errorf("Split(%+q, %+q) = %v, want %v", tt.sep, tt.str, got, tt.want)
		}
	}
}

func TestConformance_LS013_Split(t *testing.T) {
	conformance.Covers(t, "LS-013")
	for _, tt := range []struct {
		sep, str string
		want     tenon.Value
	}{
		{",", "", strs("")},
		{",", "abc", strs("abc")},
		// An empty separator cuts clusters, and the empty string into none.
		{"", "", strs()},
		{"", "a\r\nb", strs("a", "\r\n", "b")},
		{"", "\U00000915\U0000094D\U00000937", strs("\U00000915\U0000094D", "\U00000937")},
		{"", "\U0001F1FA\U0001F1F8", strs("\U0001F1FA\U0001F1F8")},
	} {
		if got := call(stdlib.SplitFunc, tenon.String(tt.sep), tenon.String(tt.str)); !got.Equal(tt.want) {
			t.Errorf("Split(%+q, %+q) = %v, want %v", tt.sep, tt.str, got, tt.want)
		}
	}
	if !stdlib.SplitFunc.NotNull() {
		t.Error("Split does not declare its result never null")
	}
}

func TestConformance_LS014_SplitNotKnown(t *testing.T) {
	conformance.Covers(t, "LS-014")
	// The range records a,b,c, of a,b,c,d, a combining mark able to follow
	// the d: two separators it settles, and a third a mark could join to
	// what follows. At least three elements.
	if got := call(stdlib.SplitFunc, tenon.String(","), prefixed("a,b,c,d")); got.IsKnown() || got.Range().LengthMin() != 3 {
		t.Errorf("Split(\",\", unknown beginning a,b,c,d) = %v, want at least 3 elements", got)
	}
	// An empty separator: as many elements as the string's length.
	bounded := tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix("ab-"), tenon.LengthMax(5))
	got := call(stdlib.SplitFunc, tenon.String(""), bounded)
	if hi, ok := got.Range().LengthMax(); got.IsKnown() || got.Range().LengthMin() != 3 || !ok || hi != 5 {
		t.Errorf("Split(\"\", unknown of 3 to 5 clusters) = %v, want 3 to 5 elements", got)
	}
	promisesHoldFor(t, stdlib.SplitFunc, 1, func(rng *rand.Rand) []tenon.Value {
		return []tenon.Value{tenon.String(texts[rng.IntN(len(texts))]), tenon.Value{}}
	})
}

func TestConformance_LS015_Replace(t *testing.T) {
	conformance.Covers(t, "LS-015")
	us, sg, gb := "\U0001F1FA\U0001F1F8", "\U0001F1F8\U0001F1EC", "\U0001F1EC\U0001F1E7"
	for _, tt := range []struct{ str, sub, rep, want string }{
		{"aaa", "aa", "b", "ba"},
		{"banana", "/a/", "o", "banana"},
		{"abc", "", "-", "-a-b-c-"},
		{"", "", "-", "-"},
		{"\U00000915\U0000094D\U00000937", "", "-", "-\U00000915\U0000094D-\U00000937-"},
		{us + gb, sg, "", us + gb},
		{"a\r\nb", "\r", "", "a\nb"},
		// The answer is the string value made: e, then a combining acute
		// put in place of the hyphen, compose.
		{"e-y", "-", "\U00000301", "\U000000E9y"},
		{"q\U00000301", "q", "e", "q\U00000301"},
	} {
		if got := call(stdlib.ReplaceFunc, tenon.String(tt.str), tenon.String(tt.sub), tenon.String(tt.rep)); got.AsString() != tenon.String(tt.want).AsString() {
			t.Errorf("Replace(%+q, %+q, %+q) = %v, want %+q", tt.str, tt.sub, tt.rep, got, tt.want)
		}
	}
	// The bound: 64 times the arguments' size and 64 KiB, decided before
	// the answer is made, at the replacement.
	many := tenon.String(strings.Repeat("a", 1000))
	big := tenon.String(strings.Repeat("x", 70000))
	failsWith(t, "Replace(a thousand a, a, seventy thousand x)", call(stdlib.ReplaceFunc, many, tenon.String("a"), big), tenon.CodeFunctionTooLarge, at(2))
	if got := call(stdlib.ReplaceFunc, many, tenon.String("a"), tenon.String("xyz")); got.IsError() || len(got.AsString()) != 3000 {
		t.Errorf("Replace(a thousand a, a, xyz) = %d bytes, want 3000", len(got.AsString()))
	}
}

func TestConformance_LS016_ReplaceNotKnown(t *testing.T) {
	conformance.Covers(t, "LS-016")
	// The prefix's occurrences replaced, up to where one could reach past.
	if got := call(stdlib.ReplaceFunc, prefixed("a-b-c-d-e"), tenon.String("-"), tenon.String("+")); !promises(got, "a+b+c+d") {
		t.Errorf("Replace(unknown beginning a-b-c-d-e, -, +) = %v, want an unknown beginning a+b+c+", got)
	}
	if got := call(stdlib.ReplaceFunc, prefixed("hello wo"), tenon.String("world"), tenon.String("there")); !promises(got, "hello ") {
		t.Errorf("Replace(unknown beginning hello wo, world, there) = %v, want an unknown beginning hello", got)
	}
	// A replacement not known yet: the text before the first occurrence,
	// and the string itself where it holds none, the replacement unread.
	if got := call(stdlib.ReplaceFunc, tenon.String("a-b"), tenon.String("-"), tenon.Unknown(tenon.StringType())); !promises(got, "a") {
		t.Errorf("Replace(a-b, -, unknown) = %v, want an unknown beginning a", got)
	}
	if got := call(stdlib.ReplaceFunc, tenon.String("abc"), tenon.String("-"), tenon.Unknown(tenon.StringType())); !got.Equal(tenon.String("abc")) {
		t.Errorf("Replace(abc, -, unknown) = %v, want abc", got)
	}
	promisesHoldFor(t, stdlib.ReplaceFunc, 0, func(rng *rand.Rand) []tenon.Value {
		return []tenon.Value{{}, tenon.String(texts[rng.IntN(len(texts))]), tenon.String([]string{"", "x", "\U00000301", "ab"}[rng.IntN(4)])}
	})
}

func TestConformance_LS017_TrimPrefix(t *testing.T) {
	conformance.Covers(t, "LS-017")
	for _, tt := range []struct{ str, prefix, want string }{
		{"aaa", "a", "aa"},
		{"abc", "", "abc"},
		{"abc", "x", "abc"},
		{"q\U00000301x", "q", "q\U00000301x"},
		{"\U0001F1FA\U0001F1F8\U0001F1EC\U0001F1E7", "\U0001F1FA", "\U0001F1FA\U0001F1F8\U0001F1EC\U0001F1E7"},
		{"\r\nx", "\r", "\nx"},
	} {
		if got := call(stdlib.TrimPrefixFunc, tenon.String(tt.str), tenon.String(tt.prefix)); got.AsString() != tt.want {
			t.Errorf("TrimPrefix(%+q, %+q) = %+q, want %+q", tt.str, tt.prefix, got.AsString(), tt.want)
		}
	}
	if got := call(stdlib.TrimPrefixFunc, prefixed("v1-alpha"), tenon.String("v1-")); !promises(got, "alph") {
		t.Errorf("TrimPrefix(unknown beginning v1-alpha, v1-) = %v, want an unknown beginning alph", got)
	}
	if got := call(stdlib.TrimPrefixFunc, prefixed("v2-alpha"), tenon.String("v1-")); !promises(got, "v2-alph") {
		t.Errorf("TrimPrefix(unknown beginning v2-alpha, v1-) = %v, want an unknown beginning v2-alph", got)
	}
	promisesHoldFor(t, stdlib.TrimPrefixFunc, 0, func(rng *rand.Rand) []tenon.Value {
		return []tenon.Value{{}, tenon.String(randomText(rng, 2))}
	})
}

func TestConformance_LS018_TrimSuffix(t *testing.T) {
	conformance.Covers(t, "LS-018")
	for _, tt := range []struct{ str, suffix, want string }{
		{"aaa", "a", "aa"},
		{"abc", "", "abc"},
		{"a\r\n", "\n", "a\r"},
		{"xq\U00000301", "\U00000301", "xq\U00000301"},
		{"\U000000E9", "\U00000301", "\U000000E9"},
	} {
		if got := call(stdlib.TrimSuffixFunc, tenon.String(tt.str), tenon.String(tt.suffix)); got.AsString() != tenon.String(tt.want).AsString() {
			t.Errorf("TrimSuffix(%+q, %+q) = %+q, want %+q", tt.str, tt.suffix, got.AsString(), tt.want)
		}
	}
	if got := call(stdlib.TrimSuffixFunc, prefixed("hello world"), tenon.String(".txt")); !promises(got, "hello ") {
		t.Errorf("TrimSuffix(unknown beginning hello world, .txt) = %v, want an unknown beginning hello", got)
	}
	promisesHoldFor(t, stdlib.TrimSuffixFunc, 0, func(rng *rand.Rand) []tenon.Value {
		return []tenon.Value{{}, tenon.String(randomText(rng, 2))}
	})
}
