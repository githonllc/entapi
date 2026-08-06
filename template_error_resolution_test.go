package entdomain

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// entdomainSymbols are this package's sentinels, which have no counterpart in
// `package ent` and must therefore always carry the entdomain qualifier.
var entdomainSymbols = []string{"ErrNotFound", "ErrAlreadyExists", "ErrValidation"}

// templateComment matches a {{/* ... */}} block. Comments are stripped before
// the scan below: the assertion is about the Go source the template emits, and
// the comments explaining the resolution necessarily name the symbols they warn
// against.
var templateComment = regexp.MustCompile(`(?s)\{\{/\*.*?\*/\}\}`)

// goLineComment matches a Go line comment in a template's emitted source. No
// template here puts "//" inside a string literal, so a line-wise strip is
// exact rather than approximate.
var goLineComment = regexp.MustCompile(`(?m)//.*$`)

// TestTemplatesQualifyEntdomainSentinels is the half of the resolution rule
// that applies to every template rather than to one call.
//
// The emitted files land in the consumer's `package ent`, which has no
// ErrNotFound, ErrAlreadyExists or ErrValidation of its own. A bare reference
// therefore does not compile — but it is the kind of mistake a template makes
// silently while it is being edited, and only the fixture build would catch it.
// This catches it in the source.
//
// It used to be asserted against base_service.tmpl alone, together with the
// converse rule for IsConstraintError. That template is gone (#29), but the
// converse rule came back with #13: errors.tmpl hands ent's IsNotFound and
// IsConstraintError to the runtime's mapper, and both must stay unqualified.
// The two unqualified-Ent-predicate rules — dto.tmpl's IsNotFound and
// errors.tmpl's pair — are pinned below.
func TestTemplatesQualifyEntdomainSentinels(t *testing.T) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("reading embedded templates failed: %v", err)
	}

	scanned := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}
		src, err := templateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			t.Fatalf("reading embedded %s failed: %v", entry.Name(), err)
		}
		scanned++
		text := templateComment.ReplaceAllString(string(src), "")

		for _, name := range entdomainSymbols {
			bare := regexp.MustCompile(`([^.\w])` + name + `\b`)
			for _, m := range bare.FindAllStringSubmatch(text, -1) {
				t.Errorf("templates/%s references %s without the entdomain qualifier (matched %q); package ent has no such symbol", entry.Name(), name, m[0])
			}
		}
	}

	if scanned == 0 {
		t.Fatal("no embedded templates were scanned; this test would pass vacuously")
	}
}

// TestDTOTemplateResolvesIsNotFoundToEnt extends the same rule to dto.tmpl,
// which acquired an IsNotFound call with the response constructors: a to-one
// edge that was loaded but matched no row comes back from <Edge>OrErr() as
// Ent's *NotFoundError, and telling that apart from a not-loaded edge is the
// whole contract. entdomain.IsNotFound tests this package's sentinels instead,
// so qualifying the call would compile and silently route every loaded-but-
// absent edge into the error branch.
func TestDTOTemplateResolvesIsNotFoundToEnt(t *testing.T) {
	src, err := templateFS.ReadFile("templates/dto.tmpl")
	if err != nil {
		t.Fatalf("reading embedded dto.tmpl failed: %v", err)
	}
	text := templateComment.ReplaceAllString(string(src), "")

	if !strings.Contains(text, "IsNotFound(err)") {
		t.Error("dto.tmpl no longer calls unqualified IsNotFound(err); if the edge contract changed, update this test")
	}
	qualified := regexp.MustCompile(`[\w.]+\.IsNotFound\b`)
	if m := qualified.FindAllString(text, -1); len(m) > 0 {
		t.Errorf("dto.tmpl qualifies IsNotFound as %v; it must stay unqualified so it binds to Ent's generated predicate in package ent", m)
	}
}

