package entdomain

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestNoAmbiguousEntPackages is the root-cause guard for #49.
//
// goimports resolves a bare `ent.` reference by PACKAGE NAME, not by import
// path. `entgo.io/ent` is itself `package ent` and is a dependency of every
// module in this repository, so it is ALWAYS one candidate. Any package named
// `ent` inside the repository is therefore a second one, and which of the two
// goimports picks for a file that does not already spell the import out depends
// on the state of its module index cache — the choice is non-deterministic
// across runs. That is how internal/fixtures/query/queryent/client.go had
// "entgo.io/ent" rewritten to internal/fixture/ent, and, one incident earlier,
// how internal/fixtures/softdelete/ent/client.go had it rewritten to
// internal/fixtures/basic/ent.
//
// The files that get rewritten are written by ENT's generator, not by this
// package's formatFile, so nothing this extension passes to imports.Process can
// reach them. The only fix available from here is to stop the collision
// existing: no package in this repository may be named `ent`.
//
// # Why the census walks the filesystem
//
// The previous version of this guard ran `go list ./...` and allowed "at most
// one package named ent". Both halves were wrong, and together they reported
// green while the collision that caused the third incident sat untouched:
//
//   - `go list ./...` enumerates ONE module. internal/fixture,
//     internal/softdeleteproof and internal/fixtures/wiring/e2e each have their
//     own go.mod, so the root module's `go list` cannot see into them — and
//     internal/fixture/ent, the package that actually won the rewrite, lived in
//     exactly that blind spot.
//   - goimports does not consult the module graph at all. It walks the
//     filesystem (golang.org/x/tools/internal/gopathwalk), so a nested module is
//     no more hidden from it than any other directory. The census has to see
//     what goimports sees, which means walking the same way rather than asking
//     the build system.
//   - "at most one" was never the right threshold once entgo.io/ent is counted.
//     One in-repo `ent` plus the dependency is already two.
//
// Running `go list` once per module would fix the first point but not the
// second or third, and it inherits `go list`'s failure mode: the third incident
// broke an import, `go list ./...` then failed outright, and the guard took its
// "census could not be taken" branch instead of reporting the collision. Parsing
// package clauses only needs the files to be syntactically valid up to the
// package statement, so a tree this broken is still censused.
//
// # What the walk skips, and why exactly these
//
// gopathwalk refuses to descend into a directory whose name begins with `.` or
// `_`, and into `testdata` (walk.go, the base-name check). A package in one of
// those is invisible to goimports, so counting it here would fail the build over
// an ambiguity that cannot occur. Nothing else is skipped — notably `vendor` and
// `node_modules` are NOT, because in module mode gopathwalk skips neither
// outside GOROOT. This repository currently contains no directory of any of
// these kinds, so the skip set is a statement about future trees, not a hole in
// the present one.
func TestNoAmbiguousEntPackages(t *testing.T) {
	root := repoRoot(t)

	found := censusEntPackages(t, root)
	if len(found) == 0 {
		return
	}

	t.Errorf("found %d directory(ies) under this repository holding Go files that declare `package ent`.\n"+
		"Rule: NONE may. `entgo.io/ent` is already `package ent`, so the first in-repo one\n"+
		"makes the name ambiguous, and goimports resolves a bare `ent.` reference against all\n"+
		"of them by name, picking whichever its cache offers (#49).\n\n"+
		"Directories declaring `package ent`:\n%s\n"+
		"Give the package a distinct name. A Go package is named after the last element of its\n"+
		"import path, so the mechanism is the directory name: internal/fixtures/<dir>/<dir>ent\n"+
		"gives `package <dir>ent`, and internal/fixture/spikeent gives `package spikeent`.\n"+
		"Nested modules count: this census walks the filesystem, exactly as goimports does, so\n"+
		"a directory the root module's `go list` cannot see is still caught here.",
		len(found), formatPkgList(found))
}

// censusEntPackages returns every directory under root, relative to it, holding
// at least one .go file whose package clause is `ent`.
//
// The package clause is read with go/parser in PackageClauseOnly mode: it is the
// only thing that decides what goimports can resolve a bare `ent.` against, and
// reading just it means a tree that does not compile — which is what every #49
// incident produced — is still censused.
//
// Build constraints are deliberately ignored. `//go:build ignore` files such as
// a fixture's entc.go declare `package main` and so never match anyway, and a
// file excluded by a constraint still sits in a directory whose other files name
// the package. Counting a directory whenever ANY of its files says `ent` errs
// toward reporting a collision that goimports might not actually hit, which is
// the safe direction for a guard whose failure in the other direction is this
// issue.
func censusEntPackages(t *testing.T, root string) []string {
	t.Helper()

	seen := make(map[string]bool)
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		base := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			// The gopathwalk skip set. See the doc comment on the test.
			if base == "" || base[0] == '.' || base[0] == '_' || base == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(base, ".go") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
		if parseErr != nil {
			// Not fatal: a file too broken to yield a package clause is a file
			// goimports cannot offer as a candidate either.
			t.Logf("ent package census: skipping %s, its package clause does not parse: %v", path, parseErr)
			return nil
		}
		if f.Name == nil || f.Name.Name != "ent" {
			return nil
		}
		rel, relErr := filepath.Rel(root, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		seen[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s to census packages named `ent` failed: %v", root, err)
	}

	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}

func formatPkgList(paths []string) string {
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString("    ")
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	return sb.String()
}
