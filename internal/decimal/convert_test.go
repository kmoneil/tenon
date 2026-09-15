package decimal

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestRat(t *testing.T) {
	if r := (Dec{}).Rat(); r.Sign() != 0 {
		t.Errorf("zero as a rational is %s", r.RatString())
	}
	rng := rand.New(rand.NewPCG(26, 26))
	for range 2000 {
		d := randomNumber(t, rng)
		if got, want := d.Rat(), toRat(d); got.Cmp(want) != 0 {
			t.Fatalf("%s as a rational is %s, want %s", d, got.RatString(), want.RatString())
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
