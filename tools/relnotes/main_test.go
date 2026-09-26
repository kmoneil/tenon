package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const fixture = `# Changelog

## Unreleased

- Not yet.

## 0.2.0 (2026-01-02)

Second.

### Fixed

- A fix.


## 0.1.0 (2026-01-01)

First.
`

func TestSection(t *testing.T) {
	for _, tt := range []struct{ version, want string }{
		{"0.2.0", "Second.\n\n### Fixed\n\n- A fix.\n"},
		{"v0.2.0", "Second.\n\n### Fixed\n\n- A fix.\n"},
		{"0.1.0", "First.\n"},
	} {
		got, err := section(fixture, tt.version)
		if err != nil || got != tt.want {
			t.Errorf("section(%q) = %q, %v; want %q", tt.version, got, err, tt.want)
		}
	}
	for _, tt := range []struct{ changelog, version, want string }{
		{fixture, "0.3.0", "no section for 0.3.0"},
		{fixture, "0.2", "no section for 0.2"},
		{fixture, "Unreleased", "no section for Unreleased"},
		{fixture, "v", "no version given"},
		{"## 0.1.0 (2026-01-01)\n\n## 0.0.1 (2025-12-31)\n\nx\n", "0.1.0", "the section for 0.1.0 is empty"},
		{fixture + "\n## 0.1.0 (2026-01-03)\n\nAgain.\n", "0.1.0", "two sections for 0.1.0"},
	} {
		if _, err := section(tt.changelog, tt.version); err == nil || err.Error() != tt.want {
			t.Errorf("section(%q) failed with %v, want %q", tt.version, err, tt.want)
		}
	}
}

// TestChangelogSections holds the module's own changelog to what the release
// workflow needs of it: every released version has a section with notes in it.
func TestChangelogSections(t *testing.T) {
	data, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	headings := regexp.MustCompile(`(?m)^## (\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?) \(\d{4}-\d{2}-\d{2}\)$`).FindAllStringSubmatch(string(data), -1)
	if len(headings) < 6 {
		t.Fatalf("CHANGELOG.md has %d release sections, want at least the six up to 0.6.0", len(headings))
	}
	for _, h := range headings {
		notes, err := section(string(data), h[1])
		if err != nil || strings.TrimSpace(notes) == "" {
			t.Errorf("the notes of %s: %q, %v", h[1], notes, err)
		}
	}
}
