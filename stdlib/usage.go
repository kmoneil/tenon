package stdlib

import "fmt"

// internalPanic reports a defect in this package's own program: an
// invariant that it keeps and did not. It panics with a message that begins
// "tenon/stdlib: internal: ", and a caller can do nothing about it but
// report it.
func internalPanic(format string, args ...any) {
	panic("tenon/stdlib: internal: " + fmt.Sprintf(format, args...))
}
