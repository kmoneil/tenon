package gotenon_test

import (
	"reflect"
	"sync"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/gotenon"
)

// sharedLeaf and sharedTree are mapped first by
// TestMappingsAreBuiltConcurrently, so that its goroutines build their
// mappings, and every mapping those need, all at once.
type sharedLeaf struct {
	N int               `tenon:"n"`
	S []string          `tenon:"s"`
	M map[string]string `tenon:"m"`
}

type sharedTree struct {
	Name   string                `tenon:"name"`
	Leaf   *sharedLeaf           `tenon:"leaf"`
	Leaves []sharedLeaf          `tenon:"leaves"`
	ByName map[string]sharedLeaf `tenon:"by_name"`
}

// TestMappingsAreBuiltConcurrently has goroutines meet Go types for the first
// time together: each encodes and decodes a value of them and asks for the
// type and the constraint of types built from them, while the mappings they
// share are built and cached. make check runs it under the race detector,
// which fails on a write to the cache that another goroutine sees unfinished,
// and each goroutine must read back what it wrote.
func TestMappingsAreBuiltConcurrently(t *testing.T) {
	const workers = 16
	in := sharedTree{
		Name:   "root",
		Leaf:   &sharedLeaf{N: 1, S: []string{"a"}, M: map[string]string{"k": "v"}},
		Leaves: []sharedLeaf{{N: 2}, {N: 3, S: []string{"b", "c"}}},
		ByName: map[string]sharedLeaf{"x": {N: 4}},
	}
	finished := make([]bool, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			v, err := gotenon.Encode(in)
			if err != nil {
				t.Errorf("worker %d: Encode: %v", w, err)
				return
			}
			out, err := gotenon.Decode[sharedTree](v, tenon.Safe)
			if err != nil {
				t.Errorf("worker %d: Decode: %v", w, err)
				return
			}
			if !reflect.DeepEqual(out, in) {
				t.Errorf("worker %d read back %+v, want %+v", w, out, in)
			}
			if _, ok := gotenon.TypeFor(reflect.TypeFor[[]sharedTree]()); !ok {
				t.Errorf("worker %d: TypeFor found no type for []sharedTree", w)
			}
			gotenon.ConstraintFor(reflect.TypeFor[map[string]sharedTree]())
			finished[w] = true
		})
	}
	wg.Wait()
	for w, ok := range finished {
		if !ok {
			t.Errorf("worker %d did not finish", w)
		}
	}
}
