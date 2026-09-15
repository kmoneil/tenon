package tenon

// Diagnostic describes one problem with data: a stable code for programs and a
// message for people.
type Diagnostic struct {
	Code    Code
	Message string
}
