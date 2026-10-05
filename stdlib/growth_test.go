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
		// Merge reads each argument once, a map not known yet among them
		// leaving every attribute before it open to its element type.
		var maps, objects []tenon.Value
		for i := range size / 2 {
			key := fmt.Sprint(i)
			maps = append(maps, tenon.Map(tenon.NumberType(), map[string]tenon.Value{key: tenon.NumberFromInt(int64(i))}), tenon.Unknown(tenon.MapType(tenon.NumberType())))
			objects = append(objects, tenon.Object(map[string]tenon.Value{key: tenon.String(key)}), tenon.Unknown(tenon.MapType(tenon.NumberType())))
		}
		b.Run(fmt.Sprintf("merge-maps/%d", size), func(b *testing.B) {
			for b.Loop() {
				tenon.Call(stdlib.MergeFunc, maps, tenon.Safe)
			}
		})
		b.Run(fmt.Sprintf("merge-objects/%d", size), func(b *testing.B) {
			for b.Loop() {
				tenon.Call(stdlib.MergeFunc, objects, tenon.Safe)
			}
		})
		// The set operations ask each set after each member once.
		var left, right []tenon.Value
		for i := range size {
			left = append(left, tenon.NumberFromInt(int64(i)))
			right = append(right, tenon.NumberFromInt(int64(i+size/2)))
		}
		right = append(right, tenon.Unknown(tenon.NumberType()))
		sets := []tenon.Value{tenon.Set(tenon.NumberType(), left...), tenon.Set(tenon.NumberType(), right...)}
		b.Run(fmt.Sprintf("setintersection/%d", size), func(b *testing.B) {
			for b.Loop() {
				tenon.Call(stdlib.SetIntersectionFunc, sets, tenon.Safe)
			}
		})
		b.Run(fmt.Sprintf("setsymmetricdifference/%d", size), func(b *testing.B) {
			for b.Loop() {
				tenon.Call(stdlib.SetSymmetricDifferenceFunc, sets, tenon.Safe)
			}
		})
	}
}
