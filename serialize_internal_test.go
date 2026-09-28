package tenon

import (
	"bytes"
	"math"
	"math/big"
	"math/rand"
	"slices"
	"testing"

	"github.com/kmoneil/tenon/internal/cbor"
	"github.com/kmoneil/tenon/internal/conformance"
	"github.com/kmoneil/tenon/internal/decimal"
)

// edgeParts returns coefficients and exponents at the edges that writing and
// reading a number without a big.Int turn on: the ends of an int64; the
// coefficients whose product with a power of ten does or does not fit in the
// 64 bits a CBOR integer holds, on either side of zero; the exponents that
// make an integer of twenty digits or more; and the exponents at and past the
// ends of the digit window, with trailing zeros to strip or none.
func edgeParts() (cs, exps []int64) {
	cs = []int64{1, 7, 10, 1000, math.MaxInt64, math.MaxInt64 - 1}
	for k := int64(1); k <= 19; k++ {
		p := new(big.Int).Exp(big.NewInt(10), big.NewInt(k), nil)
		for _, most := range []*big.Int{
			new(big.Int).SetUint64(math.MaxUint64), // what a CBOR integer holds
			new(big.Int).Lsh(big.NewInt(1), 64),    // what a negative one holds, as a magnitude
		} {
			q := new(big.Int).Quo(most, p)
			for _, delta := range []int64{-1, 0, 1} {
				if c := new(big.Int).Add(q, big.NewInt(delta)); c.IsInt64() {
					cs = append(cs, c.Int64())
				}
			}
		}
	}
	for _, c := range slices.Clone(cs) {
		cs = append(cs, -c)
	}
	cs = append(cs, math.MinInt64)
	const m = decimal.MaxAdjustedExponent
	exps = []int64{math.MinInt64, -m - 20, -m - 19, -m - 18, -m - 1, -m, -m + 1, m - 19, m - 18, m, m + 1, math.MaxInt64}
	for e := int64(-22); e <= 22; e++ {
		exps = append(exps, e)
	}
	return cs, exps
}

// randomParts returns a coefficient of any magnitude that fits an int64, and
// an exponent near zero.
func randomParts(r *rand.Rand) (int64, int64) {
	c := r.Int63() >> r.Intn(63)
	if r.Intn(2) == 0 {
		c = -c
	}
	return c, int64(r.Intn(61) - 30)
}

// TestConformance_NU003_SmallCoefficientsEncodeAsBigOnes holds the encoding of
// a number whose coefficient fits an int64, which is written without a
// big.Int, to the encoding the big.Int path gives the same number, at the
// edges where the two could part and over random numbers besides.
func TestConformance_NU003_SmallCoefficientsEncodeAsBigOnes(t *testing.T) {
	conformance.Covers(t, "NU-003", "SE-001", "SE-032")
	encoded := 0
	check := func(c, exp int64) {
		t.Helper()
		d, err := decimal.FromParts(big.NewInt(c), exp)
		if err != nil {
			return // outside the window, which no value holds
		}
		small, coefficient, e := d.Parts()
		if coefficient != nil {
			t.Fatalf("%d × 10^%d is held with a big.Int coefficient", c, exp)
		}
		encoded++
		if fast, slow := appendNumber(nil, d), appendBigNumber(nil, big.NewInt(small), e); !bytes.Equal(fast, slow) {
			t.Errorf("%d × 10^%d encodes as % x, where the big path gives % x", small, e, fast, slow)
		}
	}
	cs, exps := edgeParts()
	for _, c := range cs {
		for _, e := range exps {
			check(c, e)
		}
	}
	r := rand.New(rand.NewSource(1612))
	for range conformance.Iterations(t, 10000) {
		check(randomParts(r))
	}
	if encoded < 5000 {
		t.Errorf("only %d numbers were encoded", encoded)
	}
}

