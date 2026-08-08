package entapi

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// guardedTree is the only subtree this package's tests write into.
// TestCodegenFixtures and TestCodegenFixtureStaleArtifacts regenerate
// internal/fixtures/<dir>/<dir>ent in place; nothing else in this package writes to
// the repository at all. So this is exactly the surface the drift guard is
// accountable for.
//
// Deliberately NOT covered:
//
//	internal/fixture        a separate module, the #22 hand-written spike. No
//	                        test regenerates it, so drift there is impossible
//	                        to attribute to a run of this package.
//	internal/softdeleteproof  likewise a separate module, hand-written, never
//	                        a generation target.
//
// internal/fixtures/wiring/e2e is a nested Go module but it is not a git
// submodule, so `git status -- internal/fixtures` already covers it. It holds
// no generated code either way.
//
// The scope is narrow on purpose. A repo-wide check would make the
// already-dirty skip below fire on any unrelated work in progress, which is
// how a guard gets trained out of existence.
const guardedTree = "internal/fixtures"

// driftGuardPrefix marks every line this guard emits. A skip is never silent:
// grep for this string to find out whether the guard actually ran.
const driftGuardPrefix = "regeneration-drift guard: "

// driftDiffMaxLines caps the diff quoted in a failure. The point is to name
// what moved, not to reproduce a regenerated tree on the terminal.
const driftDiffMaxLines = 400

// TestMain owns the drift guard.
//
// The guard has to observe the tree at two moments: before any test has run,
// and after every test has run. That rules out writing it as an ordinary test
// function. Go runs tests in source order within a file, but the order across
// files in a package is not specified, and `go test -shuffle=on` discards it
// entirely — a plain test placed "after" TestCodegenFixtures would be relying
// on an ordering nothing guarantees. TestMain is the only hook in the testing
// package that is defined to bracket the whole run.
//
// The guard only speaks when m.Run() succeeded. A run that already failed has
// its own diagnosis, and a fixture that failed to generate leaves the tree
// dirty as a matter of course; adding a drift report on top of that would bury
// the real error. Turning a green-but-dirty run red is the entire job.
func TestMain(m *testing.M) {
	baseline := newDriftBaseline()

	code := m.Run()

	if code == 0 {
		msg, failed := baseline.verdict()
		if msg != "" {
			fmt.Fprintln(os.Stderr, msg)
		}
		if failed {
			code = 1
		}
	}

	os.Exit(code)
}

// driftBaseline is the state of guardedTree as of process start.
type driftBaseline struct {
	// root is the directory holding this package's source.
	root string
	// skip, when non-empty, explains why the guard cannot draw a conclusion
	// from this run. It is always reported.
	skip string
}

// newDriftBaseline samples guardedTree before any test has had a chance to
// write to it.
func newDriftBaseline() driftBaseline {
	root, ok := sourceDir()
	if !ok {
		return driftBaseline{skip: "cannot determine this package's source directory"}
	}
	if err := requireGitCheckout(root); err != nil {
		return driftBaseline{root: root, skip: err.Error()}
	}

	status, err := fixtureTreeStatus(root)
	if err != nil {
		return driftBaseline{root: root, skip: err.Error()}
	}

	// The pre-dirty case. A developer who edited a fixture schema, or who is
	// mid-way through a template change, has a legitimately dirty
	// internal/fixtures; failing on it would be a false alarm every single run
	// until they commit, and a guard that cries wolf gets ignored.
	//
	// The failure mode this accepts, stated plainly: drift introduced by a run
	// that STARTED dirty is not caught by that run. It is caught by the next
	// run that starts clean — which is every CI run, every fresh checkout, and
	// every run a developer makes after committing. The #49 incident happened
	// on a clean tree straight after a merge, which is precisely the case this
	// covers.
	if strings.TrimSpace(status) != "" {
		return driftBaseline{
			root: root,
			skip: "the tree was ALREADY dirty under " + guardedTree +
				" before these tests ran, so drift caused by this run cannot be told apart from work in progress:\n" +
				indentLines(status),
		}
	}

	return driftBaseline{root: root}
}

