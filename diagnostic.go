package tenon

// Diagnostic describes one problem with data: a stable code for programs, a
// message for people, and the path locating the problem within the value that
// carries it. The path is empty when the problem is the value itself.
type Diagnostic struct {
	Code    Code
	Message string
	Path    Path
}
