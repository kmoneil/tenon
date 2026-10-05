package stdlib_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/stdlib"
)

// prefixed returns a string not known yet whose range records the prefix p.
func prefixed(p string) tenon.Value {
	return tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix(p))
}

// promises reports whether the answer got is a string not known yet, not
// null, beginning with p as a range records it: cut back to what no text
// following p can change, as StringPrefix cuts it.
func promises(got tenon.Value, p string) bool {
	return !got.IsKnown() && !got.IsError() && notNull(got) && got.Range().StringPrefix() == prefixed(p).Range().StringPrefix()
}

func TestConformance_LS004_StringResults(t *testing.T) {
	conformance.Covers(t, "LS-004")
	// The answer is the string value of what the rule maps, so it is
	// normalized: i and a combining dot above uppercase to I and the dot,
	// which compose to one code point; J and a combining caron lowercase to
	// the precomposed j with caron.
	for _, tt := range []struct {
		f        tenon.Function
		in, want string
	}{
		{stdlib.UpperFunc, "i\U00000307", "\U00000130"},
		{stdlib.LowerFunc, "J\U0000030C", "\U000001F0"},
	} {
		if got := call(tt.f, tenon.String(tt.in)); !got.Equal(tenon.String(tt.want)) || got.AsString() != tt.want {
			t.Errorf("%s(%+q) = %v, want %+q", tt.f.Name(), tt.in, got, tt.want)
		}
	}
	// The boundary carries the argument's marks to the answer.
	if got := call(stdlib.UpperFunc, tenon.WithMarks(tenon.String("x"), bare("arg"))); !tenon.HasMark(got, bare("arg")) {
		t.Errorf("Upper of a marked string = %v, want its mark", got)
	}
}

func TestConformance_LS005_Upper(t *testing.T) {
	conformance.Covers(t, "LS-005")
	for _, tt := range []struct{ in, want string }{
		{"hello", "HELLO"},
		{"stra\U000000DFe", "STRASSE"},
		{"\U0000FB01sh", "FISH"},
		{"\U000001F0", "J\U0000030C"},
		{"x\U00000345", "X\U00000399"},
		{"", ""},
	} {
		if got := call(stdlib.UpperFunc, tenon.String(tt.in)); got.AsString() != tt.want {
			t.Errorf("Upper(%+q) = %+q, want %+q", tt.in, got.AsString(), tt.want)
		}
	}
	// The range records v1-a of v1-ab, a combining mark able to follow the
	// b, and the answer's promise is the uppercase of that, cut again.
	if got := call(stdlib.UpperFunc, prefixed("v1-ab")); !promises(got, "V1-A") {
		t.Errorf("Upper(unknown beginning v1-ab) = %v, want an unknown beginning V1-", got)
	}
	promisesHold(t, stdlib.UpperFunc)
	if !stdlib.UpperFunc.NotNull() {
		t.Error("Upper does not declare its result never null")
	}
}

func TestConformance_LS006_Lower(t *testing.T) {
	conformance.Covers(t, "LS-006")
	odos := "\U0000039F\U00000394\U0000039F\U000003A3"
	for _, tt := range []struct{ in, want string }{
		{"HELLO", "hello"},
		{odos, "\U000003BF\U000003B4\U000003BF\U000003C2"},
		{odos + " " + odos, "\U000003BF\U000003B4\U000003BF\U000003C2 \U000003BF\U000003B4\U000003BF\U000003C2"},
		{"\U00000130", "i\U00000307"},
	} {
		if got := call(stdlib.LowerFunc, tenon.String(tt.in)); got.AsString() != tt.want {
			t.Errorf("Lower(%+q) = %+q, want %+q", tt.in, got.AsString(), tt.want)
		}
	}
	// A capital sigma at the end of the prefix: what follows decides
	// whether it ends a word, so the promise stops before it.
	if got := call(stdlib.LowerFunc, prefixed(odos)); !promises(got, "\U000003BF\U000003B4\U000003BF") {
		t.Errorf("Lower(unknown beginning %+q) = %v, want an unknown beginning %+q", odos, got, "\U000003BF\U000003B4\U000003BF")
	}
	// One the prefix settles is kept.
	if got := call(stdlib.LowerFunc, prefixed(odos+" X")); !promises(got, "\U000003BF\U000003B4\U000003BF\U000003C2 ") {
		t.Errorf("Lower(unknown beginning %+q) = %v", odos+" X", got)
	}
	promisesHold(t, stdlib.LowerFunc)
}

