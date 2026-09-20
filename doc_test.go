package tenon_test

import (
	"go/ast"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestPackageDocNamesRealSymbols holds the package doc to the API it names.
// Every bracketed name in it is a documentation link, which pkg.go.dev renders
// as a link where the symbol exists and as literal brackets where it does not,
// so a symbol that is renamed or withdrawn must take its mention with it. The
// doc is a map of an API too large to read through, and a map with a road that
// is not there is worse than no map.
func TestPackageDocNamesRealSymbols(t *testing.T) {
	text := packageDoc(t)
	symbols, methods := exportedAPI(t)

	// Every bracketed name resolves. The parser is told that every symbol
	// exists, so that it makes a link of each and they are checked here
	// rather than silently left as text.
	var doc comment.Parser
	doc.LookupSym = func(recv, name string) bool { return true }
	doc.LookupPackage = func(name string) (string, bool) { return name, true }
	links := map[string]bool{}
	walkDoc(doc.Parse(text), func(link *comment.DocLink) {
		links[linkText(link)] = true
		switch {
		case link.ImportPath != "" && link.Name == "":
			if _, err := os.Stat(packageDir(t, link.ImportPath)); err != nil {
				t.Errorf("the package doc links to package %s, which is not in this module", link.ImportPath)
			}
		case link.ImportPath != "":
			// A symbol of another package: only its package is checked here.
			if _, err := os.Stat(packageDir(t, link.ImportPath)); err != nil {
				t.Errorf("the package doc links to %s, whose package is not in this module", linkText(link))
			}
		case link.Recv != "":
			if !methods[link.Recv][link.Name] {
				t.Errorf("the package doc names %s.%s, which this package does not have", link.Recv, link.Name)
			}
		default:
			if !symbols[link.Name] {
				t.Errorf("the package doc names %s, which this package does not export", link.Name)
			}
		}
	})
	if len(links) < 60 {
		t.Errorf("the package doc links to %d symbols, too few for a map of this API", len(links))
	}

	// Nothing else is in brackets. A bracketed name the parser did not take
	// for a link is text that reads as a broken link, unless it follows a
	// word, where it is Go type syntax rather than a reference: the brackets
	// of map[string]any are part of what they spell.
	brackets := regexp.MustCompile(`\[[^\]\n]+\]`)
	for _, at := range brackets.FindAllStringIndex(text, -1) {
		m := text[at[0]:at[1]]
		if at[0] > 0 && text[at[0]-1] != ' ' && text[at[0]-1] != '\n' {
			continue
		}
		if inner := strings.Trim(m, "[]"); !links[inner] {
			t.Errorf("the package doc holds %s, which is not a documentation link", m)
		}
	}
}

// linkText returns a link as it is written in the doc.
func linkText(link *comment.DocLink) string {
	var b strings.Builder
	if link.ImportPath != "" {
		b.WriteString(link.ImportPath)
		if link.Name != "" {
			b.WriteString(".")
		}
	}
	if link.Recv != "" {
		b.WriteString(link.Recv)
		b.WriteString(".")
	}
	b.WriteString(link.Name)
	return b.String()
}

// walkDoc calls visit for every documentation link in the parsed comment.
func walkDoc(doc *comment.Doc, visit func(*comment.DocLink)) {
	var text func(ts []comment.Text)
	text = func(ts []comment.Text) {
		for _, t := range ts {
			switch t := t.(type) {
			case *comment.DocLink:
				visit(t)
				text(t.Text)
			case *comment.Link:
				text(t.Text)
			case *comment.Italic, comment.Plain:
			}
		}
	}
	var block func(bs []comment.Block)
	block = func(bs []comment.Block) {
		for _, b := range bs {
			switch b := b.(type) {
			case *comment.Paragraph:
				text(b.Text)
			case *comment.Heading:
				text(b.Text)
			case *comment.List:
				for _, item := range b.Items {
					block(item.Content)
				}
			case *comment.Code:
			}
		}
	}
	block(doc.Content)
}

// packageDoc returns the text of the package comment.
func packageDoc(t *testing.T) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "doc.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if file.Doc == nil {
		t.Fatal("doc.go carries no package comment")
	}
	return file.Doc.Text()
}

// packageDir returns the directory of an import path within this module.
func packageDir(t *testing.T, path string) string {
	t.Helper()
	module := modulePath(t)
	switch {
	case path == module:
		return "."
	case strings.HasPrefix(path, module+"/"):
		return strings.TrimPrefix(path, module+"/")
	}
	return "\x00 not in this module"
}

// modulePath returns the module's import path, as go.mod declares it.
func modulePath(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatal("go.mod declares no module path")
	return ""
}

// exportedAPI returns the exported top-level names of this package, and its
// exported methods by the type they are on, read from the source.
func exportedAPI(t *testing.T) (symbols map[string]bool, methods map[string]map[string]bool) {
	t.Helper()
	symbols, methods = map[string]bool{}, map[string]map[string]bool{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if !decl.Name.IsExported() {
					continue
				}
				if decl.Recv == nil {
					symbols[decl.Name.Name] = true
					continue
				}
				recv := receiverName(decl.Recv.List[0].Type)
				if methods[recv] == nil {
					methods[recv] = map[string]bool{}
				}
				methods[recv][decl.Name.Name] = true
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						if spec.Name.IsExported() {
							symbols[spec.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, n := range spec.Names {
							if n.IsExported() {
								symbols[n.Name] = true
							}
						}
					}
				}
			}
		}
	}
	return symbols, methods
}

// receiverName returns the name of the type a method is on.
func receiverName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.StarExpr:
		return receiverName(expr.X)
	case *ast.IndexExpr: // a generic type, as Type[E]
		return receiverName(expr.X)
	case *ast.Ident:
		return expr.Name
	}
	return ""
}
