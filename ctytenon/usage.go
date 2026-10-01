package ctytenon

import "fmt"

// usagePanic reports a usage error, a defect in the calling program such as
// asking for the tenon type of cty.NilType, as tenon does: with a message that
// begins "tenon: usage: ", so that every usage error reads alike.
func usagePanic(format string, args ...any) {
	panic("tenon: usage: " + fmt.Sprintf(format, args...))
}

// internalPanic reports a defect in this package rather than in the calling
// program, as tenon does: an invariant that the implementation keeps and did
// not, with a message that begins "ctytenon: internal: ".
func internalPanic(format string, args ...any) {
	panic("ctytenon: internal: " + fmt.Sprintf(format, args...))
}
