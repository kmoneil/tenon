package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ruleID matches a bare rule identifier.
var ruleID = regexp.MustCompile(`^[A-Z]{2}-[0-9]{3}$`)

// Manifest is what the manifest file holds: the specification's version and
// its rules.
type Manifest struct {
	Version string
	Rules   []Rule
}

// specVersionPattern matches a version as the specification writes it.
var specVersionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]*$`)

// encodeManifest renders the manifest file: a JSON object holding the
// specification's version and the rule list, one rule per line so that changes
// diff cleanly.
func encodeManifest(m Manifest) ([]byte, error) {
	var b bytes.Buffer
	version, err := json.Marshal(m.Version)
	if err != nil {
		return nil, err
	}
	b.WriteString("{\n  \"version\": ")
	b.Write(version)
	b.WriteString(",\n  \"rules\": [")
	for i, r := range m.Rules {
		line, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("\n    ")
		b.Write(line)
	}
	b.WriteString("\n  ]\n}\n")
	return b.Bytes(), nil
}

// decodeManifest parses a manifest file. It rejects unknown fields, trailing
// data, a missing or malformed version, malformed or duplicate identifiers,
// and areas that disagree with their identifiers.
func decodeManifest(data []byte) (Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var file struct {
		Version string `json:"version"`
		Rules   []Rule `json:"rules"`
	}
	if err := dec.Decode(&file); err != nil {
		return Manifest{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return Manifest{}, errors.New("unexpected data after the manifest object")
	}
	if !specVersionPattern.MatchString(file.Version) {
		return Manifest{}, fmt.Errorf("malformed or missing specification version %q", file.Version)
	}
	seen := make(map[string]bool, len(file.Rules))
	for _, r := range file.Rules {
		switch {
		case !ruleID.MatchString(r.ID):
			return Manifest{}, fmt.Errorf("malformed rule identifier %q", r.ID)
		case r.Area != r.ID[:2]:
			return Manifest{}, fmt.Errorf("rule %s has area %q", r.ID, r.Area)
		case seen[r.ID]:
			return Manifest{}, fmt.Errorf("rule %s is listed twice", r.ID)
		}
		seen[r.ID] = true
	}
	return Manifest{Version: file.Version, Rules: file.Rules}, nil
}

// checkPermanence enforces that rule identifiers are permanent: every rule in
// old must still exist in current, and a withdrawn rule must stay withdrawn.
func checkPermanence(old, current []Rule) error {
	byID := make(map[string]Rule, len(current))
	for _, r := range current {
		byID[r.ID] = r
	}
	var msgs []string
	for _, o := range old {
		r, ok := byID[o.ID]
		switch {
		case !ok:
			msgs = append(msgs, fmt.Sprintf("rule %s has disappeared from the specification; withdraw it instead", o.ID))
		case o.Withdrawn && !r.Withdrawn:
			msgs = append(msgs, fmt.Sprintf("rule %s was withdrawn and cannot be reinstated; define a new rule instead", o.ID))
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return errors.New("rule identifiers are permanent:\n  " + strings.Join(msgs, "\n  "))
}

// describeChanges lists how current differs from old, for reporting a stale
// manifest.
func describeChanges(old, current []Rule) string {
	byID := make(map[string]Rule, len(old))
	for _, r := range old {
		byID[r.ID] = r
	}
	var lines []string
	for _, r := range current {
		o, ok := byID[r.ID]
		switch {
		case !ok:
			lines = append(lines, "  added "+r.ID)
		case o != r:
			lines = append(lines, fmt.Sprintf("  changed %s: withdrawn %t -> %t, outline %t -> %t",
				r.ID, o.Withdrawn, r.Withdrawn, o.Outline, r.Outline))
		}
	}
	if len(lines) == 0 {
		return "  no rule changed; the file's formatting or order differs"
	}
	return strings.Join(lines, "\n")
}

// summarize describes a rule list in one line.
func summarize(rules []Rule) string {
	var outline, withdrawn int
	for _, r := range rules {
		if r.Outline {
			outline++
		}
		if r.Withdrawn {
			withdrawn++
		}
	}
	return fmt.Sprintf("%d rules: %d outline, %d withdrawn", len(rules), outline, withdrawn)
}
