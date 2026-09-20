package tenon_test

import (
	"fmt"

	"github.com/kmoneil/tenon"
)

// service builds the value describing one service. The token is marked, so
// neither a display form nor a diff ever shows it.
func service(replicas int64, address, token tenon.Value) tenon.Value {
	return tenon.ObjectVal(map[string]tenon.Value{
		"name":     tenon.String("web"),
		"replicas": tenon.NumberFromInt(replicas),
		"address":  address,
		"token":    tenon.WithMarks(token, sensitive{}),
	})
}

// A plan engine: what a change will do is shown before it is done, with the
// parts that are not knowable yet left honestly open and the secrets left
// out.
func Example_planAndApply() {
	deployed := service(3, tenon.String("10.0.0.7"), tenon.String("old-token"))

	// What the configuration asks for. The address will not be known until
	// the load balancer is replaced, but its network is settled.
	planned := service(5,
		tenon.Narrow(tenon.Unknown(tenon.StringType()), tenon.NotNull(), tenon.StringPrefix("10.")),
		tenon.String("new-token"))

	fmt.Println("plan:")
	for _, change := range tenon.Diff(deployed, planned) {
		fmt.Println("  ", change)
	}

	// Applying settles what was not known. The plan promised the address
	// would start with 10., and narrowing is monotone, so a value that
	// breaks the promise is a contradiction rather than a surprise.
	applied := service(5, tenon.String("10.0.0.9"), tenon.String("new-token"))
	fmt.Println("applied:")
	for _, change := range tenon.Diff(planned, applied) {
		fmt.Println("  ", change)
	}

	broken := tenon.Narrow(planned.Attribute("address"), tenon.StringPrefix("192.168."))
	fmt.Println("outside the plan:", broken.IsError(), broken.Diagnostics()[0].Code)
	// Output:
	// plan:
	//    ~ .address: "10.0.0.7" -> unknown(string, not null, prefix "10.", length >= 3)
	//    ~ .replicas: 3 -> 5
	//    ~ .token: redacted("acme/sensitive") -> redacted("acme/sensitive")
	// applied:
	//    ~ .address: unknown(string, not null, prefix "10.", length >= 3) -> "10.0.0.9"
	// outside the plan: true range.contradiction
}
