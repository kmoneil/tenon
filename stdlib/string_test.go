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
