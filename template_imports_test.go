package entdomain

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"text/template"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"golang.org/x/tools/imports"
)

// TestTemplatesDeclareTheirImports pins the invariant behind #11's second
// criterion: goimports is a safety net, not the mechanism.
//
// For every template and every fixture entity it renders the template, runs the
// result through imports.Process, and requires the set of imports to come out
// unchanged. An import goimports has to ADD is one the template failed to
// declare; one it has to REMOVE is one the template declared under the wrong
// condition. Either way the file only compiles because the formatter rewrote
// it — and writeFile now aborts when the formatter fails, so a template that
// depends on the repair has no fallback.
func TestTemplatesDeclareTheirImports(t *testing.T) {
	root := repoRoot(t)

	schemas := []struct {
		name      string
		schemaDir string
		pkgPath   string
	}{
		{
			name:      "basic",
			schemaDir: filepath.Join(root, "internal", "fixtures", "basic", "ent", "schema"),
			pkgPath:   modulePath + "/internal/fixtures/basic/ent",
		},
		{
			// Enums, whose Go type lives in the entity's own subpackage, and
			// named JSON types, whose Go type lives in the schema package —
			// neither of which any template used to name.
			name:      "fieldshapes",
			schemaDir: filepath.Join(root, "internal", "fixtures", "fieldshapes", "ent", "schema"),
			pkgPath:   modulePath + "/internal/fixtures/fieldshapes/ent",
		},
		{
			// Edges, and — through Secret — an entity with no response-scoped
			// field at all. The response and summary types are emitted for it
			// anyway, carrying only the ID, so the ID's import is needed on a
			// path where responseFields is empty.
			name:      "edges",
			schemaDir: filepath.Join(root, "internal", "fixtures", "edges", "ent", "schema"),
			pkgPath:   modulePath + "/internal/fixtures/edges/ent",
		},
		{
			// An enum in a create AND a patch request, which is what makes the
			// generated validators call ent's <Field>Validator. Those live in
			// the entity's own subpackage, so this is the one path where the
			// DTO names a package no field TYPE required it to import.
			name:      "presence",
			schemaDir: filepath.Join(root, "internal", "fixtures", "presence", "ent", "schema"),
			pkgPath:   modulePath + "/internal/fixtures/presence/ent",
		},
		{
			// The query surface: an enum, a time field and an optional int in
			// the filter struct, plus an entity that marks nothing and whose
			// filter file therefore names only the three unconditional
			// imports.
			name:      "query",
			schemaDir: filepath.Join(root, "internal", "fixtures", "query", "ent", "schema"),
			pkgPath:   modulePath + "/internal/fixtures/query/ent",
		},
		// The refused fixtures ("immutable", "intid", "selfref",
		// "queryconflict") are
		// deliberately absent: the generator stops before rendering anything for
		// them. Asserting on the imports of output that is never emitted would
		// be testing a fiction.
		//
		// "selfrefpartial" is absent for a different reason: its templates see
		// the same shapes "edges" already covers, one self-referential edge less.
	}

	tmpls := []struct {
		name string
		text string
	}{
		{"dto", dtoTemplate},
		{"filter", filterTemplate},
		{"wiring", wiringTemplate},
		{"base_service", baseServiceTemplate},
		{"base_handler", baseHandlerTemplate},
	}

	ext := NewExtensionWithOptions(WithBaseService(true), WithBaseHandler(true))

	for _, sc := range schemas {
		t.Run(sc.name, func(t *testing.T) {
			for _, node := range loadFixtureNodes(t, sc.schemaDir, sc.pkgPath) {
				if len(domainFields(node)) == 0 {
					continue
				}
				for _, tc := range tmpls {
					t.Run(node.Name+"/"+tc.name, func(t *testing.T) {
						rendered := renderTemplate(t, ext, tc.name, tc.text, node)

						formatted, err := imports.Process(node.Name+"_"+tc.name+".go", rendered, nil)
						if err != nil {
							t.Fatalf("rendered %s template is not valid Go: %v\n%s", tc.name, err, rendered)
						}

						declared := importPaths(t, rendered)
						resolved := importPaths(t, formatted)

						for _, added := range missing(resolved, declared) {
							t.Errorf("goimports had to ADD %q: the %s template does not declare an import its output uses",
								added, tc.name)
						}
						for _, removed := range missing(declared, resolved) {
							t.Errorf("goimports had to REMOVE %q: the %s template declares an import its output does not use",
								removed, tc.name)
						}
					})
				}
			}
		})
	}
}

// loadFixtureNodes runs ent's loader over a fixture schema and returns the
// graph's nodes. Generation goes to a temporary directory: only the parsed
// schema is wanted here, not the output.
func loadFixtureNodes(t *testing.T, schemaDir, pkgPath string) []*gen.Type {
	t.Helper()

	var nodes []*gen.Type
	capture := &captureExtension{fn: func(g *gen.Graph) { nodes = g.Nodes }}

	err := entc.Generate(schemaDir, &gen.Config{
		Target:  t.TempDir(),
		Package: pkgPath,
	}, entc.Extensions(capture))
	if err != nil {
		t.Fatalf("loading fixture schema %s: %v", schemaDir, err)
	}
	if len(nodes) == 0 {
		t.Fatalf("fixture schema %s produced no nodes", schemaDir)
	}
	return nodes
}

// captureExtension hands the loaded graph back to the test.
type captureExtension struct {
	entc.DefaultExtension
	fn func(*gen.Graph)
}

func (c *captureExtension) Hooks() []gen.Hook {
	return []gen.Hook{func(next gen.Generator) gen.Generator {
		return gen.GenerateFunc(func(g *gen.Graph) error {
			c.fn(g)
			return next.Generate(g)
		})
	}}
}

// renderTemplate renders one template the way generation does, but stops before
// the formatter — which is the only place the declared imports are visible.
func renderTemplate(t *testing.T, ext *Extension, name, text string, node *gen.Type) []byte {
	t.Helper()
	tmpl, err := template.New(name).Funcs(ext.templateFuncMap()).Parse(text)
	if err != nil {
		t.Fatalf("parsing %s template: %v", name, err)
	}
	var buf []byte
	w := &byteWriter{buf: &buf}
	if err := tmpl.Execute(w, node); err != nil {
		t.Fatalf("rendering %s template for %s: %v", name, node.Name, err)
	}
	return buf
}

type byteWriter struct{ buf *[]byte }

func (w *byteWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// importPaths returns the sorted import paths declared by src.
func importPaths(t *testing.T, src []byte) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", src, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parsing generated source: %v\n%s", err, src)
	}
	paths := make([]string, 0, len(f.Imports))
	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquoting import %s: %v", spec.Path.Value, err)
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// missing returns the elements of want that are absent from have.
func missing(want, have []string) []string {
	index := make(map[string]bool, len(have))
	for _, s := range have {
		index[s] = true
	}
	var out []string
	for _, s := range want {
		if !index[s] {
			out = append(out, s)
		}
	}
	return out
}
