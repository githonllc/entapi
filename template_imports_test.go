package entapi

import (
	"go/parser"
	"go/token"
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
// it — and formatFile now aborts when the formatter fails, so a template that
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
			schemaDir: fixtureSchemaDir(root, "basic"),
			pkgPath:   fixtureEntPkgPath("basic"),
		},
		{
			// Enums, whose Go type lives in the entity's own subpackage, and
			// named JSON types, whose Go type lives in the schema package —
			// neither of which any template used to name.
			name:      "fieldshapes",
			schemaDir: fixtureSchemaDir(root, "fieldshapes"),
			pkgPath:   fixtureEntPkgPath("fieldshapes"),
		},
		{
			// A response-only JSON field carrying []time.Time whose ent schema
			// marks it Sensitive.
			// When that field disappears from the response surface, the "time"
			// import it alone required must disappear with it.
			name:      "sensitive",
			schemaDir: fixtureSchemaDir(root, "sensitive"),
			pkgPath:   fixtureEntPkgPath("sensitive"),
		},
		{
			// Edges, and — through Secret — an entity with no response-scoped
			// field at all. The response and summary types are emitted for it
			// anyway, carrying only the ID, so the ID's import is needed on a
			// path where responseFields is empty.
			name:      "edges",
			schemaDir: fixtureSchemaDir(root, "edges"),
			pkgPath:   fixtureEntPkgPath("edges"),
		},
		{
			// An enum in a create AND a patch request, which is what makes the
			// generated validators call ent's <Field>Validator. Those live in
			// the entity's own subpackage, so this is the one path where the
			// DTO names a package no field TYPE required it to import.
			name:      "presence",
			schemaDir: fixtureSchemaDir(root, "presence"),
			pkgPath:   fixtureEntPkgPath("presence"),
		},
		{
			// The query surface: an enum, a time field and an optional int in
			// the filter struct, plus an entity that marks nothing and whose
			// filter file therefore names only the three unconditional
			// imports.
			name:      "query",
			schemaDir: fixtureSchemaDir(root, "query"),
			pkgPath:   fixtureEntPkgPath("query"),
		},
		{
			// An int primary key, which is the one identifier shape that needs
			// NO import at all. wiringImports asks fieldImportSpec for the id's
			// package and gets "" — and an import declared without a use fails
			// generation exactly as loudly as one used without being declared,
			// so this direction needs its own case now that a non-UUID key is
			// generated rather than refused (#29).
			name:      "intid",
			schemaDir: fixtureSchemaDir(root, "intid"),
			pkgPath:   fixtureEntPkgPath("intid"),
		},
		// The refused fixtures ("immutable", "selfref", "queryconflict") are
		// deliberately absent: the generator stops before rendering anything for
		// them. Asserting on the imports of output that is never emitted would
		// be testing a fiction.
		//
		// "selfrefpartial" is absent for a different reason: its templates see
		// the same shapes "edges" already covers, one self-referential edge less.
		//
		// "softdelete" is here for the graph-level template only, which is
		// checked separately below — its per-type output is the same shapes
		// "edges" covers.
		{
			name:      "softdelete",
			schemaDir: fixtureSchemaDir(root, "softdelete"),
			pkgPath:   fixtureEntPkgPath("softdelete"),
		},
	}

	tmpls := []struct {
		name string
		text string
	}{
		{"dto", dtoTemplate},
		{"filter", filterTemplate},
		{"wiring", wiringTemplate},
		{"handler", handlerTemplate},
	}

	ext := NewExtensionWithOptions()

	for _, sc := range schemas {
		t.Run(sc.name, func(t *testing.T) {
			for _, node := range loadFixtureNodes(t, sc.schemaDir, sc.pkgPath) {
				if !isResource(node) {
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

// TestSoftDeleteTemplateDeclaresItsImports is the same invariant for the one
// template rendered over a *gen.Graph rather than a *gen.Type, so it cannot
// share the loop above.
//
// It is the template with the most to get wrong here: its entity-subpackage
// imports are one per soft-deletable type, computed by softDeleteImports, while
// "context", "time" and entapi are named unconditionally — and are correct
// only because the file is not written at all when the type switch would be
// empty.
func TestSoftDeleteTemplateDeclaresItsImports(t *testing.T) {
	root := repoRoot(t)
	schemaDir := fixtureSchemaDir(root, "softdelete")
	pkgPath := fixtureEntPkgPath("softdelete")

	g := loadFixtureGraph(t, schemaDir, pkgPath)
	if len(softDeleteTypes(g)) == 0 {
		t.Fatal("the softdelete fixture has no soft-deletable entity; this test would pass vacuously")
	}

	ext := NewExtensionWithOptions()
	tmpl, err := template.New("softdelete").Funcs(ext.templateFuncMap()).Parse(softDeleteTemplate)
	if err != nil {
		t.Fatalf("parsing softdelete template: %v", err)
	}
	var buf []byte
	if err := tmpl.Execute(&byteWriter{buf: &buf}, g); err != nil {
		t.Fatalf("rendering softdelete template: %v", err)
	}

	formatted, err := imports.Process(softDeleteFileName, buf, nil)
	if err != nil {
		t.Fatalf("rendered softdelete template is not valid Go: %v\n%s", err, buf)
	}

	declared := importPaths(t, buf)
	resolved := importPaths(t, formatted)
	for _, added := range missing(resolved, declared) {
		t.Errorf("goimports had to ADD %q: the softdelete template does not declare an import its output uses", added)
	}
	for _, removed := range missing(declared, resolved) {
		t.Errorf("goimports had to REMOVE %q: the softdelete template declares an import its output does not use", removed)
	}
}

// TestErrorMapTemplateDeclaresItsImports is the same invariant for the other
// graph-level template. It has exactly one import and no conditional at all,
// which is precisely why it is worth pinning: the file is short enough that a
// future edit adding a helper would be tempted to lean on goimports, and
// formatFile aborts rather than repairing.
func TestErrorMapTemplateDeclaresItsImports(t *testing.T) {
	root := repoRoot(t)
	schemaDir := fixtureSchemaDir(root, "basic")
	pkgPath := fixtureEntPkgPath("basic")

	g := loadFixtureGraph(t, schemaDir, pkgPath)

	ext := NewExtensionWithOptions()
	tmpl, err := template.New("errors").Funcs(ext.templateFuncMap()).Parse(errorMapTemplate)
	if err != nil {
		t.Fatalf("parsing errors template: %v", err)
	}
	var buf []byte
	if err := tmpl.Execute(&byteWriter{buf: &buf}, g); err != nil {
		t.Fatalf("rendering errors template: %v", err)
	}

	formatted, err := imports.Process(errorMapFileName, buf, nil)
	if err != nil {
		t.Fatalf("rendered errors template is not valid Go: %v\n%s", err, buf)
	}

	declared := importPaths(t, buf)
	resolved := importPaths(t, formatted)
	for _, added := range missing(resolved, declared) {
		t.Errorf("goimports had to ADD %q: the errors template does not declare an import its output uses", added)
	}
	for _, removed := range missing(declared, resolved) {
		t.Errorf("goimports had to REMOVE %q: the errors template declares an import its output does not use", removed)
	}
}

func TestHTTPTemplateDeclaresItsImports(t *testing.T) {
	root := repoRoot(t)
	g := loadFixtureGraph(t, fixtureSchemaDir(root, "basic"), fixtureEntPkgPath("basic"))
	ext := NewExtensionWithOptions()
	rendered := renderGraphTemplate(t, ext, "http", httpTemplate, g)

	formatted, err := imports.Process(httpFileName, rendered, nil)
	if err != nil {
		t.Fatalf("rendered http template is not valid Go: %v\n%s", err, rendered)
	}
	declared := importPaths(t, rendered)
	resolved := importPaths(t, formatted)
	for _, added := range missing(resolved, declared) {
		t.Errorf("goimports had to ADD %q: the http template does not declare an import its output uses", added)
	}
	for _, removed := range missing(declared, resolved) {
		t.Errorf("goimports had to REMOVE %q: the http template declares an import its output does not use", removed)
	}
}

// loadFixtureGraph is loadFixtureNodes for a caller that needs the whole graph,
// which the graph-level template takes as its data.
func loadFixtureGraph(t *testing.T, schemaDir, pkgPath string) *gen.Graph {
	t.Helper()

	var graph *gen.Graph
	capture := &captureExtension{fn: func(g *gen.Graph) { graph = g }}

	err := entc.Generate(schemaDir, &gen.Config{
		Target:  t.TempDir(),
		Package: pkgPath,
	}, entc.Extensions(capture))
	if err != nil {
		t.Fatalf("loading fixture schema %s: %v", schemaDir, err)
	}
	if graph == nil {
		t.Fatalf("fixture schema %s produced no graph", schemaDir)
	}
	return graph
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
