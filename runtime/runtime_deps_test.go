package entapi

import (
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestRuntimeCoreImportsOnlyStdlib is criterion 5 of #24, restated at the
// granularity #15 created.
//
// Until #15 this was a list of FILE names inside a package that also held the
// generator, because a file list was the only boundary that existed —
// `go list -deps github.com/githonllc/entapi` reported 15 entgo.io packages
// however ent-free those five files were. The boundary is now a package, so the
// check reads the directory instead of a hand-maintained list, and a file
// dropped into the runtime is covered the moment it lands rather than the
// moment someone remembers to name it.
//
// Restricting the core to the standard library settles the question outright,
// because the standard library is closed: no stdlib package can reach entgo.io
// however deep you follow it. The transitive statement — that nothing in the
// closure embeds or loads a template — is TestRuntimePackageIsGeneratorFree in
// the root package, which asks the go tool rather than the parser.
func TestRuntimeCoreImportsOnlyStdlib(t *testing.T) {
	for _, name := range runtimeSourceFiles(t) {
		t.Run(name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
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

// runtimeSourceFiles returns this package's non-test Go files, and fails if
// there are none — an empty list would make the check above pass vacuously,
// which is exactly what a hand-maintained file list could silently become.
func runtimeSourceFiles(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the runtime package directory: %v", err)
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		t.Fatal("the runtime package has no source files; the import check would pass vacuously")
	}
	sort.Strings(names)
	return names
}
