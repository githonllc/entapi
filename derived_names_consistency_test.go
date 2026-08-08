package entapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
	"text/template"

	"entgo.io/ent/entc/gen"
)

// TestDerivedEntityNamesMatchTheTemplates is the guard that keeps #62's reserved
// name list from rotting.
//
// reservedNameConflicts refuses a schema whose entity name collides with a name
// this extension generates, and the whole check is only as good as the list in
// derivedEntityDecls. A list is exactly the kind of thing that goes stale: a
// template gains a declaration, nobody remembers this file, and the collision
// walks through with no refusal and no test failure — which is precisely how the
// list arrived one name short (New<N>ListResponse, added by #65) before this
// test existed.
//
// So the list is not read, it is DERIVED and compared. All five standalone
// output templates are rendered over a probe entity, go/parser reads the
// exported top-level declarations back out, and the two sets are compared in
// both directions:
//
//	forward  every exported declaration the templates emit must be in the
//	         reserved set, or reservedNameConflicts cannot refuse it;
//	reverse  every name in the reserved set must actually be emitted, or the
//	         list is refusing entity names for no reason.
//
// # What is deliberately NOT collected
//
//   - Methods (FuncDecl with a receiver). A method name lives in its receiver's
//     namespace, so Predicates, Validate, Apply and the Has<Field> family
//     collide with nothing an ent schema can declare.
//   - Unexported declarations. An ent schema type is a Go type in the consumer's
//     schema package, so its name is always exported; probeSortOptions can never
//     be collided with.
//   - References. The spec that shaped this test anticipated an allowance for
//     ent-side names such as IsNotFound, but no template DECLARES one — they are
//     called, not defined — and this collects declarations only. The allowance is
//     therefore empty rather than merely unused, and adding it would be a hole:
//     it would let a template start declaring `IsNotFound` unnoticed.
//
// # Why the probe is a real fixture graph
//
// Rendering needs a *gen.Type ent itself produced: the templates reach
// $.ID.Type, $.QueryName, $.Package and $.Config.Package, and every conditional
// emission is driven by real annotations. internal/fixtures/reservednames is
// that graph — Probe carries a create scope, an update scope, a response-only
// field, all three query markers and the soft-delete mixin, which between them
// fire every conditional in the five output templates. Without that the reverse
// direction would pass while quietly checking half the list.
func TestDerivedEntityNamesMatchTheTemplates(t *testing.T) {
	root := repoRoot(t)
	g := loadFixtureGraph(t, fixtureSchemaDir(root, "reservednames"), fixtureEntPkgPath("reservednames"))

	probe := nodeNamed(t, g, "Probe")
	if len(domainFields(probe)) == 0 {
		t.Fatal("the probe entity carries no domain fields, so the per-type templates would emit nothing at all")
	}
	if len(softDeleteTypes(g)) == 0 {
		t.Fatal("the probe graph has no soft-deletable entity, so the soft-delete helpers would never be rendered")
	}

	ext := NewExtensionWithOptions()

	emitted := map[string]string{} // name -> the template that declared it
	record := func(tmplName string, src []byte) {
		for _, name := range exportedDecls(t, tmplName, src) {
			if prev, dup := emitted[name]; dup {
				t.Errorf("%q is declared by both the %s and the %s template; "+
					"the generated files land in one package, so that does not compile", name, prev, tmplName)
			}
			emitted[name] = tmplName
		}
	}

	// The three per-type templates, rendered over the probe entity.
	record("dto", renderTemplate(t, ext, "dto", dtoTemplate, probe))
	record("filter", renderTemplate(t, ext, "filter", filterTemplate, probe))
	record("wiring", renderTemplate(t, ext, "wiring", wiringTemplate, probe))

	// The two graph-level templates, rendered over the whole graph — they take a
	// *gen.Graph as their data, so they cannot go through renderTemplate.
	record("errors", renderGraphTemplate(t, ext, "errors", errorMapTemplate, g))
	record("softdelete", renderGraphTemplate(t, ext, "softdelete", softDeleteTemplate, g))

	reserved := map[string]bool{
		errorMapSymbol: true,
	}
	for _, name := range derivedEntityNames(probe) {
		reserved[name] = true
	}

	// Forward: nothing is generated that reservedNameConflicts would not refuse.
	for _, name := range sortedKeys(emitted) {
		if reserved[name] {
			continue
		}
		t.Errorf("templates/%s.tmpl declares %q, which is in neither derivedEntityDecls nor the graph-level reserved set.\n"+
			"An entity named %q would therefore generate a package that does not compile, with no refusal (#62).\n"+
			"Add it to derivedEntityDecls in schema_conflicts.go, next to the other names from that template.",
			emitted[name], name, name)
	}

	// Reverse: nothing is reserved that no template emits. Every conditional
	// emission in the five output templates is unlocked by the probe entity's
	// annotations, so an absence here is a name that has gone away — the list
	// would be refusing entity names for a symbol that no longer exists.
	for _, name := range sortedKeys(reserved) {
		if _, ok := emitted[name]; ok {
			continue
		}
		t.Errorf("%q is reserved but no template declares it for the probe entity.\n"+
			"Either a template stopped emitting it — in which case remove it from derivedEntityDecls — or\n"+
			"internal/fixtures/reservednames' Probe lost the annotation that made that emission conditional.",
			name)
	}
}

