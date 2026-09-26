package tenon

import (
	"slices"
	"strings"
	"unicode/utf8"
)

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

// manyDiagnostics is the most diagnostics a diagnosticLookup compares one
// against. A value fails in a handful of places, among which a scan is
// quickest and allocates nothing; but one document can fail in every member
// it holds, and comparing each failure with every failure recorded costs the
// square of them: 20,000 of them took 7 seconds to collect.
const manyDiagnostics = 16

// diagnosticLookup says whether a diagnostic is among those a list holds so
// far, by comparing them while the list is short and by a set of their keys
// once it is not. The list only grows while a diagnosticLookup is in use, so
// the set, once made, needs only the diagnostics the list gains after it.
type diagnosticLookup struct {
	set map[string]struct{}
	n   int    // how many of the list's diagnostics the set holds
	buf []byte // the key of the diagnostic being looked up, kept to be reused
}

// holds reports whether d is in list, which holds the diagnostics the
// previous calls saw and perhaps more at its end.
func (l *diagnosticLookup) holds(list []Diagnostic, d Diagnostic) bool {
	if l.set == nil {
		if len(list) <= manyDiagnostics {
			return slices.ContainsFunc(list, d.Equal)
		}
		l.set = make(map[string]struct{}, 2*len(list))
	}
	for _, h := range list[l.n:] {
		l.set[diagnosticKey(h)] = struct{}{}
	}
	l.n = len(list)
	// The key of what is looked up is built in a buffer of its own, which a
	// lookup of a map by a string of bytes does not copy.
	l.buf = appendDiagnostic(l.buf[:0], d)
	_, ok := l.set[string(l.buf)]
	return ok
}

// diagnosticKey is the encoding of d, which two diagnostics share exactly
// when Equal reports them the same: the encoding holds the code, the message
// and each step of the path, and it writes a number key canonically. A key
// carries no marks, which Index refuses, so there are none to leave out.
func diagnosticKey(d Diagnostic) string { return string(appendDiagnostic(nil, d)) }

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
		case !utf8.ValidString(d.Message):
			usagePanic("ErrorVal: diagnostic %d has a message that is not valid UTF-8", i)
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
