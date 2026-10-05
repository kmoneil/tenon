package stdlib_test

import (
	"fmt"

	"github.com/kmoneil/tenon"
	"github.com/kmoneil/tenon/stdlib"
)

// A host offers its users the library's functions by the names its language
// gives them, and calls each with tenon.Call under the policy its language
// converts by. What a function answers is the specification's: exact
// numbers, an answer from what is known where an argument is not known
// yet, and a failure with a code where there is none.
func Example() {
	functions := map[string]tenon.Function{
		"range":    stdlib.RangeFunc,
		"merge":    stdlib.MergeFunc,
		"contains": stdlib.ContainsFunc,
	}
	call := func(name string, args ...tenon.Value) {
		fmt.Println(name+":", tenon.Call(functions[name], args, tenon.Unsafe))
	}
	n := tenon.NumberFromText

	// Every element is exact, so the steps do not drift.
	call("range", n("0"), n("1"), n("0.1"))
	// A map not known yet leaves the keys open, but not what is known.
	tier := tenon.Map(tenon.StringType(), map[string]tenon.Value{"tier": tenon.String("web")})
	call("merge", tenon.Unknown(tenon.MapType(tenon.StringType())), tier)
	// A language's untyped null is found among nulls.
	null := tenon.Narrow(tenon.Pending(tenon.Any()), tenon.NullOnly())
	call("contains", tenon.List(tenon.StringType(), tenon.String("a"), tenon.Null(tenon.StringType())), null)
	// A result past its bound fails before any of it is made.
	call("range", n("5000"))
	// Output:
	// range: list(number)[0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9]
	// merge: unknown(map(string), not null, length >= 1)
	// contains: true
	// range: error(function.too_large: "Range: from 0 to 5000 by 1 is more than 1024 elements, the most it makes" at .[0])
}
