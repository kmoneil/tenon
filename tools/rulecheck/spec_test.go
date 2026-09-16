package main

import (
	"reflect"
	"strings"
	"testing"
)

// markdown joins lines into a specification fixture.
func markdown(lines ...string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

// fixture prefixes body with a twelve-line preamble holding the version and
// the area table. The table lists BB before AA, so that ordering by table
// position is observable.
func fixture(body ...string) []byte {
	preamble := []string{
		"# Fixture",
		"**Version:** 0.0.1-fixture",
		"",
		"## 1. Introduction",
		"",
		"### 1.4 Rule identifiers",
		"",
		"| Prefix | Area |",
		"| ------ | ---- |",
		"| `BB` | Beta |",
		"| `AA` | Alpha |",
		"",
	}
	return markdown(append(preamble, body...)...)
}

// sampleSpec exercises every convention: definitions and references, a
// reference wrapped to the start of a line, a code fence, a withdrawn rule, an
// outline section and an appendix.
var sampleSpec = fixture(
	"## 2. Alpha",
	"",
	"`[AA-002]` The second rule, defined first. Its text wraps so that a",
	"`[AA-001]` reference begins a line without beginning a paragraph.",
	"",
	"`[AA-001]` The first rule.",
	"",
	"> **Rationale.** `[AA-001]` is referenced from a note.",
	"",
	"```",
	"`[AA-009]` inside a code fence is ignored.",
	"```",
	"",
	"## 3. Beta",
	"",
	"`[BB-001]` *(withdrawn)* A withdrawn rule.",
	"",
	"`[BB-002]` Refers to `[BB-003]`, which an outline section defines.",
	"",
	"## 4. Gamma *(outline)*",
	"",
	"`[BB-003]` A rule defined in an outline section.",
	"",
	"- `[BB-004]` is referenced only, and only from an outline section.",
	"",
	"## Appendix A: Notes",
	"",
	"`[BB-005]` begins a paragraph in an appendix, which mentions `[AA-001]`.",
)

func TestParseSpec(t *testing.T) {
	got, err := parseSpec(sampleSpec)
	if err != nil {
		t.Fatal(err)
	}
	want := []Rule{
		{ID: "BB-001", Area: "BB", Withdrawn: true},
		{ID: "BB-002", Area: "BB"},
		{ID: "BB-003", Area: "BB", Outline: true},
		{ID: "BB-004", Area: "BB", Outline: true},
		{ID: "BB-005", Area: "BB", Outline: true},
		{ID: "AA-001", Area: "AA"},
		{ID: "AA-002", Area: "AA"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseSpec:\n got %+v\nwant %+v", got, want)
	}
}

func TestParseSpecProblems(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
		want []string // substrings of the error, in order
	}{
		{
			name: "rule defined twice",
			src:  fixture("## 2. Alpha", "", "`[AA-001]` One.", "", "`[AA-001]` Again."),
			want: []string{"line 17: rule AA-001 is already defined at line 15"},
		},
		{
			name: "normative reference to an undefined rule",
			src:  fixture("## 2. Alpha", "", "`[AA-001]` Refers to `[AA-002]`."),
			want: []string{"line 15: rule AA-002 is referenced but never defined"},
		},
		{
			name: "unknown area",
			src:  fixture("## 2. Alpha", "", "`[ZZ-001]` Wrong area."),
			want: []string{"line 15: rule ZZ-001 has unknown area ZZ"},
		},
		{
			name: "identifier without backticks",
			src:  fixture("## 2. Alpha", "", "[AA-001] Not canonical."),
			want: []string{"line 15: AA-001 is not written as `[AA-001]`"},
		},
		{
			name: "identifier without brackets",
			src:  fixture("## 2. Alpha", "", "`[AA-001]` Refers to `AA-001`."),
			want: []string{"line 15: AA-001 is not written as `[AA-001]`"},
		},
		{
			name: "no area table",
			src:  markdown("## 2. Alpha", "", "`[AA-001]` One."),
			want: []string{"no area table found"},
		},
		{
			name: "area listed twice",
			src:  markdown("### Rule identifiers", "", "| `AA` | Alpha |", "| `AA` | Again |", "", "## 2. Alpha", "", "`[AA-001]` One."),
			want: []string{"line 4: area AA is listed twice"},
		},
		{
			name: "every problem reported in line order",
			src: fixture("## 2. Alpha", "",
				"`[AA-001]` Refers to `[AA-004]`.", "",
				"[AA-003] Not canonical.", "",
				"`[AA-002]` Refers to `[AA-005]`."),
			want: []string{
				"3 problem(s)",
				"line 15: rule AA-004 is referenced but never defined",
				"line 17: AA-003 is not written as `[AA-003]`",
				"line 19: rule AA-005 is referenced but never defined",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := parseSpec(tt.src)
			if err == nil {
				t.Fatalf("parseSpec succeeded with %+v, want an error", rules)
			}
			msg, at := err.Error(), 0
			for _, w := range tt.want {
				i := strings.Index(msg[at:], w)
				if i < 0 {
					t.Fatalf("error %q lacks %q after offset %d", msg, w, at)
				}
				at += i + len(w)
			}
		})
	}
}

func TestSpecVersion(t *testing.T) {
	if got, err := specVersion(fixture("## 2. Alpha")); got != "0.0.1-fixture" || err != nil {
		t.Errorf("specVersion = %q, %v", got, err)
	}
	for _, tt := range []struct{ name, src, want string }{
		{"none", "# Spec\n\n## 1. Introduction\n", "no **Version:** line"},
		{"after the first section", "# Spec\n\n## 1. Introduction\n\n**Version:** 1.0\n", "no **Version:** line"},
		{"malformed", "# Spec\n**Version:** 1.0/2\n", "malformed specification version"},
	} {
		if _, err := specVersion([]byte(tt.src)); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error %v, want one containing %q", tt.name, err, tt.want)
		}
	}
}
