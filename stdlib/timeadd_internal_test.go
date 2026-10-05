package stdlib

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

// TestTimeAddAgreesWithGoTime holds TimeAdd's arithmetic and its text to
// Go's time package where Go's is exact: random timestamps, fractions of a
// second included, and durations of whole numbers and short fractions of
// each unit, whose sums Go holds in int64 nanoseconds without rounding,
// written by RFC3339Nano, which drops a fraction's trailing zeros as TimeAdd
// does. TimeAdd's days are held to Go's proleptic calendar by it too.
func TestTimeAddAgreesWithGoTime(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261005, 41))
	units := []string{"ns", "us", "\U000000B5s", "ms", "s", "m", "h"}
	compared := 0
	for range 20000 {
		year, month := 1+rng.IntN(9998), 1+rng.IntN(12)
		day := 1 + rng.IntN(daysIn(month, year))
		frac := ""
		if rng.IntN(2) == 0 {
			frac = "." + fmt.Sprint(rng.IntN(1000000000))
		}
		offset := ""
		switch rng.IntN(3) {
		case 0:
			offset = "Z"
		case 1:
			offset = fmt.Sprintf("+%02d:%02d", rng.IntN(24), rng.IntN(60))
		default:
			offset = fmt.Sprintf("-%02d:%02d", rng.IntN(24), rng.IntN(60))
		}
		stamp := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d%s%s", year, month, day, rng.IntN(24), rng.IntN(60), rng.IntN(60), frac, offset)
		var d strings.Builder
		if rng.IntN(3) == 0 {
			d.WriteString("-")
		}
		for range 1 + rng.IntN(3) {
			u := units[rng.IntN(len(units))]
			d.WriteString(fmt.Sprint(rng.IntN(100000)))
			if u != "ns" && rng.IntN(2) == 0 {
				d.WriteString("." + fmt.Sprint(rng.IntN(1000)))
			}
			d.WriteString(u)
		}
		ts, why := parseTimestamp(stamp)
		if why != "" {
			t.Fatalf("%s: %s", stamp, why)
		}
		dur, why := parseDuration(d.String())
		if why != "" {
			t.Fatalf("%s: %s", d.String(), why)
		}
		got, ok := rfc3339(ts.instant().plus(dur), ts.offset)
		gt, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			t.Fatalf("Go reads %s: %v", stamp, err)
		}
		gd, err := time.ParseDuration(d.String())
		if err != nil {
			t.Fatalf("Go reads %s: %v", d.String(), err)
		}
		want := gt.Add(gd)
		if y := want.Year(); y < 0 || y > 9999 {
			if ok {
				t.Fatalf("%s + %s: Go's year is %d, and TimeAdd answers %s", stamp, d.String(), y, got)
			}
			continue
		}
		compared++
		if w := want.Format(time.RFC3339Nano); !ok || got != w {
			t.Fatalf("%s + %s = %s, %v; Go %s", stamp, d.String(), got, ok, w)
		}
	}
	if compared < 15000 {
		t.Fatalf("only %d compared", compared)
	}
}
