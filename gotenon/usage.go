package gotenon

import "fmt"

// usagePanic reports a usage error, a defect in the calling program such as a
// struct tag that does not parse, as the tenon package does: with a message
// that begins "tenon: usage: ", so that every usage error reads alike.
func usagePanic(format string, args ...any) {
	panic("tenon: usage: " + fmt.Sprintf(format, args...))
}
