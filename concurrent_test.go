package tenon_test

import (
	"sync"
	"testing"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/internal/conformance/values"
)

// shared is the deep mark that TestValuesAreSharedBetweenGoroutines attaches.
type shared struct{}

func (shared) MarkID() string                 { return "shared" }
func (shared) Propagation() tenon.Propagation { return tenon.Propagate }
func (shared) Redacting() bool                { return false }
func (shared) Deep() bool                     { return true }

// TestValuesAreSharedBetweenGoroutines reads each value of the corpus from
// several goroutines at once, as the package's documentation says a program
// may. Some of them work out what a value keeps once it is worked out, its
// hash and a set's count of provably distinct members, while the others copy
// the value, compare it, mark it and read its members; make check runs this
// under the race detector, which fails on any read or copy of those fields
// that is not atomic. The values are built here, so that nothing has worked
// their facts out before the goroutines meet.
func TestValuesAreSharedBetweenGoroutines(t *testing.T) {
	var hashed, counted int
	for _, v := range values.All() {
		_, marks := tenon.UnmarkDeep(v)
		hashable := v.IsKnown() && v.HasContent() && len(marks) == 0
		var kind tenon.Kind
		if v.HasContent() {
			kind = v.Type().Kind()
		}
		sequence := kind == tenon.KindList || kind == tenon.KindSet || kind == tenon.KindTuple
		marked := tenon.WithMarks(v, shared{})
		var wg sync.WaitGroup
		if hashable {
			hashed++
			wg.Go(func() { tenon.Hash(v) })
			wg.Go(func() { tenon.Set(v.Type(), v, v) })
		}
		if kind == tenon.KindSet {
			if !v.IsKnown() {
				counted++
			}
			wg.Go(func() { tenon.Length(v) })
		}
		wg.Go(func() { tenon.Equals(v, marked) })
		wg.Go(func() { tenon.Diff(v, marked) })
		wg.Go(func() { tenon.WithMarks(v, shared{}) })
		if sequence {
			wg.Go(func() { marked.Elements() })
		}
		wg.Wait()
	}
	// The corpus hashed 40 values and counted the members of 3 sets when this
	// was written; far fewer, and the goroutines no longer meet over caches.
	if hashed < 30 || counted < 2 {
		t.Errorf("hashed %d values and counted the members of %d sets, want at least 30 and 2", hashed, counted)
	}
}
