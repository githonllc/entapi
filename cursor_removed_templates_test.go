package entdomain

import (
	"io/fs"
	"strings"
	"testing"
)

// removedCursorSymbols is the exclusion list decided on #6 and recorded in
// docs/DESIGN-v2.md §9.1.
//
// It is restated here rather than shared with runtime/cursor_removed_test.go
// because the two halves now live in different packages and check different
// things: that one asserts the symbols are not DECLARED, this one asserts no
// template EMITS a reference to them. A shared list would have to be exported
// from one package purely to be read by the other's test.
var removedCursorSymbols = []string{
	"Cursor",
	"PageInfo",
	"EncodeCursor",
	"DecodeCursor",
	"normalizeJSONNumber",
}

// TestNoTemplateEmitsCursorMetadata is the generation half of #6.
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
