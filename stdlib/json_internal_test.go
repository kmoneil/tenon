package stdlib

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/kmoneil/tenon"
)

// TestJSONLenIsWhatIsWritten holds jsonLen to jsonText, and positionalLen
// to positional, for numbers of every shape and strings needing every
// escape, alone and in collections, and for random numbers: the bounds
// that read them decide before the text is made, so they must count what
// it would be exactly.
func TestJSONLenIsWhatIsWritten(t *testing.T) {
	str, num := tenon.StringType(), tenon.NumberType()
	numbers := []string{"0", "1", "-1", "0.5", "-0.05", "123.4500", "1e21", "1e-7", "-1.5e-20", "120", "1.20e3", "987654321987654321987654321", "1e300", "-1e-300"}
	rng := rand.New(rand.NewPCG(20261005, 21))
	for range 2000 {
		text := strconv.Itoa(rng.IntN(1_000_000)-500_000) + "e" + strconv.Itoa(rng.IntN(80)-40)
		numbers = append(numbers, text)
	}
	var values []tenon.Value
	for _, text := range numbers {
		n := tenon.NumberFromText(text)
		if got, want := positionalLen(n), int64(len(positional(n))); got != want {
			t.Errorf("positionalLen(%s) = %d, and positional writes %d bytes", text, got, want)
		}
		values = append(values, n)
	}
	values = append(values,
		tenon.String(""), tenon.String("plain"), tenon.String("q\"b\\\b\f\n\r\t<>&\x01\x1f\U00002028\U00002029\U000000E9\U0001F600"),
		tenon.Bool(true), tenon.Bool(false), tenon.Null(str),
		tenon.List(num, tenon.NumberFromText("1e21"), tenon.NumberFromText("-0.25")),
		tenon.Set(str, tenon.String("<a>"), tenon.String("b")),
		tenon.Map(num, map[string]tenon.Value{"x&y": tenon.NumberFromInt(1), "z": tenon.Null(num)}),
		tenon.Tuple(tenon.String("a"), tenon.NumberFromText("1e-9"), tenon.Bool(true)),
		tenon.Object(map[string]tenon.Value{"a\U00002028": tenon.List(str, tenon.String("\t")), "b": tenon.Null(num)}),
		tenon.List(num),
	)
	for _, v := range values {
		if got, want := jsonLen(v, false), int64(len(jsonText(v))); got != want {
			t.Errorf("jsonLen(%v) = %d, and jsonText writes %d bytes", v, got, want)
		}
		// Canonically, a number is the text a string converts it to.
		if v.Type().Kind() == tenon.KindNumber {
			if got, want := jsonLen(v, true), int64(len(tenon.Convert(v, text, tenon.Unsafe).AsString())); got != want {
				t.Errorf("jsonLen(%v, canonical) = %d, want %d", v, got, want)
			}
		}
	}
	// A number at the window's edge measures without being written.
	if got := positionalLen(tenon.NumberFromText("1e999999")); got != 1_000_000 {
		t.Errorf("positionalLen(1e999999) = %d, want 1,000,000", got)
	}
	if got := positionalLen(tenon.NumberFromText("-1e-999999")); got != int64(len("-0."))+999_999 {
		t.Errorf("positionalLen(-1e-999999) = %d", got)
	}
}
