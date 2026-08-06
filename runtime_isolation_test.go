package entdomain

import (
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// runtimePackage is the package a consumer's PRODUCTION binary links: the one
// generated code imports for ErrValidation, ListRequest, ListPage and the rest
// of Layer 2.
//
// generatorPackage is the package ent loads at generation time: the extension,
// the annotations, the template functions and the embedded templates.
//
// #15 is the assertion that these two are different packages. They were the
// same string until it landed, and while they were, every criterion below that
// names runtimePackage was a statement about the generator too — which was
// exactly the defect: #4 fixed the Windows path bug inside mustLoadTemplate,
// but could not stop mustLoadTemplate from running in a binary that only wanted
// a sentinel error, because the sentinel and the loader shared a package.
//
// runtimePackage must stay equal to defaultEntDomainPackage: that constant is
// what generated code imports, so a test pointed anywhere else would prove
// isolation for a package nobody links.
const (
	runtimePackage   = defaultEntDomainPackage
	generatorPackage = "github.com/githonllc/entdomain"
)

// forbiddenRuntimeDeps are the packages whose reachability from runtimePackage
// would falsify #15's acceptance criteria, each paired with the criterion it
// answers.
var forbiddenRuntimeDeps = map[string]string{
	"embed":                       `criterion 3 ("neither embeds nor loads templates"): //go:embed requires importing "embed", so if "embed" is unreachable from the runtime package, NOTHING in its transitive closure can embed a file`,
	generatorPackage:              `criterion 3, load half: mustLoadTemplate runs from template_index.go's package-level vars, so reaching the generator package at all means running the template loader at init`,
	"golang.org/x/tools/imports":  `criterion 1 ("imports neither the ent codegen packages nor the source formatter"): the formatter`,
	"entgo.io/ent/entc/gen":       `criterion 1: the ent codegen packages`,
	"entgo.io/ent/entc":           `criterion 1: the ent codegen packages`,
	"entgo.io/ent/entc/load":      `criterion 1: the ent codegen packages`,
	"entgo.io/ent/entc/integrate": `criterion 1: the ent codegen packages`,
}

// listedPackage is the subset of `go list -json` this test reads.
type listedPackage struct {
	ImportPath string
	EmbedFiles []string
}

// TestRuntimePackageIsGeneratorFree is #15's mechanical proof, and it is
// mechanical rather than a reading of import blocks for a specific reason: an
// import that is not ent can still pull ent in three hops down, and no amount
// of staring at a file settles that. `go list -deps` computes the transitive
// closure the linker will actually walk.
//
// It is falsifiable in both directions, which is the point of the control
// below. A probe that only ever asserts an absence passes just as happily when
// it is broken — a typo'd package path, a `go list` that failed, an empty
// closure. So the same probe is pointed at the generator package and REQUIRED
// to find "embed" and a non-empty EmbedFiles there. If the generator half ever
// stops embedding templates, this test fails loudly and asks to be rewritten,
// instead of silently becoming a rubber stamp.
func TestRuntimePackageIsGeneratorFree(t *testing.T) {
	t.Run("control/the probe can detect an embedded template", func(t *testing.T) {
		pkgs := listDeps(t, generatorPackage)

		if _, ok := pkgs["embed"]; !ok {
			t.Fatalf("the generator package %s does not reach \"embed\"; the probe below "+
				"would then assert an absence it cannot distinguish from a broken query",
				generatorPackage)
		}
		self, ok := pkgs[generatorPackage]
		if !ok {
			t.Fatalf("go list -deps %s did not report the package itself", generatorPackage)
		}
		if len(self.EmbedFiles) == 0 {
			t.Fatalf("the generator package %s embeds no files; this test's control is void",
				generatorPackage)
		}
		t.Logf("control: %s embeds %v and reaches \"embed\"", generatorPackage, self.EmbedFiles)
	})

	pkgs := listDeps(t, runtimePackage)
	if len(pkgs) == 0 {
		t.Fatalf("go list -deps %s returned nothing; the assertions below would pass vacuously",
			runtimePackage)
	}

	// Criterion 3, embed half: nothing in the closure embeds a file.
	if self := pkgs[runtimePackage]; len(self.EmbedFiles) > 0 {
		t.Errorf("the runtime package %s embeds %v.\n"+
			"    Importing it for a sentinel error therefore copies the templates into the consumer's binary.",
			runtimePackage, self.EmbedFiles)
	}

	// Criteria 1 and 3: the closure must not reach the generator's machinery.
	for path, why := range forbiddenRuntimeDeps {
		if _, ok := pkgs[path]; ok {
			t.Errorf("the runtime package %s reaches %q.\n    %s", runtimePackage, path, why)
		}
	}

	// Criterion 1, stated as a count rather than as a list, because the exact
	// set of ent packages is not the invariant — zero of them is.
	var ent []string
	for path := range pkgs {
		if strings.HasPrefix(path, "entgo.io/") {
			ent = append(ent, path)
		}
	}
	if len(ent) > 0 {
		t.Errorf("the runtime package %s reaches %d entgo.io packages; the runtime must link without ent.\n    %v",
			runtimePackage, len(ent), ent)
	}
}

// listDeps returns the transitive closure of pkg, keyed by import path.
//
// It shells out to the real go command rather than parsing imports here: the
// closure is what the toolchain computes, and a reimplementation of it would
// share this package's own blind spots.
func listDeps(t *testing.T, pkg string) map[string]listedPackage {
	t.Helper()

	cmd := exec.Command(goToolPath(t), "list", "-deps", "-json", pkg)
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("go list -deps -json %s: %v\n%s", pkg, err, stderr)
	}

	pkgs := make(map[string]listedPackage)
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var p listedPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decoding go list output for %s: %v", pkg, err)
		}
		pkgs[p.ImportPath] = p
	}
	return pkgs
}