// TestErrorMapTemplateResolvesEntPredicates is the converse rule, and the one
// with the nastiest failure mode in this repository.
//
// errors.tmpl emits
//
//	var ErrorMap = entdomain.NewErrorMapper(IsNotFound, IsConstraintError)
//
// into the consumer's `package ent`, so both names bind to Ent's own generated
// predicates. Qualifying either as entdomain.* still COMPILES — this package
// exports an IsNotFound of its own, testing its sentinels — and the result is a
// mapper whose not-found predicate can only ever be true for an error the
// mapper itself produced. Every missing row would come back unclassified, and
// no test that only reads the template would notice.
//
// entdomain.NewErrorMapper is the one qualified reference the file is allowed,
// because the runtime type genuinely lives in this package.
func TestErrorMapTemplateResolvesEntPredicates(t *testing.T) {
	src, err := templateFS.ReadFile("templates/errors.tmpl")
	if err != nil {
		t.Fatalf("reading embedded errors.tmpl failed: %v", err)
	}
	// Go line comments go too, not just {{/* */}} blocks: this template's whole
	// doc comment is about which package the two predicates resolve in, so it
	// names both of them qualified, on purpose. The assertion is about the one
	// line of emitted code.
	text := goLineComment.ReplaceAllString(templateComment.ReplaceAllString(string(src), ""), "")

	if !strings.Contains(text, "entdomain.NewErrorMapper(IsNotFound, IsConstraintError)") {
		t.Error("errors.tmpl no longer constructs the mapper from Ent's unqualified IsNotFound and IsConstraintError; if the wiring changed, update this test")
	}
	for _, name := range []string{"IsNotFound", "IsConstraintError"} {
		qualified := regexp.MustCompile(`[\w.]+\.` + name + `\b`)
		if m := qualified.FindAllString(text, -1); len(m) > 0 {
			t.Errorf("errors.tmpl qualifies %s as %v; it must stay unqualified so it binds to Ent's generated predicate in package ent", name, m)
		}
	}
}

// TestWiringMapsEveryExportedOperation is the drift detector for acceptance
// criterion 3: the sentinel helpers must behave identically whichever generated
// operation produced the error.
//
// It is asserted structurally rather than by grepping for a count, because the
// operations a template emits depend on the entity's scopes — an entity with no
// create-scoped field gets no Create. Parsing the rendered output and requiring
// EVERY exported function to return through ErrorMap holds regardless of which
// ones were emitted, and it fails on the realistic regression: someone adds a
// seventh operation and forgets the mapping.
//
// The behavioural half is internal/fixtures/wiring/e2e; this one runs over
// every fixture shape rather than the three that fixture has.
func TestWiringMapsEveryExportedOperation(t *testing.T) {
	root := repoRoot(t)
	ext := NewExtensionWithOptions()

	for _, fixture := range []string{"wiring", "edges", "intid", "query"} {
		t.Run(fixture, func(t *testing.T) {
			schemaDir := filepath.Join(root, "internal", "fixtures", fixture, "ent", "schema")
			pkgPath := modulePath + "/internal/fixtures/" + fixture + "/ent"

			checked := 0
			for _, node := range loadFixtureNodes(t, schemaDir, pkgPath) {
				if len(domainFields(node)) == 0 {
					continue
				}
				rendered := renderTemplate(t, ext, "wiring", wiringTemplate, node)

				fset := token.NewFileSet()
				file, err := parser.ParseFile(fset, node.Name+"_wiring.go", rendered, 0)
				if err != nil {
					t.Fatalf("rendered wiring for %s is not valid Go: %v\n%s", node.Name, err, rendered)
				}
				for _, decl := range file.Decls {
					fn, ok := decl.(*ast.FuncDecl)
					if !ok || fn.Recv != nil || !fn.Name.IsExported() {
						continue
					}
					checked++
					var body strings.Builder
					if err := printer.Fprint(&body, fset, fn.Body); err != nil {
						t.Fatalf("printing %s: %v", fn.Name.Name, err)
					}
					if !strings.Contains(body.String(), "ErrorMap.MapError") {
						t.Errorf("%s.%s does not return through ErrorMap.MapError; a caller cannot then use entdomain.IsNotFound uniformly across operations\n%s",
							node.Name, fn.Name.Name, body.String())
					}
				}
			}
			if checked == 0 {
				t.Fatalf("fixture %q rendered no exported wiring function; this subtest would pass vacuously", fixture)
			}
		})
	}
}
