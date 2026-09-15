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
	// larger magnitude.
	if da, ea := d.adjusted(), e.adjusted(); da != ea {
		return cmp.Compare(da, ea) * ds
	}
	// With equal adjusted exponents, the exponents differ by less than the
	// longer coefficient's digit count, so aligning the coefficients is cheap.
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

// adjusted returns the adjusted exponent of d, which must not be zero.
func (d Dec) adjusted() int64 {
	if d.big == nil {
		return d.exp + digits64(d.small) - 1
	}
	return d.exp + int64(len(d.coefficientDigits())) - 1
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
