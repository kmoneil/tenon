package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// codeSpec names codes in every place the conventions distinguish: rules,
// notes, fences, an outline section and an appendix.
var codeSpec = fixture(
	"## 2. Alpha",
	"",
	"`[AA-001]` Fails with `aa.one`, or with `aa.two` where it",
	"wraps onto a line of its own: `aa.one` again.",
	"",
	"> **Rationale.** A note belongs to the rule before it: `aa.three`.",
	"",
	"| Code | Condition |",
	"| ---- | --------- |",
	"| `aa.two` | a table belongs to the rule before it too |",
	"",
	"```",
	"`aa.fenced` is not a code",
	"```",
	"",
	"`[AA-002]` Also fails with `aa.one`, and names `big.Rat`, which is no code.",
	"",
	"## 3. Beta *(outline)*",
	"",
	"`[BB-001]` An outline names `bb.outlined`.",
	"",
	"## Appendix A: Notes",
	"",
	"An appendix names `bb.appended`.",
)

func TestNamedCodes(t *testing.T) {
	got, err := namedCodes(codeSpec)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"aa.one":   {"AA-001", "AA-002"},
		"aa.two":   {"AA-001"},
		"aa.three": {"AA-001"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("namedCodes:\n got %v\nwant %v", got, want)
	}

	// A code is named within a rule, or it has no rule to be listed under.
	_, err = namedCodes(fixture("## 2. Alpha", "", "Before any rule, `aa.one`.", "", "`[AA-001]` One.", "",
		"## 3. Beta", "", "A new section starts afresh: `aa.two`."))
	if err == nil || !strings.Contains(err.Error(), "line 14: code aa.one is named outside any rule") ||
		!strings.Contains(err.Error(), "line 20: code aa.two is named outside any rule") {
		t.Errorf("codes outside rules: error %v", err)
	}
}

// registryFixture writes a registry declaring codes and returns its path.
func registryFixture(t *testing.T, dir string, codes ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("package tenon\n\n// Code is a code; \"xx.comment\" in a comment is not one.\ntype Code string\n\nconst (\n")
	for i, c := range codes {
		b.WriteString("\tCode" + string(rune('A'+i)) + " Code = \"" + c + "\"\n")
	}
	b.WriteString(")\n")
	path := filepath.Join(dir, "codes.go")
	writeFile(t, path, []byte(b.String()))
	return path
}

func TestRegistryCodes(t *testing.T) {
	dir := t.TempDir()
	got, err := registryCodes(registryFixture(t, dir, "bb.two", "aa.one"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"aa.one", "bb.two"}; !reflect.DeepEqual(got, want) {
		t.Errorf("registryCodes = %q, want %q", got, want)
	}
	if _, err := registryCodes(registryFixture(t, dir, "aa.one", "aa.one")); err == nil || !strings.Contains(err.Error(), "declares the code aa.one twice") {
		t.Errorf("a duplicate code: error %v", err)
	}
}

func TestAppendix(t *testing.T) {
	named := map[string][]string{"aa.one": {"AA-001", "AA-002"}, "aa.two": {"AA-001"}}
	got, err := appendix([]string{"aa.one", "aa.two"}, named)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []string{
		appendixHeading + "\n\n",
		"| `aa.one` | `[AA-001]`, `[AA-002]` |\n| `aa.two` | `[AA-001]` |\n",
	} {
		if !strings.Contains(got, row) {
			t.Errorf("appendix lacks %q:\n%s", row, got)
		}
	}

	// The registry and the rules must agree, both ways.
	_, err = appendix([]string{"aa.one", "aa.zero"}, named)
	if err == nil || !strings.Contains(err.Error(), "AA-001 names aa.two, which the registry does not declare") ||
		!strings.Contains(err.Error(), "the registry declares aa.zero, which no rule names") {
		t.Errorf("a disagreement: error %v", err)
	}
}

func TestWithAppendix(t *testing.T) {
	text := appendixHeading + "\n\nNew.\n"
	for _, tt := range []struct{ name, src, want string }{
		{"appended", "# Spec\n", "# Spec\n\n" + text},
		{"replaced at the end", "# Spec\n\n" + appendixHeading + "\n\nOld.\n", "# Spec\n\n" + text},
		{"replaced before another appendix", "# Spec\n\n" + appendixHeading + "\n\nOld.\n\n## Appendix E: More\n",
			"# Spec\n\n" + text + "\n## Appendix E: More\n"},
	} {
		if got := string(withAppendix([]byte(tt.src), text)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestRunCodes(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.md")
	manifest := filepath.Join(dir, "rules.json")
	active := filepath.Join(dir, "active-areas.txt")
	registry := registryFixture(t, dir, "aa.one")
	writeFile(t, active, nil)
	t.Setenv("TENON_SPEC", "")
	c := cli{t}
	noSpec := []string{"check", "-manifest", manifest, "-active", active, "-cover", filepath.Join(dir, "cover"), "-codes", registry}
	check := append([]string{noSpec[0], "-spec", spec}, noSpec[1:]...)

	body := []string{"## 2. Alpha", "", "`[AA-001]` Fails with `aa.one`."}
	writeFile(t, spec, fixture(body...))
	c.ok("wrote", "manifest", "-spec", spec, "-manifest", manifest)

	// Until the appendix is written, check finds it stale.
	c.fails("stale; regenerate it with `make codes`", check...)
	c.ok("wrote the appendix", "codes", "-spec", spec, "-codes", registry)
	c.ok("already up to date", "codes", "-spec", spec, "-codes", registry)
	c.ok("diagnostic codes up to date (1 codes)", check...)

	// Editing the appendix by hand makes it stale again.
	edited := strings.Replace(string(readFile(t, spec)), "| `aa.one` |", "| `aa.one`, by hand |", 1)
	writeFile(t, spec, []byte(edited))
	c.fails("appendix of diagnostic codes in "+spec+" is stale", check...)
	c.ok("wrote the appendix", "codes", "-spec", spec, "-codes", registry)

	// A code in the registry that no rule names, or the other way about, is
	// refused by both commands.
	registryFixture(t, dir, "aa.one", "aa.two")
	c.fails("the registry declares aa.two, which no rule names", check...)
	c.fails("the registry declares aa.two, which no rule names", "codes", "-spec", spec, "-codes", registry)
	registryFixture(t, dir)
	c.fails("AA-001 names aa.one, which the registry does not declare", check...)

	// Without a specification, check skips the codes, and codes refuses to run.
	c.ok("skipping the diagnostic code check", noSpec...)
	c.fails("set TENON_SPEC", "codes", "-codes", registry)
}
