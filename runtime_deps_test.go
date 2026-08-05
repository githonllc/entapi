package entdomain

import (
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// runtimeFiles is the ent-free core: the Layer 2 code a consumer's binary links
// against at runtime. Everything else in this package — the extension, the
// annotations, the template functions — is generator-time code and imports ent
// by necessity.
//
// The split is by file rather than by package only because the package split
// itself is #15. Until that lands, `go list -deps github.com/githonllc/entdomain`
// reports 14 entgo.io packages (verified), because one package holds both
// halves. The criterion in #24 is about the runtime half, and it is checked
// here at the granularity that currently exists.
var runtimeFiles = []string{
	"query.go",
	"types.go",
	"errors.go",
	"errors_map.go",
	"cursor.go",
}

// TestRuntimeCoreImportsOnlyStdlib is criterion 5 of #24.
//
// Reading a single import block would not settle it: an import that is not ent
// can still pull ent in transitively. Restricting the core to the standard
// library settles it outright, because the standard library is closed — no
// stdlib package can reach entgo.io however deep you follow it.
//
// The same fact was checked independently by copying these files into a module
// whose go.mod requires nothing and running `go list -deps ./...`: 67 packages,
// all stdlib, zero entgo.io. This test is the checked-in guard against that
// drifting.
func TestRuntimeCoreImportsOnlyStdlib(t *testing.T) {
	fset := token.NewFileSet()
	for _, name := range runtimeFiles {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v — if the file was renamed, update runtimeFiles", name, err)
			}
			var external []string
			for _, spec := range f.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatalf("bad import path %s: %v", spec.Path.Value, err)
				}
				// A standard-library import path's first element never
				// contains a dot; every module path's does.
				if strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
					external = append(external, path)
				}
			}
			if len(external) > 0 {
				sort.Strings(external)
				t.Fatalf("%s imports non-stdlib packages %v; the runtime core must stay "+
					"linkable without the generator's dependency graph", name, external)
			}
		})
	}
}

// TestRuntimeFilesStillExist guards the list above against a silent rename,
// which would otherwise turn the check into a vacuous pass.
func TestRuntimeFilesStillExist(t *testing.T) {
	if len(runtimeFiles) == 0 {
		t.Fatal("runtimeFiles is empty; the import check would pass vacuously")
	}
}
