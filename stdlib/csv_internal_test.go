package stdlib

import (
	"encoding/csv"
	"errors"
	"io"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestCSVReaderIsEncodingCSV holds tenon's CSV reader to encoding/csv with
// its defaults, which go-cty's CSVDecode reads with: for random texts of
// commas, quotation marks, line ends, CRs, spaces and letters, the same
// records, field by field, and a failure where it fails, on the same line
// and column.
func TestCSVReaderIsEncodingCSV(t *testing.T) {
	pieces := []string{"a", "b", ",", `"`, `""`, "\n", "\r", "\r\n", " ", "\U000000E9"}
	rng := rand.New(rand.NewPCG(20261005, 31))
	failures := 0
	for range 50000 {
		var b strings.Builder
		for range rng.IntN(12) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		text := b.String()
		want := csv.NewReader(strings.NewReader(text))
		want.FieldsPerRecord = -1
		got := &csvReader{text: text}
		for {
			wfields, werr := want.Read()
			gfields, _, ok, failure := got.record()
			if errors.Is(werr, io.EOF) {
				if ok || failure != nil {
					t.Fatalf("%q: encoding/csv ends, and tenon reads %q, %v", text, gfields, failure)
				}
				break
			}
			var pe *csv.ParseError
			if errors.As(werr, &pe) {
				failures++
				at := "line " + strconv.Itoa(pe.Line) + ", column " + strconv.Itoa(pe.Column) + ":"
				if failure == nil || !strings.Contains(failure.message, at) {
					t.Fatalf("%q: encoding/csv fails %v, and tenon %v, %q", text, werr, failure, gfields)
				}
				break
			}
			if failure != nil || !ok || !slices.Equal(gfields, wfields) {
				t.Fatalf("%q: encoding/csv reads %q, and tenon %q, %v", text, wfields, gfields, failure)
			}
		}
	}
	if failures < 1000 {
		t.Fatalf("only %d texts failed", failures)
	}
}
