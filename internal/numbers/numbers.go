// Package numbers lets the standard library compute with the exact decimals
// that tenon's Number values hold, which package tenon does not export.
// Package tenon sets both functions here when it is initialized, which is
// before any package importing it runs; the values cross as any, so that
// this package stays below tenon.
package numbers

import "github.com/kmoneil/tenon/internal/decimal"

var (
	// Dec returns the decimal that v, a known Number value, holds; its marks
	// play no part.
	Dec func(v any) decimal.Dec
	// Value returns the Number value of d.
	Value func(d decimal.Dec) any
)
