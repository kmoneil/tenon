package decimal

import (
	"cmp"
	"encoding/binary"
	"hash/maphash"
)

// Cmp compares d and e numerically, returning -1, 0 or +1 as d is less than,
// equal to, or greater than e.
func (d Dec) Cmp(e Dec) int {
	ds, es := d.Sign(), e.Sign()
	switch {
	case ds != es:
		return cmp.Compare(ds, es)
	case ds == 0:
		return 0
	}
	// Both numbers have the same sign. A larger adjusted exponent means a
	// larger magnitude, and bounds on the adjusted exponents, which the bit
	// lengths of the coefficients give without counting a digit, settle most
	// comparisons.
	dlo, dhi := d.adjustedBounds()
	elo, ehi := e.adjustedBounds()
	switch {
	case dhi < elo:
		return -ds
	case ehi < dlo:
		return ds
	}
	// Otherwise the adjusted exponents are within two of each other, so the
	// exponents differ by little more than the longer coefficient's digit
	// count, and aligning the coefficients is cheap.
	exp := min(d.exp, e.exp)
	if d.big == nil && e.big == nil {
		if x, ok := scaleSmall(d.small, d.exp-exp); ok {
			if y, ok := scaleSmall(e.small, e.exp-exp); ok {
				return cmp.Compare(x, y)
			}
		}
	}
	return d.scaledCoefficient(d.exp - exp).Cmp(e.scaledCoefficient(e.exp - exp))
}

// adjustedBounds returns the least and the greatest adjusted exponent that d,
// which must not be zero, can have for the bit length of its coefficient: the
// adjusted exponent itself for a coefficient that fits in an int64, and two
// neighbours at most for a big one.
func (d Dec) adjustedBounds() (lo, hi int64) {
	if d.big == nil {
		adj := d.exp + digits64(d.small) - 1
		return adj, adj
	}
	dlo, dhi := digitBounds(d.big)
	return d.exp + dlo - 1, d.exp + dhi - 1
}

// WriteHash writes d to h. It writes the canonical form, which every
// construction of a number shares, so equal numbers write the same bytes. The
// bytes delimit themselves, so numbers written in turn hash as a sequence.
func (d Dec) WriteHash(h *maphash.Hash) {
	switch {
	case d.big != nil:
		mag := d.big.Bytes()
		var head [18]byte
		head[0] = 2
		binary.LittleEndian.PutUint64(head[1:9], uint64(d.exp))
		if d.big.Sign() < 0 {
			head[9] = 1
		}
		binary.LittleEndian.PutUint64(head[10:], uint64(len(mag)))
		h.Write(head[:])
		h.Write(mag)
	case d.small != 0:
		var b [17]byte
		b[0] = 1
		binary.LittleEndian.PutUint64(b[1:9], uint64(d.exp))
		binary.LittleEndian.PutUint64(b[9:], uint64(d.small))
		h.Write(b[:])
	default:
		h.WriteByte(0)
	}
}
