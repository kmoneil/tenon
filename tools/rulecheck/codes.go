package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// appendixHeading is the heading of the specification's appendix of
// diagnostic codes, which the codes command writes.
const appendixHeading = "## Appendix D: diagnostic codes"

// codeToken matches a diagnostic code as the specification writes it,
// backticks included, and captures the bare code.
var codeToken = regexp.MustCompile("`([a-z][a-z0-9_]*(?:\\.[a-z][a-z0-9_]*)+)`")

// namedCodes returns, for each diagnostic code that a normative section of the
// specification names, the rules that name it, each once, in the order they
// first do. A code belongs to the rule whose definition most recently precedes
// it in its section, and one that no rule precedes is a problem.
func namedCodes(src []byte) (map[string][]string, error) {
	var (
		problems  problemList
		named     = map[string][]string{}
		normative bool
		inFence   bool
		paraStart = true
		rule      string // the rule defined most recently in this section
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
			continue
		}
		heading := headingLine.MatchString(line)
		if heading && strings.HasPrefix(line, "## ") {
			normative = numberedSection.MatchString(line) && !strings.Contains(line, outlineMarker)
			rule = ""
		}
		if loc := canonicalID.FindStringSubmatchIndex(line); loc != nil && paraStart && !heading && loc[0] == 0 {
			rule = line[loc[2]:loc[3]]
		}
		if normative {
			for _, m := range codeToken.FindAllStringSubmatch(line, -1) {
				switch code := m[1]; {
				case rule == "":
					problems.add(n, "code %s is named outside any rule", code)
				case !slices.Contains(named[code], rule):
					named[code] = append(named[code], rule)
				}
			}
		}
		paraStart = heading || strings.TrimSpace(line) == ""
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := problems.err(); err != nil {
		return nil, err
	}
	return named, nil
}

// registryCodes returns the codes that the registry file declares, the string
// constants of its const declarations, in sorted order.
func registryCodes(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	var codes []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			for _, value := range spec.(*ast.ValueSpec).Values {
				if lit, ok := value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					code, err := strconv.Unquote(lit.Value)
					if err != nil {
						return nil, err
					}
					codes = append(codes, code)
				}
			}
		}
	}
	slices.Sort(codes)
	for i := 1; i < len(codes); i++ {
		if codes[i] == codes[i-1] {
			return nil, fmt.Errorf("%s declares the code %s twice", display(path), codes[i])
		}
	}
	return codes, nil
}

// appendix renders the appendix of diagnostic codes: every code of the
// registry with the rules that name it. It fails where the registry and the
// rules disagree about which codes there are.
func appendix(registry []string, named map[string][]string) (string, error) {
	var problems problemList
	for _, code := range registry {
		if len(named[code]) == 0 {
			problems.add(0, "the registry declares %s, which no rule names", code)
		}
	}
	for code := range named {
		if !slices.Contains(registry, code) {
			problems.add(0, "%s names %s, which the registry does not declare", strings.Join(named[code], ", "), code)
		}
	}
	if err := problems.err(); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(appendixHeading + "\n\n")
	b.WriteString("Every diagnostic code that this specification defines, with the rules that\n")
	b.WriteString("name it. This appendix is generated from the reference implementation's\n")
	b.WriteString("registry of codes, and a check fails where the two disagree.\n\n")
	b.WriteString("| Code | Named by |\n| ---- | -------- |\n")
	for _, code := range registry {
		refs := make([]string, len(named[code]))
		for i, id := range named[code] {
			refs[i] = "`[" + id + "]`"
		}
		fmt.Fprintf(&b, "| `%s` | %s |\n", code, strings.Join(refs, ", "))
	}
	return b.String(), nil
}

// withAppendix returns src with its appendix of codes replaced by text, or with
// text appended where it has none.
func withAppendix(src []byte, text string) []byte {
	lines := strings.SplitAfter(string(src), "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return strings.TrimRight(l, "\n") == appendixHeading })
	if start < 0 {
		out := strings.TrimRight(string(src), "\n")
		if out != "" {
			out += "\n\n"
		}
		return []byte(out + text)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	rest := strings.Join(lines[end:], "")
	if rest != "" {
		text += "\n"
	}
	return []byte(strings.Join(lines[:start], "") + text + rest)
}

// freshAppendix returns the specification at specPath as it would be with its
// appendix of codes generated from the registry at codesPath, with the
// specification as it is.
func freshAppendix(specPath, codesPath string) (fresh, current []byte, codes int, err error) {
	current, err = os.ReadFile(specPath)
	if err != nil {
		return nil, nil, 0, err
	}
	named, err := namedCodes(current)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s: %w", specPath, err)
	}
	registry, err := registryCodes(codesPath)
	if err != nil {
		return nil, nil, 0, err
	}
	text, err := appendix(registry, named)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s and %s disagree: %w", display(specPath), display(codesPath), err)
	}
	return withAppendix(current, text), current, len(registry), nil
}

// checkCodes verifies that the specification's appendix of codes is what
// runCodes would write, or says why it cannot tell.
func checkCodes(w io.Writer, p paths) error {
	if p.spec == "" {
		fmt.Fprintln(w, "rulecheck: TENON_SPEC not set; skipping the diagnostic code check")
		return nil
	}
	fresh, current, n, err := freshAppendix(p.spec, p.codes)
	if err != nil {
		return err
	}
	if !bytes.Equal(fresh, current) {
		return fmt.Errorf("the appendix of diagnostic codes in %s is stale; regenerate it with `make codes`", display(p.spec))
	}
	fmt.Fprintf(w, "rulecheck: diagnostic codes up to date (%d codes)\n", n)
	return nil
}

// runCodes regenerates the specification's appendix of codes from the registry.
func runCodes(w io.Writer, p paths) error {
	if p.spec == "" {
		return errors.New("no specification: set TENON_SPEC or pass -spec")
	}
	fresh, current, n, err := freshAppendix(p.spec, p.codes)
	if err != nil {
		return err
	}
	if bytes.Equal(fresh, current) {
		fmt.Fprintf(w, "rulecheck: the appendix of diagnostic codes is already up to date (%d codes)\n", n)
		return nil
	}
	if err := os.WriteFile(p.spec, fresh, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "rulecheck: wrote the appendix of diagnostic codes to %s (%d codes)\n", display(p.spec), n)
	return nil
}
