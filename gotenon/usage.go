package gotenon

import (
	"fmt"
	"strings"
)

// usagePanic reports a usage error, a defect in the calling program such as a
// struct tag that does not parse, as the tenon package does: with a message
// that begins "tenon: usage: ", so that every usage error reads alike.
func usagePanic(format string, args ...any) {
	panic(usagePrefix + fmt.Sprintf(format, args...))
}

// usagePrefix begins the message of every usage error.
const usagePrefix = "tenon: usage: "

// unlessUsageError runs f and reports whether it returned: a usage error it
// panics with is caught and reported as false, and any other panic, a defect
// rather than a mistake of the caller's, goes on.
func unlessUsageError(f func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if msg, usage := r.(string); !usage || !strings.HasPrefix(msg, usagePrefix) {
				panic(r)
			}
			ok = false
		}
	}()
	f()
	return true
}
