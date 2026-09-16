package decimal

import (
	"go/scanner"
	"go/token"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kmoneil/tenon/conformance"
)

// mustParse parses s, failing t if s is not a number.
func mustParse(t *testing.T, s string) Dec {
	t.Helper()
	d, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return d
}

// bigInt returns the integer that the decimal digits s denote.
func bigInt(t *testing.T, s string) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("%q is not an integer", s)
	}
	return v
}

// checkCanonical fails t unless d is in the canonical form that Dec documents.
func checkCanonical(t *testing.T, d Dec) {
	t.Helper()
	switch {
	case d.big != nil && d.big.IsInt64():
		t.Errorf("%v: coefficient %v is big but fits in an int64", d, d.big)
	case d.Sign() == 0 && d.exp != 0:
		t.Errorf("zero has exponent %d", d.exp)
	case d.Sign() != 0 && strings.HasSuffix(d.coefficientDigits(), "0"):
		t.Errorf("%v: coefficient %s has a trailing zero", d, d.coefficientDigits())
	case d.Sign() != 0 && !inRange(d.exp, int64(len(d.coefficientDigits()))):
		t.Errorf("%v is out of range", d)
	}
}

// spell writes the number coeff × 10^exp with lead leading zeros, trail
// extra trailing zeros, and point digits after a decimal point (none if point
// is 0), choosing the written exponent so that the value stays the same.
func spell(neg bool, coeff string, exp, lead, trail, point int, e string) string {
	body := strings.Repeat("0", lead) + coeff + strings.Repeat("0", trail)
	if point > 0 {
		body = body[:len(body)-point] + "." + body[len(body)-point:]
	}
	s := body + e + strconv.Itoa(exp-trail+point)
	if neg {
		s = "-" + s
	}
	return s
}

func TestConformance_NU001_Domain(t *testing.T) {
	conformance.Covers(t, "NU-001")
	// Terminating decimal fractions are held exactly, with any number of
	// digits.
	long := strings.Repeat("1234567890", 100)
	for _, s := range []string{"0.1", "-0.3", "123.456", long, "0." + long, long + "." + long, "-" + long + "e-500"} {
		d := mustParse(t, s)
		checkCanonical(t, d)
		if back := mustParse(t, d.String()); !back.Equal(d) {
			t.Errorf("%q: the canonical text %s is a different number", s, d)
		}
	}
	if d := mustParse(t, "0.1"); d.small != 1 || d.exp != -1 {
		t.Errorf("0.1 is held as %d × 10^%d, not exactly one tenth", d.small, d.exp)
	}

	// The domain reaches the magnitude limit in both directions.
	for _, s := range []string{"9.99e999999", "-1e-999999", "-" + long + "e999000"} {
		checkCanonical(t, mustParse(t, s))
	}
}

func TestConformance_NU002_NoInfinityOrNaN(t *testing.T) {
	conformance.Covers(t, "NU-002")
	for _, s := range []string{"Inf", "-Inf", "inf", "Infinity", "-Infinity", "NaN", "nan", "-NaN", "sNaN"} {
		if d, err := Parse(s); err != ErrSyntax {
			t.Errorf("Parse(%q) = %v, %v; want ErrSyntax", s, d, err)
		}
	}
	// A magnitude too large or too small to hold is an error, never an
	// infinity or a zero.
	for _, s := range []string{"1e1000000", "-1e1000000", "1e-1000000"} {
		if d, err := Parse(s); err != ErrOutOfRange {
			t.Errorf("Parse(%q) = %v, %v; want ErrOutOfRange", s, d, err)
		}
	}
}

func TestConformance_NU003_RepresentationNotObservable(t *testing.T) {
	conformance.Covers(t, "NU-003")
	groups := []struct {
		text      string   // the canonical text
		spellings []string // texts of the number
		ints      []int64  // int64s of the number
		bigs      []string // big.Int values of the number
	}{
		{"0", []string{"0", "-0", "0.000", "0e10", "-0.0E-5", "000"}, []int64{0}, []string{"0"}},
		{"1200", []string{"1200", "1.2e3", "12e2", "1200.000", "0001200", "120000e-2", "1.2E+3"}, []int64{1200}, []string{"1200"}},
		{"0.5", []string{"0.5", "5e-1", "0.50", "50E-2", "000.5000"}, nil, nil},
		{"-9223372036854775808", []string{"-9223372036854775808", "-9.223372036854775808e18"}, []int64{math.MinInt64}, []string{"-9223372036854775808"}},
		{"9223372036854775808", []string{"9223372036854775808", "922337203685477580.8e1"}, nil, []string{"9223372036854775808"}},
		{"1e30", []string{"1e30", "1000000000000000000000000000000", "0.001e33"}, nil, []string{"1000000000000000000000000000000"}},
		{
			"-1.23456789012345678901234567890123e29",
			[]string{"-123456789012345678901234567890.123", "-1.23456789012345678901234567890123e29", "-123456789012345678901234567890123000e-6"},
			nil, nil,
		},
	}
	for _, g := range groups {
		var all []Dec
		for _, s := range g.spellings {
			all = append(all, mustParse(t, s))
		}
		for _, v := range g.ints {
			all = append(all, FromInt64(v))
		}
		for _, s := range g.bigs {
			d, err := FromBigInt(bigInt(t, s))
			if err != nil {
				t.Fatalf("FromBigInt(%s): %v", s, err)
			}
			all = append(all, d)
		}
		for _, d := range all {
			checkCanonical(t, d)
			if !d.Equal(all[0]) || d.String() != g.text {
				t.Errorf("%s: a construction gave %s", g.text, d)
			}
		}
	}
}

