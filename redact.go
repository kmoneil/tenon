package tenon

import "strings"

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
// withhold: the identifiers of the marks, as in redacted("secret").
func writeRedacted(b *strings.Builder, ms []Mark) {
	b.WriteString("redacted(")
	writeIdentifiers(b, ms)
	b.WriteByte(')')
}

// redactedText returns the placeholder that writeRedacted writes.
func redactedText(ms []Mark) string {
	var b strings.Builder
	writeRedacted(&b, ms)
	return b.String()
}

// plain returns n as a message shows it: with no mark written out, at any
// depth, since a mark that does not redact cannot change a message (MK-005).
// A value the display form shows as a placeholder, one that is not an error
// and carries a redacting mark, names those marks alone and shows nothing it
// holds (MK-011), so it comes back as it is, and so does whatever carries no
// mark and holds none.
func (n *node) plain() *node {
	if !n.isMarked() || n.state != stateError && n.redactingMarks() != nil {
		return n
	}
	nn := *n
	nn.marks = nil
	if n.markedWithin {
		nn.markedWithin = false
		switch data := n.data.(type) {
		case []Value:
			members := make([]Value, len(data))
			for i, m := range data {
				members[i] = Value{m.n.plain()}
				nn.markedWithin = nn.markedWithin || members[i].n.isMarked()
			}
			nn.data = members
		case []mapEntry:
			entries := make([]mapEntry, len(data))
			for i, e := range data {
				entries[i] = mapEntry{key: e.key, val: Value{e.val.n.plain()}}
				nn.markedWithin = nn.markedWithin || entries[i].val.n.isMarked()
			}
			nn.data = entries
		}
	}
	return &nn
}
