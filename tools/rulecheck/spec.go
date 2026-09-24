package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Rule is one entry of the rule manifest.
type Rule struct {
	ID        string `json:"id"`
	Area      string `json:"area"`
	Withdrawn bool   `json:"withdrawn"`
	Outline   bool   `json:"outline"`
}

const (
	areaHeading     = "Rule identifiers"
	outlineMarker   = "*(outline)*"
	withdrawnMarker = "*(withdrawn)*"
)

var (
	// canonicalID matches a rule identifier as the specification writes it,
	// backticks included, and captures the bare identifier.
	canonicalID = regexp.MustCompile("`\\[([A-Z]{2}-[0-9]{3})\\]`")
	// idShaped matches anything shaped like a rule identifier, so that an
	// identifier written in another form is reported instead of missed.
	idShaped        = regexp.MustCompile(`\b[A-Z]{2}-[0-9]{3}\b`)
	headingLine     = regexp.MustCompile(`^#{1,6} `)
	numberedSection = regexp.MustCompile(`^## [0-9]+\. `)
	// areaRow matches a row of the area table and captures the prefix.
	areaRow = regexp.MustCompile("^\\|\\s*`([A-Z]{2})`\\s*\\|[^|]*\\|\\s*$")
)

// loadSpec reads the specification file and derives its manifest.
func loadSpec(path string) (Manifest, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	rules, err := parseSpec(src)
	if err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", path, err)
	}
	version, err := specVersion(src)
	if err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", path, err)
	}
	return Manifest{Version: version, Rules: rules}, nil
}

// versionLine matches the line that states the specification's version, as in
// **Version:** 0.1.0, and captures the version.
var versionLine = regexp.MustCompile(`^\*\*Version:\*\*\s+(\S+)\s*$`)

// specVersion returns the version that the specification states before its
// first section.
func specVersion(src []byte) (string, error) {
	for line := range strings.Lines(string(src)) {
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "## ") {
			break
		}
		if m := versionLine.FindStringSubmatch(line); m != nil {
			if !specVersionPattern.MatchString(m[1]) {
				return "", fmt.Errorf("malformed specification version %q", m[1])
			}
			return m[1], nil
		}
	}
	return "", errors.New("no **Version:** line before the first section")
}

// occurrence is one appearance of a rule identifier in the specification.
type occurrence struct {
	line       int
	normative  bool // in a normative section
	definition bool // begins its paragraph
	withdrawn  bool // a definition carrying the withdrawn marker
}

// parseSpec derives the rule manifest from the Markdown source of the
// specification, following the conventions in the package documentation.
// Rules are ordered by the position of their area in the area table, then by
// identifier.
func parseSpec(src []byte) ([]Rule, error) {
	var (
		problems    problemList
		areaOrder   = map[string]int{}
		occurrences = map[string][]occurrence{}
		normative   bool // the current section is normative
		inAreaTable bool // the current section holds the area table
		inFence     bool // inside a fenced code block
		paraStart   = true
	)
	scanner := bufio.NewScanner(bytes.NewReader(src))
	for n := 1; scanner.Scan(); n++ {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			paraStart = true
			continue
		}
		if inFence {
			// A fence is skipped whole, so a rule defined inside one would
			// be invisible to the manifest; a line that begins as a
			// definition does is refused rather than ignored, or a rule
			// could hide there. A bare mention deeper in a fence line, as a
			// grammar's comment makes, stays harmless.
			if loc := canonicalID.FindStringIndex(line); loc != nil && loc[0] == 0 {
				problems.add(n, "a rule definition inside a code fence, which the manifest does not read")
			}
			continue
		}
		heading := headingLine.MatchString(line)
		if heading {
			if strings.HasPrefix(line, "## ") {
				normative = numberedSection.MatchString(line) && !strings.Contains(line, outlineMarker)
			}
			inAreaTable = strings.Contains(line, areaHeading)
		}
		if inAreaTable {
			if m := areaRow.FindStringSubmatch(line); m != nil {
				if _, dup := areaOrder[m[1]]; dup {
					problems.add(n, "area %s is listed twice", m[1])
				} else {
					areaOrder[m[1]] = len(areaOrder)
				}
			}
		}
		for _, loc := range canonicalID.FindAllStringSubmatchIndex(line, -1) {
			definition := paraStart && !heading && loc[0] == 0
			id := line[loc[2]:loc[3]]
			occurrences[id] = append(occurrences[id], occurrence{
				line:       n,
				normative:  normative,
				definition: definition,
				withdrawn:  definition && strings.HasPrefix(strings.TrimSpace(line[loc[1]:]), withdrawnMarker),
			})
		}
		for _, m := range idShaped.FindAllString(canonicalID.ReplaceAllString(line, " "), -1) {
			problems.add(n, "%s is not written as `[%s]`", m, m)
		}
		paraStart = heading || strings.TrimSpace(line) == ""
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(areaOrder) == 0 {
		problems.add(0, "no area table found under a %q heading", areaHeading)
	}

	rules := make([]Rule, 0, len(occurrences))
	for id, occs := range occurrences {
		area := id[:2]
		if _, known := areaOrder[area]; !known {
			if len(areaOrder) > 0 {
				problems.add(occs[0].line, "rule %s has unknown area %s", id, area)
			}
			continue
		}
		var defs []occurrence
		normativeRef := 0
		for _, o := range occs {
			if o.definition {
				defs = append(defs, o)
			}
			if o.normative && normativeRef == 0 {
				normativeRef = o.line
			}
		}
		switch {
		case len(defs) > 1:
			problems.add(defs[1].line, "rule %s is already defined at line %d", id, defs[0].line)
		case len(defs) == 1:
			rules = append(rules, Rule{ID: id, Area: area, Withdrawn: defs[0].withdrawn, Outline: !defs[0].normative})
		case normativeRef > 0:
			problems.add(normativeRef, "rule %s is referenced but never defined", id)
		default:
			rules = append(rules, Rule{ID: id, Area: area, Outline: true})
		}
	}
	if err := problems.err(); err != nil {
		return nil, err
	}
	sort.Slice(rules, func(i, j int) bool {
		if ai, aj := areaOrder[rules[i].Area], areaOrder[rules[j].Area]; ai != aj {
			return ai < aj
		}
		return rules[i].ID < rules[j].ID
	})
	return rules, nil
}

// problem is one defect found in the specification. Line 0 means the defect
// has no single location.
type problem struct {
	line int
	msg  string
}

// problemList collects problems so that all of them are reported at once.
type problemList []problem

func (p *problemList) add(line int, format string, args ...any) {
	*p = append(*p, problem{line: line, msg: fmt.Sprintf(format, args...)})
}

// err returns nil if the list is empty, and otherwise an error listing every
// problem in line order.
func (p problemList) err() error {
	if len(p) == 0 {
		return nil
	}
	sort.Slice(p, func(i, j int) bool {
		if p[i].line != p[j].line {
			return p[i].line < p[j].line
		}
		return p[i].msg < p[j].msg
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%d problem(s):", len(p))
	for _, q := range p {
		if q.line > 0 {
			fmt.Fprintf(&b, "\n  line %d: %s", q.line, q.msg)
		} else {
			fmt.Fprintf(&b, "\n  %s", q.msg)
		}
	}
	return errors.New(b.String())
}
