package tenon

import "fmt"

// usagePanic reports a usage error: a defect in the calling program, such as
// asking a list type for its attributes. Usage errors are never represented as
// values; they panic with a message that begins "tenon: usage: ".
func usagePanic(format string, args ...any) {
	panic("tenon: usage: " + fmt.Sprintf(format, args...))
}
