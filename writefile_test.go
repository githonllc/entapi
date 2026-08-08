package entapi

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

func TestFormatFile_FormattingFailureReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "widget_dto.go")

	_, err := formatFile(path, []byte(unparseableSource))
	if err == nil {
		t.Fatal("formatFile returned nil for source goimports cannot parse; " +
			"a formatting failure must abort generation")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the target path %q", err, path)
	}
}

// TestFormatFile_TouchesNoDisk is what makes formatFile usable as phase 1's
// gate: it is handed the path only so imports.Process can resolve against it
// and so the error can name the file. Writing there — on success or failure —
// would put the run's atomicity back where it was before #61.
func TestFormatFile_TouchesNoDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "widget_dto.go")

	if _, err := formatFile(path, []byte(validSource)); err != nil {
		t.Fatalf("formatFile on valid source: %v", err)
	}
	_, _ = formatFile(path, []byte(unparseableSource))

	assertOnlyFiles(t, dir)
}

func TestWriteFormatted_SuccessLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "widget_dto.go")

	if err := writeFormatted(path, []byte(validSource)); err != nil {
		t.Fatalf("writeFormatted on valid source: %v", err)
	}
	assertOnlyFiles(t, dir, "widget_dto.go")
}

// TestWriteFormatted_CreatesMissingDirectory covers the MkdirAll prerequisite:
// the writer used os.WriteFile directly, so generating into a directory that
// does not exist yet failed on the first run.
func TestWriteFormatted_CreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dto", "widget_dto.go")

	if err := writeFormatted(path, []byte(validSource)); err != nil {
		t.Fatalf("writeFormatted into a missing directory: %v", err)
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
