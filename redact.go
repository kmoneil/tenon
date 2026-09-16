package tenon

import (
	"strconv"
	"strings"
)

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
func (n *node) redactingMarks() []Mark { return redactingOf(n.markList()) }

// writeRedacted writes the placeholder that stands in for what redacting marks
// withhold: the identifiers of the marks, as in redacted("secret"). ms must be
// sorted by identifier, so that an identifier two marks share is written once.
func writeRedacted(b *strings.Builder, ms []Mark) {
	b.WriteString("redacted(")
	last := ""
	for i, m := range ms {
		id := m.MarkID()
		if i > 0 {
			if id == last {
				continue
			}
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(id))
		last = id
	}
	b.WriteByte(')')
}

// redactedText returns the placeholder that writeRedacted writes.
func redactedText(ms []Mark) string {
	var b strings.Builder
	writeRedacted(&b, ms)
	return b.String()
}