// verdict re-reads guardedTree after the run and reports what it found. The
// bool is whether the run must be failed.
func (b driftBaseline) verdict() (string, bool) {
	if b.skip != "" {
		return driftGuardPrefix + "SKIPPED — " + b.skip + "\n" +
			"    Regeneration drift under " + guardedTree + " was NOT checked for this run.", false
	}

	status, err := fixtureTreeStatus(b.root)
	if err != nil {
		return driftGuardPrefix + "SKIPPED — " + err.Error() + "\n" +
			"    Regeneration drift under " + guardedTree + " was NOT checked for this run.", false
	}
	if strings.TrimSpace(status) == "" {
		return "", false
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%sFAILED — regenerating %s changed files that are committed.\n\n", driftGuardPrefix, guardedTree)
	fmt.Fprintf(&sb, "%s was clean before these tests ran, so every path below was rewritten by\n", guardedTree)
	sb.WriteString("this test run. Committed generated output and freshly generated output must be\n")
	sb.WriteString("byte-identical. They are not.\n\n")

	sb.WriteString("Files that drifted (git status --porcelain):\n")
	sb.WriteString(indentLines(status))
	sb.WriteString("\n")

	diff, diffErr := fixtureTreeDiff(b.root)
	switch {
	case diffErr != nil:
		fmt.Fprintf(&sb, "Diff unavailable: %v\n", diffErr)
	case strings.TrimSpace(diff) == "":
		sb.WriteString("No tracked-content diff; the paths above are untracked files that generation\n" +
			"created and nobody committed.\n")
	default:
		fmt.Fprintf(&sb, "Diff (git diff -- %s):\n", guardedTree)
		sb.WriteString(indentLines(truncateLines(diff, driftDiffMaxLines)))
	}

	sb.WriteString("\nIf the diff is an import line being rewritten to another fixture's generated\n")
	sb.WriteString("package, that is #49 returning: goimports resolves a bare reference by PACKAGE\n")
	sb.WriteString("NAME, so two fixtures sharing one made the choice cache-dependent. Each\n")
	sb.WriteString("fixture's package is named after its directory (<dir>ent) to keep them\n")
	sb.WriteString("distinct, and TestNoAmbiguousEntPackages is the standing guard.\n")
	sb.WriteString("Otherwise a template or a schema changed and the regenerated output simply needs\n")
	sb.WriteString("to be committed. Either way: regenerate, inspect, commit. Do not hand-edit\n")
	sb.WriteString("anything under " + guardedTree + "/*/*ent — those files carry DO NOT EDIT headers.\n")

	return sb.String(), true
}

// sourceDir returns the directory holding this file, which is this module's
// root. Derived from the source location rather than the process working
// directory so the guard does not depend on where `go test` was invoked from.
func sourceDir() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	return filepath.Dir(file), true
}

// requireGitCheckout reports why the guard cannot run, or nil if it can.
// Running outside a checkout is a legitimate situation — a vendored copy, a
// source tarball — and has to skip rather than fail.
func requireGitCheckout(root string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is not on PATH (%v)", err)
	}
	cmd := exec.Command("git", "-C", root, "rev-parse", "--is-inside-work-tree")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s is not a git checkout (%v: %s)", root, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// fixtureTreeStatus is the porcelain status of guardedTree. Untracked files are
// included: generation writing a file nobody committed is drift too.
func fixtureTreeStatus(root string) (string, error) {
	return runGit(root, "status", "--porcelain", "--untracked-files=all", "--", guardedTree)
}

// fixtureTreeDiff is the tracked-content diff of guardedTree, used only to
// build the failure message.
func fixtureTreeDiff(root string) (string, error) {
	return runGit(root, "--no-pager", "diff", "--", guardedTree)
}

func runGit(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s failed (%v: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func indentLines(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "    " + line
	}
	return strings.Join(lines, "\n") + "\n"
}

func truncateLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n") +
		fmt.Sprintf("\n... %d more diff lines; run `git diff -- %s` to see all of it\n", len(lines)-max, guardedTree)
}
