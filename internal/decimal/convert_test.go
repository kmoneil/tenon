package decimal

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"

	"tenon/conformance"
)

func TestRat(t *testing.T) {
	if r := (Dec{}).Rat(); r.Sign() != 0 {
		t.Errorf("zero as a rational is %s", r.RatString())
	}
	rng := rand.New(rand.NewPCG(26, 26))
	for range conformance.Iterations(t, 2000) {
		d := randomNumber(t, rng)
		got, want := d.Rat(), toRat(d)
		if got.Cmp(want) != 0 {
			t.Fatalf("%s as a rational is %s, want %s", d, got.RatString(), want.RatString())
		}
		if b, ok := d.BigInt(); ok != want.IsInt() || ok && b.Cmp(want.Num()) != 0 {
			t.Fatalf("%s as an integer is %v, %t", d, b, ok)
		}
	}
}

func TestInt64(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want int64
		ok   bool
	}{
		{"0", 0, true},
		{"-0", 0, true},
		{"1200", 1200, true},
		{"1.2e3", 1200, true},
		{"9.2e18", 9200000000000000000, true},
		{"9223372036854775807", math.MaxInt64, true},
		{"-9223372036854775808", math.MinInt64, true},
		{"9223372036854775808", 0, false},
		{"-9223372036854775809", 0, false},
		{"1e19", 0, false},
		{"1.5", 0, false},
		{"1e-5", 0, false},
	} {
		if got, ok := mustParse(t, tt.in).Int64(); got != tt.want || ok != tt.ok {
			t.Errorf("Int64 of %s = %d, %t; want %d, %t", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestPartsAndFromParts(t *testing.T) {
	for _, text := range []string{"0", "1", "-1", "1000", "1.5", "-0.25", "1e999999", "1e-999999", "123456789012345678901234567890", "9.99e-5"} {
		d, err := Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		small, bigC, exp := d.Parts()
		c := bigC
		if c == nil {
			c = big.NewInt(small)
		}
		if d.Sign() != 0 && new(big.Int).Rem(c, big.NewInt(10)).Sign() == 0 {
			t.Errorf("%s: the coefficient %v is a multiple of ten", text, c)
		}
		back, err := FromParts(c, exp)
		if err != nil || !back.Equal(d) {
			t.Errorf("%s: FromParts(%v, %d) = %v, %v", text, c, exp, back, err)
		}
	}
	// Trailing zeros are stripped, and exponents far outside the window are
	// refused whatever the coefficient.
	if d, err := FromParts(big.NewInt(1000), -3); err != nil || !d.Equal(FromInt64(1)) {
		t.Errorf("FromParts(1000, -3) = %v, %v", d, err)
	}
	if d, err := FromParts(big.NewInt(10), -1000000); err != nil || d.String() != "1e-999999" {
		t.Errorf("FromParts(10, -1000000) = %v, %v", d, err)
	}
	for _, exp := range []int64{1000000, -1000001, math.MaxInt64, math.MinInt64} {
		if _, err := FromParts(big.NewInt(7), exp); err != ErrOutOfRange {
			t.Errorf("FromParts(7, %d) gave %v, want ErrOutOfRange", exp, err)
		}
	}
}
