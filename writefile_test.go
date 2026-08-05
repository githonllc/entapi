package entdomain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unparseableSource is what a template regression actually produces: Go source
// that goimports cannot parse. It is the input the generator used to write to
// disk anyway, after logging a warning and returning nil.
const unparseableSource = `package ent

type Broken struct {
	Name string
`

// validSource is syntactically valid Go with no imports to resolve, so
// imports.Process always succeeds on it.
const validSource = `package ent

// Widget is fine.
type Widget struct{}
`

func TestWriteFile_FormattingFailureReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "widget_dto.go")

	err := writeFile(path, []byte(unparseableSource))
	if err == nil {
		t.Fatal("writeFile returned nil for source goimports cannot parse; " +
			"a formatting failure must abort generation")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the target path %q", err, path)
	}
}

func TestWriteFile_FormattingFailureWritesNoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "widget_dto.go")

	_ = writeFile(path, []byte(unparseableSource))

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		got, _ := os.ReadFile(path)
		t.Fatalf("writeFile left a file behind after a formatting failure: %s\n%s", path, got)
	}
	assertOnlyFiles(t, dir)
}

func TestWriteFile_FormattingFailureLeavesPreviousOutputIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "widget_dto.go")

	if err := os.WriteFile(path, []byte(validSource), 0o644); err != nil {
		t.Fatalf("seeding the previous run's output: %v", err)
	}

	_ = writeFile(path, []byte(unparseableSource))

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("previous output disappeared: %v", err)
	}
	if string(got) != validSource {
		t.Fatalf("a failed run overwrote the previous output.\nwant:\n%s\ngot:\n%s", validSource, got)
	}
	assertOnlyFiles(t, dir, "widget_dto.go")
}

func TestWriteFile_SuccessLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "widget_dto.go")

	if err := writeFile(path, []byte(validSource)); err != nil {
		t.Fatalf("writeFile on valid source: %v", err)
	}
	assertOnlyFiles(t, dir, "widget_dto.go")
}

// TestWriteFile_CreatesMissingDirectory covers the MkdirAll prerequisite: the
// writer used os.WriteFile directly, so generating into a directory that does
// not exist yet failed on the first run.
func TestWriteFile_CreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dto", "widget_dto.go")

	if err := writeFile(path, []byte(validSource)); err != nil {
		t.Fatalf("writeFile into a missing directory: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

// assertOnlyFiles fails when dir holds anything other than want — including the
// temporary files an atomic writer creates and must clean up.
func assertOnlyFiles(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	allowed := make(map[string]bool, len(want))
	for _, name := range want {
		allowed[name] = true
	}
	for _, e := range entries {
		if !allowed[e.Name()] {
			t.Errorf("unexpected leftover file in %s: %s", dir, e.Name())
		}
	}
	if len(entries) != len(want) {
		t.Errorf("directory %s holds %d entries, want %d (%v)", dir, len(entries), len(want), want)
	}
}
