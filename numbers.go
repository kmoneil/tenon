package tenon

import (
	"github.com/kmoneil/tenon/internal/decimal"
	"github.com/kmoneil/tenon/internal/numbers"
)

// The standard library computes with the decimals Number values hold, which
// it reaches through internal/numbers rather than through their text, which
// costs in proportion to a number's magnitude where a decimal costs in
// proportion to its digits.
func init() {
	numbers.Dec = func(v any) decimal.Dec { return decOf(v.(Value)) }
	numbers.Value = func(d decimal.Dec) any { return numberValue(d) }
}
