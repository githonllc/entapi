package entdomain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

// removedCursorSymbols is the exclusion list decided on #6 and recorded in
// DESIGN-v2.md §9.1: the base64(json) keyset codec and the response metadata
// that only that codec could fill.
//
// The names are spelled as strings rather than referenced, because a test that
// referenced them would stop compiling the moment they were removed — which is
// the event this test exists to observe.
var removedCursorSymbols = []string{
	"Cursor",
	"PageInfo",
	"EncodeCursor",
	"DecodeCursor",
	"normalizeJSONNumber",
}

// TestCursorSymbolsStayOutOfThePackage is the structural half of #6.
//
// The codec had zero callers, and its one remaining generated reference was the
// PageInfo field dto.tmpl emitted into every {Entity}ListResponse. Deleting an
// exported symbol is a breaking change that can only be made once, so this pins
// it: a reintroduction has to argue with a test rather than slip in as a
// convenience.
//
// It scans declarations rather than text so a mention in a comment — the README
// pointer, this file's own list — does not trip it.
func TestCursorSymbolsStayOutOfThePackage(t *testing.T) {
	forbidden := make(map[string]bool, len(removedCursorSymbols))
	for _, name := range removedCursorSymbols {
		forbidden[name] = true
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing the package: %v", err)
	}
	pkg, ok := pkgs["entdomain"]
	if !ok {
		t.Fatal("package entdomain not found in the working directory; this test would pass vacuously")
	}

	for path, file := range pkg.Files {
		for _, decl := range file.Decls {
			for _, name := range declaredNames(decl) {
				if forbidden[name] {
					t.Errorf("%s declares %q; the cursor codec left the public API on #6 "+
						"and a keyset design that is ever needed is to be written fresh, not revived",
						path, name)
				}
			}
		}
	}
}

// declaredNames returns the top-level identifiers a declaration introduces.
func declaredNames(decl ast.Decl) []string {
	var names []string
	switch d := decl.(type) {
	case *ast.FuncDecl:
		names = append(names, d.Name.Name)
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				names = append(names, s.Name.Name)
			case *ast.ValueSpec:
				for _, id := range s.Names {
					names = append(names, id.Name)
				}
			}
		}
	}
	return names
}

// TestListRequestCarriesNoCursorField pins the field half of the same decision.
//
// ListRequest.Cursor was documented as "when Cursor is set, keyset pagination is
// used" and no code ever branched on it. A consumer who believed the comment got
// offset page 1 forever — a wrong result, silently, from a published contract.
// Removing the field is breaking; leaving it and adding one later is additive,
// and that asymmetry is what decided it.
func TestListRequestCarriesNoCursorField(t *testing.T) {
	rt := reflect.TypeOf(ListRequest{})
	if _, found := rt.FieldByName("Cursor"); found {
		t.Fatal("ListRequest.Cursor is back. Nothing branches on it, so a caller who sets it " +
			"silently gets offset page 1; add the field only together with the code that reads it")
	}
	for i := 0; i < rt.NumField(); i++ {
		if tag := rt.Field(i).Tag.Get("json"); strings.HasPrefix(tag, "cursor") {
			t.Errorf("ListRequest.%s still binds the %q query parameter", rt.Field(i).Name, tag)
		}
	}
}

// TestNoTemplateEmitsCursorMetadata is the generation half.
//
// dto.tmpl emitting `PageInfo *entdomain.PageInfo` into {Entity}ListResponse was
// the single blocker that kept the codec alive: the type had no consumer — the
// generated wiring returns entdomain.Page[…] — but deleting it would have broken
// a live template. This asserts the template side directly, so the two can never
// drift back apart.
func TestNoTemplateEmitsCursorMetadata(t *testing.T) {
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		t.Fatalf("reading the embedded templates: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded templates; this test would pass vacuously")
	}
	for _, entry := range entries {
		body, err := fs.ReadFile(templateFS, "templates/"+entry.Name())
		if err != nil {
			t.Fatalf("reading templates/%s: %v", entry.Name(), err)
		}
		for _, symbol := range removedCursorSymbols {
			if strings.Contains(string(body), "entdomain."+symbol) {
				t.Errorf("templates/%s emits entdomain.%s, which no longer exists", entry.Name(), symbol)
			}
		}
	}
}
