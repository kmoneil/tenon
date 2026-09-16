package tenon

import "fmt"

// usagePanic reports a usage error: a defect in the calling program, such as
// asking a list type for its attributes. Usage errors are never represented as
// values; they panic with a message that begins "tenon: usage: ".
func usagePanic(format string, args ...any) {
	panic("tenon: usage: " + fmt.Sprintf(format, args...))
}

// internalPanic reports a defect in this package rather than in the calling
// program: an invariant that the implementation keeps and did not. It panics
// with a message that begins "tenon: internal: ", so that the two kinds of
// defect are told apart by whoever reads the panic. A caller can do nothing
// about one of these but report it.
func internalPanic(format string, args ...any) {
	panic("tenon: internal: " + fmt.Sprintf(format, args...))
}