// promisesHold holds the function f, of one string, to the promise its
// answer for an unknown string makes: for random prefixes and
// continuations, f of the whole begins with what f promised for the prefix
// alone.
func promisesHold(t *testing.T, f tenon.Function) {
	t.Helper()
	alphabet := []string{"\U000003A3", "\U00000391", "A", "a", " ", ".", "'", "_", "\U00000301", "\U00000345", "\U000000AD", "\U000000DF", "1", "i\U00000307"}
	rng := rand.New(rand.NewPCG(20261005, 2))
	word := func() string {
		var b strings.Builder
		for range rng.IntN(5) {
			b.WriteString(alphabet[rng.IntN(len(alphabet))])
		}
		return b.String()
	}
	for range 5000 {
		p, rest := tenon.String(word()).AsString(), word()
		promise := call(f, prefixed(p)).Range().StringPrefix()
		if got := call(f, tenon.String(p+rest)).AsString(); !strings.HasPrefix(got, promise) {
			t.Fatalf("%s(%+q) = %+q, which does not begin with %+q, promised for the prefix %+q", f.Name(), p+rest, got, promise, p)
		}
	}
}

func TestConformance_LS007_Title(t *testing.T) {
	conformance.Covers(t, "LS-007")
	for _, tt := range []struct{ in, want string }{
		{"hello world", "Hello World"},
		{"foo.example.com", "Foo.Example.Com"},
		{"o'neil", "O'Neil"},
		{"don\U00002019t", "Don\U00002019t"},
		{"hello_world", "Hello_world"},
		{"1st place", "1st Place"},
		{"a.b,c-d", "A.B,C-D"},
		{"hELLO", "HELLO"},
		{"hello\U000000A0world", "Hello\U000000A0World"},
		{"\U000000ABbonjour\U000000BB", "\U000000ABbonjour\U000000BB"},
		{"\U000000DFtra\U000000DFe", "Sstra\U000000DFe"},
		{"\U0000FB01sh", "Fish"},
		{"\U000001C6emal", "\U000001C5emal"},
	} {
		if got := call(stdlib.TitleFunc, tenon.String(tt.in)); got.AsString() != tt.want {
			t.Errorf("Title(%+q) = %+q, want %+q", tt.in, got.AsString(), tt.want)
		}
	}
	if got := call(stdlib.TitleFunc, prefixed("ab cd")); !promises(got, "Ab C") {
		t.Errorf("Title(unknown beginning ab cd) = %v, want an unknown beginning Ab ", got)
	}
	promisesHold(t, stdlib.TitleFunc)
}

func TestConformance_LS008_Strlen(t *testing.T) {
	conformance.Covers(t, "LS-008")
	for _, tt := range []struct {
		in   string
		want int64
	}{
		{"", 0}, {"hello", 5}, {"q\U00000301", 1}, {"\r\n", 1},
		{"\U0001F1FA\U0001F1F8\U0001F1EC", 2},
		// A ZWJ after a regional indicator joins no pictograph to it (GB11).
		{"\U0001F1FA\U0000200D\U0001F468", 2},
		{"\U00000915\U0000094D\U00000937", 2},
	} {
		if got := call(stdlib.StrlenFunc, tenon.String(tt.in)); !got.Equal(tenon.NumberFromInt(tt.want)) {
			t.Errorf("Strlen(%+q) = %v, want %d", tt.in, got, tt.want)
		}
	}
	got := call(stdlib.StrlenFunc, tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.StringPrefix("ab-"), tenon.LengthMax(5)))
	lo, _, _ := got.Range().NumberMin()
	hi, _, _ := got.Range().NumberMax()
	if got.IsKnown() || !lo.Equal(num("3")) || !hi.Equal(num("5")) || !notNull(got) {
		t.Errorf("Strlen(unknown beginning ab-, at most 5 long) = %v, want 3 to 5", got)
	}
}

func TestConformance_LS009_Reverse(t *testing.T) {
	conformance.Covers(t, "LS-009")
	for _, tt := range []struct{ in, want string }{
		{"hello", "olleh"},
		{"a\r\nb", "b\r\na"},
		{"e\U00000301x", "xe\U00000301"},
		{"\U0001F1FA\U0001F1F8\U0001F1EC\U0001F1E7", "\U0001F1EC\U0001F1E7\U0001F1FA\U0001F1F8"},
		// Clusters meeting anew compose: a lone acute after e.
		{"\U00000301e", "\U000000E9"},
	} {
		if got := call(stdlib.ReverseFunc, tenon.String(tt.in)); got.AsString() != tenon.String(tt.want).AsString() {
			t.Errorf("Reverse(%+q) = %+q, want %+q", tt.in, got.AsString(), tt.want)
		}
	}
	// So reversing twice need not give the string back: three regional
	// indicators pair afresh.
	three := tenon.String("\U0001F1FA\U0001F1F8\U0001F1EC")
	if twice := call(stdlib.ReverseFunc, call(stdlib.ReverseFunc, three)); twice.Equal(three) {
		t.Errorf("Reverse twice of %v = %v, the string itself", three, twice)
	}
	if got := call(stdlib.ReverseFunc, tenon.Unknown(tenon.StringType())); got.IsKnown() || !notNull(got) {
		t.Errorf("Reverse(unknown) = %v, want unknown and not null", got)
	}
}

