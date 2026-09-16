package tenon

import "strings"

// Diagnostic describes one problem with data: a stable code for programs, a
// message for people, and the path locating the problem within the value that
// carries it. The path is empty when the problem is the value itself.
type Diagnostic struct {
	Code    Code
	Message string
	Path    Path
}

// Equal reports whether d and e are the same diagnostic: the same code, the
// same message, and the same path. Diagnostics are compared this way rather
// than with ==, because a path holds a pointer.
func (d Diagnostic) Equal(e Diagnostic) bool {
	return d.Code == e.Code && d.Message == e.Message && d.Path.Equal(e.Path)
}

// ErrorVal returns an error value carrying diags. Data that is wrong produces
// an error value like this one rather than a panic, and operations on it carry
// its diagnostics forward.
//
// A message is shown to people, so it must not show what a redacting mark
// withholds. Build a message from a value's String, which puts a placeholder
// in place of such contents, rather than from what the value holds.
//
// ErrorVal panics if diags is empty, if a diagnostic has no message, or if a
// code is not an area and a name of lowercase letters, digits and underscores,
// joined by dots.
func ErrorVal(diags ...Diagnostic) Value {
	for i, d := range diags {
		switch {
		case !validCode(d.Code):
			usagePanic("ErrorVal: diagnostic %d has code %q, which is not an area and a name joined by a dot", i, string(d.Code))
		case d.Message == "":
			usagePanic("ErrorVal: diagnostic %d needs a message", i)
		}
	}
	return errorValue(diags...)
}

// validCode reports whether c is a diagnostic code: parts of lowercase
// letters, digits and underscores, joined by dots, at least two of them.
func validCode(c Code) bool {
	parts := strings.Split(string(c), ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !validCodePart(part) {
			return false
		}
	}
	return true
}

// validCodePart reports whether s is one part of a diagnostic code.
func validCodePart(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}
