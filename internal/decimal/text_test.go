package decimal

import (
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"

	"tenon/conformance"
)

var (
	// positionalText and scientificText match the two shapes of canonical
	// text.
	positionalText = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]*[1-9])?$`)
	scientificText = regexp.MustCompile(`^-?[1-9](\.[0-9]*[1-9])?e-?[1-9][0-9]*$`)

	// grammar matches exactly the texts that Parse accepts.
	grammar = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
)

func TestConformance_NU020_CanonicalText(t *testing.T) {
	conformance.Covers(t, "NU-020")
	zeros := func(n int) string { return strings.Repeat("0", n) }
	for _, tt := range []struct{ in, want string }{
		{"0", "0"},
		{"-0.000e5", "0"},

		// Either side of the adjusted exponents where the form changes.
		{"1e-21", "1e-21"},
		{"-1.25e-21", "-1.25e-21"},
		{"1e-20", "0." + zeros(19) + "1"},
		{"1.5e-20", "0." + zeros(19) + "15"},
		{"1", "1"},
		{"-9.999", "-9.999"},
		{"1e20", "1" + zeros(20)},
		{"1.23456789012345678901234e20", "123456789012345678901.234"},
		{strings.Repeat("9", 21), strings.Repeat("9", 21)},
		{"1e21", "1e21"},
		{"1.5e21", "1.5e21"},
		{strings.Repeat("9", 22), "9." + strings.Repeat("9", 21) + "e21"},

		{"0.05", "0.05"},
		{"-123.456", "-123.456"},
		{"1200", "1200"},
		{"12.500", "12.5"},
		{"1.23456789e100", "1.23456789e100"},
		{"-1e-999999", "-1e-999999"},
	} {
		if got := mustParse(t, tt.in).String(); got != tt.want {
			t.Errorf("canonical text of %s = %s, want %s", tt.in, got, tt.want)
		}
	}

	// Every canonical text has the shape its adjusted exponent calls for.
	rng := rand.New(rand.NewPCG(20, 20))
	for range conformance.Iterations(t, 3000) {
		d := randomNumber(t, rng)
		text := d.String()
		adj := d.exp + int64(len(d.coefficientDigits())) - 1
		scientific := adj > 20 || adj < -20
		if scientific && !scientificText.MatchString(text) || !scientific && !positionalText.MatchString(text) || text == "-0" {
			t.Fatalf("%s, with adjusted exponent %d, is not canonical text", text, adj)
		}
	}
}

func TestConformance_NU021_Parsing(t *testing.T) {
	conformance.Covers(t, "NU-021")
	for _, tt := range []struct{ in, want string }{
		{"7", "7"},
		{"-7", "-7"},
		{"007", "7"},
		{"-0", "0"},
		{"0.0", "0"},
		{"00.5", "0.5"},
		{"12.3400", "12.34"},
		{"1e5", "100000"},
		{"1E5", "100000"},
		{"1e+5", "100000"},
		{"1.5e-5", "0.000015"},
		{"-000123.4500e2", "-12345"},
		{"1e0", "1"},
		{"1e-0", "1"},
		{"1e+00000000000000000000001", "10"},
		{"9e999999", "9e999999"},
	} {
		if d, err := Parse(tt.in); err != nil || d.String() != tt.want {
			t.Errorf("Parse(%q) = %v, %v; want %s", tt.in, d, err, tt.want)
		}
	}

	// The grammar admits nothing else: no text around the number, and no
	// fraction or exponent without digits.
	for _, s := range []string{
		" 1", "1 ", "\t1", "1\n", ".5", "5.", "-.5", "1.e5", "1e", "1e+", "1e-", "e5",
		"-", ".", "--1", "-+1", "1.2.3", "1e5e5", "1e5.5", "0x", "1f",
	} {
		if d, err := Parse(s); err != ErrSyntax {
			t.Errorf("Parse(%q) = %v, %v; want ErrSyntax", s, d, err)
		}
	}

	// Random short texts over the characters of numbers and near-numbers are
	// numbers exactly when they match the grammar.
	const alphabet = "0123456789.eE+-_, x"
	rng := rand.New(rand.NewPCG(21, 21))
	for range conformance.Iterations(t, 20000) {
		var b strings.Builder
		for range rng.IntN(9) {
			b.WriteByte(alphabet[rng.IntN(len(alphabet))])
		}
		s := b.String()
		if _, err := Parse(s); (err != ErrSyntax) != grammar.MatchString(s) {
			t.Fatalf("Parse(%q) = %v, but matching the grammar is %t", s, err, grammar.MatchString(s))
		}
	}
}

func TestConformance_NU022_Rejections(t *testing.T) {
	conformance.Covers(t, "NU-022")
	for _, s := range []string{
		"+5", "+0", "+1.5e3",
		"Inf", "-Inf", "+Inf", "inf", "Infinity", "-Infinity", "NaN", "nan", "-NaN",
		"0x1A", "0X1a", "0x1.8p3", "1p4", "1P4",
		"1,000", "1_000", "1 000", "1'000", "1\u00a0000", "1\u2009000",
		"",
		"\u22125",      // a minus sign that is not "-"
		"\uff11\uff12", // fullwidth digits
		"\u0661\u0662", // Arabic-Indic digits
		"\u221e",       // infinity
	} {
		if d, err := Parse(s); err != ErrSyntax {
			t.Errorf("Parse(%q) = %v, %v; want ErrSyntax", s, d, err)
		}
	}
}

func TestConformance_NU023_ExactParsing(t *testing.T) {
	conformance.Covers(t, "NU-023")
	// No digit of a literal is lost to a fixed width.
	for _, s := range []string{
		"3.14159265358979323846264338327950288419716939937510582097494459230781640628620899",
		"9007199254740993",
		"-12345678901234567890." + strings.Repeat("1", 5000),
	} {
		if got := mustParse(t, s).String(); got != s {
			t.Errorf("Parse(%q).String() = %q", s, got)
		}
	}
	// The binary double nearest 0.1, written out, is not 0.1.
	if mustParse(t, "0.1000000000000000055511151231257827021181583404541015625").Equal(mustParse(t, "0.1")) {
		t.Error("a long literal was rounded to a short one")
	}
	if !mustParse(t, "123456789012345678901234567890e-30").Equal(mustParse(t, "0.12345678901234567890123456789")) {
		t.Error("an exponent moved the digits inexactly")
	}

	// Canonical text parses back to the same number, and is its own
	// canonical text.
	rng := rand.New(rand.NewPCG(23, 23))
	for range conformance.Iterations(t, 3000) {
		d := randomNumber(t, rng)
		if back := mustParse(t, d.String()); !back.Equal(d) || back.String() != d.String() {
			t.Fatalf("%s parsed back as %s", d, back)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"0", "-1.5e-3", "+5", "1e", "007", "1e999999", "1e1000000", " 1", "1_0", "0.000e-99"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := Parse(s)
		if (err != ErrSyntax) != grammar.MatchString(s) {
			t.Fatalf("Parse(%q) = %v, but matching the grammar is %t", s, err, grammar.MatchString(s))
		}
		if err != nil {
			return
		}
		text := d.String()
		if back, err := Parse(text); err != nil || !back.Equal(d) || back.String() != text {
			t.Fatalf("Parse(%q) gave %s, which parses back as %v, %v", s, text, back, err)
		}
	})
}