func TestConformance_LS010_Substr(t *testing.T) {
	conformance.Covers(t, "LS-010")
	hello := tenon.String("hello")
	for _, tt := range []struct {
		offset, length string
		want           string
	}{
		{"1", "3", "ell"}, {"0", "5", "hello"}, {"2", "100", "llo"},
		{"-3", "2", "ll"}, {"-1", "-1", "o"}, {"-10", "2", "he"},
		{"5", "2", ""}, {"10", "2", ""}, {"1", "-5", "ello"},
		// A length of zero is empty whatever the offset (#217).
		{"0", "0", ""}, {"2", "0", ""}, {"-3", "0", ""}, {"-6", "0", ""},
		// Of any magnitude.
		{"0", "1e30", "hello"}, {"-1e30", "1", "h"}, {"1e30", "1", ""},
	} {
		if got := call(stdlib.SubstrFunc, hello, num(tt.offset), num(tt.length)); got.AsString() != tt.want {
			t.Errorf("Substr(hello, %s, %s) = %v, want %q", tt.offset, tt.length, got, tt.want)
		}
	}
	// By cluster: an e and its acute are one.
	if got := call(stdlib.SubstrFunc, tenon.String("e\U00000301x"), num("0"), num("1")); got.AsString() != "\U000000E9" {
		t.Errorf("Substr(e and acute, x, 0, 1) = %+q", got.AsString())
	}
	failsWith(t, "Substr(hello, 1.5, 2)", call(stdlib.SubstrFunc, hello, num("1.5"), num("2")), tenon.CodeFunctionInvalidArgument, at(1))
	failsWith(t, "Substr(hello, 1, 2.5)", call(stdlib.SubstrFunc, hello, num("1"), num("2.5")), tenon.CodeFunctionInvalidArgument, at(2))
}

func TestConformance_LS011_SubstrNotKnown(t *testing.T) {
	conformance.Covers(t, "LS-011")
	str := tenon.Unknown(tenon.StringType())
	// A length of zero settles it, and a fraction fails, whatever else is
	// not known yet.
	if got := call(stdlib.SubstrFunc, str, unknownNumber, num("0")); !got.Equal(tenon.String("")) {
		t.Errorf("Substr(unknown, unknown, 0) = %v, want the empty string", got)
	}
	failsWith(t, "Substr(unknown, 1.5, unknown)", call(stdlib.SubstrFunc, str, num("1.5"), unknownNumber), tenon.CodeFunctionInvalidArgument, at(1))
	// At most the length, or as far as a negative offset counts.
	for _, tt := range []struct {
		offset, length tenon.Value
		hi             int64
	}{
		{unknownNumber, num("3"), 3},
		{num("-2"), unknownNumber, 2},
		{num("-2"), num("5"), 2},
	} {
		got := call(stdlib.SubstrFunc, str, tt.offset, tt.length)
		if hi, ok := got.Range().LengthMax(); got.IsKnown() || !ok || hi != tt.hi || !notNull(got) {
			t.Errorf("Substr(unknown, %v, %v) = %v, want at most %d long", tt.offset, tt.length, got, tt.hi)
		}
	}
	// The clusters the prefix settles: all but its last, which what
	// follows may extend.
	if got := call(stdlib.SubstrFunc, prefixed("hello world"), num("0"), num("5")); !got.Equal(tenon.String("hello")) {
		t.Errorf("Substr(unknown beginning hello world, 0, 5) = %v, want hello", got)
	}
	if got := call(stdlib.SubstrFunc, prefixed("hello world"), num("6"), num("-1")); !promises(got, "wor") {
		t.Errorf("Substr(unknown beginning hello world, 6, -1) = %v, want an unknown beginning wor", got)
	}
	// For any prefix and continuation, and any offset and length, the
	// answer for the whole is what was answered for the prefix, or begins
	// with what it promised.
	alphabet := []string{"a", "b", " ", "\r", "\n", "\U00000301", "\U0001F1FA", "\U0001F1F8", "\U0000200D", "\U0001F468", "\U00001100", "\U00001161"}
	rng := rand.New(rand.NewPCG(20261005, 3))
	word := func() string {
		var b strings.Builder
		for range rng.IntN(7) {
			b.WriteString(alphabet[rng.IntN(len(alphabet))])
		}
		return b.String()
	}
	for range 5000 {
		p, rest := tenon.String(word()).AsString(), word()
		offset, length := tenon.NumberFromInt(int64(rng.IntN(6))), tenon.NumberFromInt(int64(rng.IntN(7)-1))
		promised := call(stdlib.SubstrFunc, prefixed(p), offset, length)
		got := call(stdlib.SubstrFunc, tenon.String(p+rest), offset, length)
		switch {
		case promised.IsKnown() && !promised.Equal(got):
			t.Fatalf("Substr(%+q, %v, %v) = %v, and %v was answered for the prefix %+q", p+rest, offset, length, got, promised, p)
		case !promised.IsKnown() && !strings.HasPrefix(got.AsString(), promised.Range().StringPrefix()):
			t.Fatalf("Substr(%+q, %v, %v) = %v, which does not begin with %+q, promised for the prefix %+q", p+rest, offset, length, got, promised.Range().StringPrefix(), p)
		}
	}
}