// TestConformance_NU003_SmallCoefficientsDecodeAsBigOnes holds reading a number
// whose coefficient fits an int64, which is done without a big.Int, to the
// number the big.Int path reads from the same bytes. The two are compared by
// their representation and not only their value: a coefficient that fits an
// int64 is held in one, wherever it was read from. The bytes include forms no
// encoder writes, which the decoder reads before refusing them as not
// canonical, and exponents past the window, which it refuses as out of range;
// a decimal fraction whose mantissa is a multiple of ten, zero among them,
// it refuses as not canonical as it reads the mantissa, whatever the
// exponent, as the reading stops there.
func TestConformance_NU003_SmallCoefficientsDecodeAsBigOnes(t *testing.T) {
	conformance.Covers(t, "NU-003", "SE-001", "SE-032")
	read := 0
	check := func(item []byte, c *big.Int, exp int64) {
		t.Helper()
		v, derr := (&decoder{r: cbor.NewReader(item)}).number()
		want, err := decimal.FromParts(c, exp)
		if (derr != nil) != (err != nil) {
			t.Errorf("% x: read gives %v, and the big path %v", item, derr, err)
			return
		}
		if err != nil {
			return
		}
		read++
		gotSmall, gotBig, gotExp := v.n.data.(decimal.Dec).Parts()
		wantSmall, wantBig, wantExp := want.Parts()
		if gotSmall != wantSmall || gotExp != wantExp || (gotBig == nil) != (wantBig == nil) || gotBig != nil && gotBig.Cmp(wantBig) != 0 {
			t.Errorf("% x: read as %d, %v, 10^%d, where the big path holds %d, %v, 10^%d",
				item, gotSmall, gotBig, gotExp, wantSmall, wantBig, wantExp)
		}
	}
	// fraction checks the decimal fraction of c and exp, its coefficient
	// written by the head that coefficient appends.
	fraction := func(c *big.Int, exp int64, coefficient func([]byte) []byte) {
		t.Helper()
		item := cbor.AppendTag(nil, tagDecimal)
		item = cbor.AppendArray(item, 2)
		item = cbor.AppendInt(item, exp)
		item = coefficient(item)
		if new(big.Int).Rem(c, big.NewInt(10)).Sign() != 0 {
			check(item, c, exp)
			return
		}
		if _, derr := (&decoder{r: cbor.NewReader(item)}).number(); derr == nil || derr.code != CodeSerializeNotCanonical {
			t.Errorf("% x: a mantissa that is a multiple of ten read as %v, not refused as not canonical", item, derr)
		}
	}
	cs, exps := edgeParts()
	for _, c := range cs {
		check(cbor.AppendInt(nil, c), big.NewInt(c), 0)
		for _, e := range exps {
			fraction(big.NewInt(c), e, func(b []byte) []byte { return cbor.AppendInt(b, c) })
		}
	}
	// Integers past an int64, which CBOR's integers hold and big.Int reads.
	for _, arg := range []uint64{math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64} {
		positive := new(big.Int).SetUint64(arg)
		negative := new(big.Int).Neg(new(big.Int).Add(positive, big.NewInt(1)))
		for _, e := range []int64{0, 1, -1} {
			fraction(positive, e, func(b []byte) []byte { return cbor.AppendUint(b, arg) })
			fraction(negative, e, func(b []byte) []byte { return cbor.AppendHead(b, cbor.MajorNeg, arg) })
		}
		check(cbor.AppendUint(nil, arg), positive, 0)
		check(cbor.AppendHead(nil, cbor.MajorNeg, arg), negative, 0)
	}
	r := rand.New(rand.NewSource(1612))
	for range conformance.Iterations(t, 10000) {
		c, e := randomParts(r)
		fraction(big.NewInt(c), e, func(b []byte) []byte { return cbor.AppendInt(b, c) })
	}
	if read < 5000 {
		t.Errorf("only %d numbers were read", read)
	}
}
