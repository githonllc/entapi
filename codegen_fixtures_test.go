package entdomain

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

// modulePath is this module's import path, from go.mod. Generated fixture
// packages live underneath it, which is what lets the generated code import
// entdomain itself without a replace directive.
const modulePath = "github.com/githonllc/entdomain"

// fixtureCase is one fixture directory under internal/fixtures.
//
// Adding coverage is one new directory (internal/fixtures/<dir>/ent/schema,
// holding a hand-written ent schema) plus one line in the fixtures table below.
// Nothing else in the harness needs to change.
type fixtureCase struct {
	// dir is the directory name under internal/fixtures.
	dir string
	// opts are the extension options generation runs with.
	opts []Option
	// wantGenErr, when non-empty, turns the case around: generation is
	// required to FAIL, and its error must contain every listed substring.
	// Nothing is compiled, and nothing may be written under <dir>/ent besides
	// the hand-written schema package — a refused generation that still leaves
	// files behind is a bug of its own.
	//
	// This is how a schema the generator must refuse gets covered. Adding one
	// is still a directory plus a line in the table.
	wantGenErr []string
}

var fixtures = []fixtureCase{
	{dir: "basic", opts: []Option{WithBaseService(true), WithBaseHandler(true)}},
	{dir: "fieldshapes", opts: []Option{WithBaseService(true), WithBaseHandler(true)}},
	{dir: "immutable", opts: []Option{WithBaseService(true), WithBaseHandler(true)}, wantGenErr: []string{"Doc.origin", "Doc.source", "Immutable()", `scope "update"`, "SetOrigin", "SetSource"}},
}

// TestCodegenFixtures is the only test in this repository that proves the
// product of this project — generated Go source — actually builds. Every other
// test asserts on substrings of emitted code, which cannot detect that the
// result does not compile.
//
// For each fixture it runs the real ent code generator with this extension
// installed, then shells out to `go build` over the generated packages and
// fails with the compiler's own output when the build is non-zero.
//
// NOTE: this test WRITES INTO THE REPOSITORY TREE, at
// internal/fixtures/<dir>/ent/. That is deliberate: generated ent code has to
// live inside this Go module to resolve github.com/githonllc/entdomain without
// a network fetch or a replace directive, and t.TempDir() is outside any
// module. The generated output is committed, so a clean checkout plus a test
// run leaves `git status` clean — a dirty tree after running this test means
// generation changed, not that the test misbehaved. Regenerate rather than
// hand-editing anything under internal/fixtures/<dir>/ent/; those files carry
// DO NOT EDIT headers.
func TestCodegenFixtures(t *testing.T) {
	root := repoRoot(t)
	goTool := goToolPath(t)

	for _, fc := range fixtures {
		t.Run(fc.dir, func(t *testing.T) {
			fixtureDir := filepath.Join(root, "internal", "fixtures", fc.dir)
			schemaDir := filepath.Join(fixtureDir, "ent", "schema")
			targetDir := filepath.Join(fixtureDir, "ent")
			pkgPath := modulePath + "/internal/fixtures/" + fc.dir + "/ent"

			if _, err := os.Stat(schemaDir); err != nil {
				t.Fatalf("fixture %q: schema directory %s: %v", fc.dir, schemaDir, err)
			}

			err := entc.Generate(schemaDir, &gen.Config{
				Target:  targetDir,
				Package: pkgPath,
			}, entc.Extensions(NewExtensionWithOptions(fc.opts...)))

			if len(fc.wantGenErr) > 0 {
				assertGenerationRefused(t, fc, targetDir, err)
				return
			}

			if err != nil {
				t.Fatalf("fixture %q: ent generation failed for schema %s: %v", fc.dir, schemaDir, err)
			}

			pattern := "./internal/fixtures/" + fc.dir + "/..."
			cmd := exec.Command(goTool, "build", pattern)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("fixture %q: generated output does not compile.\n"+
					"    generated into: %s\n"+
					"    command:        %s build %s (in %s)\n"+
					"    error:          %v\n"+
					"    compiler output:\n%s",
					fc.dir, targetDir, goTool, pattern, root, err, out)
			}
		})
	}
}

// assertGenerationRefused is the wantGenErr half of the harness: a schema the
// generator must refuse has to produce an error naming what is wrong, and has
// to leave the fixture's ent directory untouched. "It failed" is not enough —
// the message is the only thing a schema author gets, so its content is the
// assertion.
func assertGenerationRefused(t *testing.T, fc fixtureCase, targetDir string, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("fixture %q: generation succeeded, but this schema must be refused (wantGenErr %q)", fc.dir, fc.wantGenErr)
	}
	for _, want := range fc.wantGenErr {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("fixture %q: generation error does not mention %q.\ngot: %v", fc.dir, want, err)
		}
	}

	// Log it even on success: the message is what a schema author sees, and
	// `go test -v -run TestCodegenFixtures/<dir>` is how you read it back.
	t.Logf("fixture %q: generation refused, as required:\n%v", fc.dir, err)

	entries, readErr := os.ReadDir(targetDir)
	if readErr != nil {
		t.Fatalf("fixture %q: cannot read %s: %v", fc.dir, targetDir, readErr)
	}
	for _, entry := range entries {
		if entry.Name() == "schema" {
			continue
		}
		t.Errorf("fixture %q: generation was refused but wrote %s into %s; a refused generation must leave nothing behind",
			fc.dir, entry.Name(), targetDir)
	}
}

// repoRoot returns the directory holding this test file. Paths are derived from
// the test's own location rather than the process working directory so the
// harness does not depend on where `go test` was invoked from.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine the location of this test file")
	}
	return filepath.Dir(file)
}

// goToolPath locates the go command used to compile the generated output.
func goToolPath(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("go"); err == nil {
		return path
	}
	path := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cannot locate the go tool: not on PATH and not at %s", path)
	}
	return path
}