// TestSoftDeleteTemplateDeclaresNoExportedSymbols pins why it contributes no
// graph-level reserved name: both helpers must stay unexported.
func TestSoftDeleteTemplateDeclaresNoExportedSymbols(t *testing.T) {
	root := repoRoot(t)
	g := loadFixtureGraph(t, fixtureSchemaDir(root, "reservednames"), fixtureEntPkgPath("reservednames"))

	ext := NewExtensionWithOptions()
	src := renderGraphTemplate(t, ext, "softdelete", softDeleteTemplate, g)

	if names := exportedDecls(t, "softdelete", src); len(names) != 0 {
		t.Errorf("templates/softdelete.tmpl declares exported names %v; each needs a graph-level reserved-name check", names)
	}
}

// exportedDecls returns the exported top-level declaration names in src, which
// is the set of identifiers the generated file claims in its package. Methods
// are excluded: their names live in the receiver's namespace.
func exportedDecls(t *testing.T, name string, src []byte) []string {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name+".go", src, 0)
	if err != nil {
		t.Fatalf("rendered %s template is not valid Go: %v\n%s", name, err, src)
	}

	var out []string
	keep := func(id *ast.Ident) {
		if id != nil && id.Name != "_" && ast.IsExported(id.Name) {
			out = append(out, id.Name)
		}
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				continue
			}
			keep(d.Name)
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					keep(s.Name)
				case *ast.ValueSpec:
					for _, id := range s.Names {
						keep(id)
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// renderGraphTemplate is renderTemplate for the two templates whose data is a
// *gen.Graph rather than a *gen.Type.
func renderGraphTemplate(t *testing.T, ext *Extension, name, text string, g *gen.Graph) []byte {
	t.Helper()
	tmpl, err := template.New(name).Funcs(ext.templateFuncMap()).Parse(text)
	if err != nil {
		t.Fatalf("parsing %s template: %v", name, err)
	}
	var buf []byte
	if err := tmpl.Execute(&byteWriter{buf: &buf}, g); err != nil {
		t.Fatalf("rendering %s template: %v", name, err)
	}
	return buf
}

// nodeNamed returns the graph node with this name, failing with the names it did
// find — a renamed fixture entity is otherwise a nil dereference three lines
// later.
func nodeNamed(t *testing.T, g *gen.Graph, name string) *gen.Type {
	t.Helper()
	var have []string
	for _, node := range g.Nodes {
		if node.Name == name {
			return node
		}
		have = append(have, node.Name)
	}
	t.Fatalf("no entity named %q in the fixture graph; it holds %s", name, strings.Join(have, ", "))
	return nil
}

// sortedKeys makes the failure output stable, so a diff between two runs is a
// change rather than map iteration order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
