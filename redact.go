package tenon

// redactingOf returns the redacting marks among ms, in the order given, or nil
// when there are none.
func redactingOf(ms []Mark) []Mark {
	var out []Mark
	for _, m := range ms {
		if m.Redacting() {
			out = append(out, m)
		}
	}
	return out
}

// redactingMarks returns the redacting marks n carries, or nil when it carries
// none. What such a value holds, what its range says and whether it is null
// are withheld wherever it is described.
func (n *node) redactingMarks() []Mark {
	if n.marks == nil {
		return nil
	}
	return n.marks.redactingMarks()
}

// withholds reports whether n carries a redacting mark, which withholds all
// that a panic's message would say of n but its marks: its type, whether it is
// null, known or pending, and what it holds. An error value is described by
// its diagnostics, which withheld what they had to when they were made, as its
// display is.
func (n *node) withholds() bool {
	return n.state != stateError && n.redactingMarks() != nil
}

// redactedBy names a value carrying the redacting marks ms for a panic's
// message, by the identifiers its display gives, as in
// `a value redacted by "secret"`.
func redactedBy(ms []Mark) string {
	var b textWriter
	b.WriteString("a value redacted by ")
	writeIdentifiers(&b, ms)
	return b.String()
}

// withheldReason ends the message of a usage panic whose reason would say of
// a value what its redacting marks withhold, in place of the reason.
const withheldReason = ", for a reason its redacting marks withhold; unmark the value to see the reason"

// writeRedacted writes the placeholder that stands in for what redacting marks
// withhold: the identifiers of the marks, as in redacted("secret").
func writeRedacted(b *textWriter, ms []Mark) {
	b.WriteString("redacted(")
	writeIdentifiers(b, ms)
	b.WriteByte(')')
}

// redactedText returns the placeholder that writeRedacted writes.
func redactedText(ms []Mark) string {
	var b textWriter
	writeRedacted(&b, ms)
	return b.String()
}