func TestConformance_NU003_RandomConstructions(t *testing.T) {
	conformance.Covers(t, "NU-003")
	rng := rand.New(rand.NewPCG(3, 3))
	for range conformance.Iterations(t, 2000) {
		var digits strings.Builder
		digits.WriteByte(byte('1' + rng.IntN(9)))
		for range rng.IntN(60) {
			digits.WriteByte(byte('0' + rng.IntN(10)))
		}
		coeff, exp, neg := digits.String(), rng.IntN(200)-100, rng.IntN(2) == 0
		base := mustParse(t, spell(neg, coeff, exp, 0, 0, 0, "e"))
		checkCanonical(t, base)
		want := base.String()

		var all []Dec
		for range 5 {
			lead, trail := rng.IntN(3), rng.IntN(5)
			point := rng.IntN(lead + len(coeff) + trail)
			e := []string{"e", "E", "e+"}[rng.IntN(3)]
			if exp-trail+point < 0 && e == "e+" {
				e = "e"
			}
			all = append(all, mustParse(t, spell(neg, coeff, exp, lead, trail, point, e)))
		}
		if exp >= 0 {
			integer := coeff + strings.Repeat("0", exp)
			if neg {
				integer = "-" + integer
			}
			d, err := FromBigInt(bigInt(t, integer))
			if err != nil {
				t.Fatalf("FromBigInt(%s): %v", integer, err)
			}
			all = append(all, d)
			if v, err := strconv.ParseInt(integer, 10, 64); err == nil {
				all = append(all, FromInt64(v))
			}
		}
		for _, d := range all {
			checkCanonical(t, d)
			if !d.Equal(base) || d.String() != want {
				t.Fatalf("%s: a construction gave %s", want, d)
			}
		}
	}
}

func TestConformance_NU016_MagnitudeLimit(t *testing.T) {
	conformance.Covers(t, "NU-016")
	// Every digit of a number lies in the window from the 10^999999 place down
	// to the 10^-999999 place: the leading digit no higher, and the last one
	// no lower.
	for _, s := range []string{
		"1e999999", "-1e999999", "9.999999e999999", "123e999997", "0.0001e1000003",
		"1e-999999", "-9e-999999", "1.2e-999998", "-12.34e-999997",
		"0e99999999999999999999999", "-0.000e-99999999999999999999",
	} {
		checkCanonical(t, mustParse(t, s))
	}
	for _, s := range []string{
		"1e1000000", "-1e1000000", "10e999999", "123e999998",
		"1e-1000000", "0.1e-999999",
		"-9.5e-999999", "1.2e-999999", "12.34e-999998", "0.000123e-999995",
		"1e99999999999999999999999999", "1e-99999999999999999999999999",
	} {
		if d, err := Parse(s); err != ErrOutOfRange {
			t.Errorf("Parse(%q) = %v, %v; want ErrOutOfRange", s, d, err)
		}
	}

	// Integers are held to the same limit: an integer's adjusted exponent is
	// its number of digits less one.
	nines := func(digits int64) *big.Int { return new(big.Int).Sub(pow10(digits), big.NewInt(1)) }
	if _, err := FromBigInt(nines(MaxAdjustedExponent + 1)); err != nil {
		t.Errorf("FromBigInt of a %d-digit integer: %v", MaxAdjustedExponent+1, err)
	}
	for _, v := range []*big.Int{nines(MaxAdjustedExponent + 2), pow10(MaxAdjustedExponent + 1)} {
		if _, err := FromBigInt(v); err != ErrOutOfRange {
			t.Errorf("FromBigInt of a %d-bit integer: %v, want ErrOutOfRange", v.BitLen(), err)
		}
	}
}

// floatNames holds the identifiers of Go's binary floating-point types and of
// the standard APIs built on them.
var floatNames = map[string]bool{
	"flo" + "at32": true, "flo" + "at64": true,
	"Float": true, "NewFloat": true, "SetFloat64": true, "Float32": true, "Float64": true,
	"Float32bits": true, "Float64bits": true, "Float32frombits": true, "Float64frombits": true,
	"ParseFloat": true, "FormatFloat": true, "AppendFloat": true,
}

// TestNoBinaryFloats keeps binary floating point out of the package, tests
// included: numbers here are exact, and a float anywhere, even as a test
// oracle, invites inexact results.
func TestNoBinaryFloats(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		var s scanner.Scanner
		s.Init(fset.AddFile(name, fset.Base(), len(src)), src, nil, 0)
		for {
			pos, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			if tok == token.FLOAT || tok == token.IMAG || (tok == token.IDENT && floatNames[lit]) {
				t.Errorf("%s: %s", fset.Position(pos), lit)
			}
		}
	}
}
