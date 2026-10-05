package stdlib_test

import (
	"fmt"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/stdlib"
)

// BenchmarkLibraryGrowth measures library functions at a size and at four
// times it, for the growth job (tools/growth): one whose work grew with the
// pairs of its input, as go-cty's distinct does, allocates sixteen times as
// much at four times the size, and fails it.
func BenchmarkLibraryGrowth(b *testing.B) {
	for _, size := range []int{2000, 8000} {
		var members []tenon.Value
		for i := range size {
			members = append(members, tenon.NumberFromInt(int64(i%(size/2))))
		}
		list := tenon.List(tenon.NumberType(), members...)
		b.Run(fmt.Sprintf("distinct/%d", size), func(b *testing.B) {
			for b.Loop() {
				tenon.Call(stdlib.DistinctFunc, []tenon.Value{list}, tenon.Safe)
			}
		})
	}
}
