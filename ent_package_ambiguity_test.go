package entdomain

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// fixturesPrefix is the import-path segment every codegen fixture lives under.
const fixturesPrefix = "/internal/fixtures/"

// TestNoAmbiguousEntPackages is the root-cause guard for #49.
//
// goimports resolves a bare `ent.` reference by PACKAGE NAME, not by import
// path. When several packages in the same module are all named `ent`, which one
// it picks for a file that does not already spell the import out depends on the
// state of its module index cache — so the choice is non-deterministic across
// runs. That is how internal/fixtures/softdelete/ent/client.go once had
// "entgo.io/ent" rewritten to internal/fixtures/basic/ent: an unrelated
// fixture's package, picked purely because it answered to the same name.
//
// The file that got rewritten is written by ENT's generator, not by this
// package's writeFile, so nothing this extension passes to imports.Process can
// reach it. The only fix available from here is to stop the collision existing:
// every fixture's generated package gets a distinct name.
//
// The rule this asserts, and why it is two clauses rather than one:
//
//   - At most one package in this module may be named `ent`. Two is the
//     smallest number that can be ambiguous.
//   - No package under internal/fixtures may be named `ent` at all. Without
//     this clause the guard would have a hole exactly where the regression
//     lives: the module currently has zero, so the first fixture added the old
//     way would bring the count to one and slip through.
//
// Derived from `go list` rather than a hardcoded list so a fixture added the
// wrong way fails here, on the next run, instead of years later as a mystery
// import rewrite.
func TestNoAmbiguousEntPackages(t *testing.T) {
	root := repoRoot(t)
	goTool := goToolPath(t)

	cmd := exec.Command(goTool, "list", "-f", "{{.Name}} {{.ImportPath}}", "./...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list over the module failed, so the `ent` package census could not be taken.\n"+
			"    command: %s list -f '{{.Name}} {{.ImportPath}}' ./... (in %s)\n"+
			"    error:   %v\n    output:\n%s", goTool, root, err, out)
	}

	var named []string
	for _, line := range strings.Split(string(out), "\n") {
		name, importPath, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || name != "ent" {
			continue
		}
		named = append(named, importPath)
	}
	sort.Strings(named)

	var underFixtures []string
	for _, p := range named {
		if strings.Contains(p, fixturesPrefix) {
			underFixtures = append(underFixtures, p)
		}
	}

	if len(named) <= 1 && len(underFixtures) == 0 {
		return
	}

	t.Errorf("%d package(s) in this module are named `ent`; %d of them are under internal/fixtures.\n"+
		"Rule: at most one package named `ent` module-wide, and none under internal/fixtures.\n"+
		"A generated file that references `ent.` without spelling the import out is resolved by\n"+
		"goimports against ALL of these, and the winner depends on its cache state (#49).\n\n"+
		"Packages named `ent`:\n%s\n"+
		"Each fixture's generated package must carry a distinct name. The Go package name comes\n"+
		"from the last element of the import path, so the mechanism is the directory name:\n"+
		"internal/fixtures/<dir>/<dir>ent, giving `package <dir>ent`.",
		len(named), len(underFixtures), formatPkgList(named))
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
