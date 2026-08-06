package entdomain

import (
	"regexp"
	"strings"
	"testing"
)

// entdomainSymbols are this package's sentinels, which have no counterpart in
// `package ent` and must therefore always carry the entdomain qualifier.
var entdomainSymbols = []string{"ErrNotFound", "ErrAlreadyExists", "ErrValidation"}

// templateComment matches a {{/* ... */}} block. Comments are stripped before
// the scan below: the assertion is about the Go source the template emits, and
// the comments explaining the resolution necessarily name the symbols they warn
// against.
var templateComment = regexp.MustCompile(`(?s)\{\{/\*.*?\*/\}\}`)

// TestTemplatesQualifyEntdomainSentinels is the half of the resolution rule
// that applies to every template rather than to one call.
//
// The emitted files land in the consumer's `package ent`, which has no
// ErrNotFound, ErrAlreadyExists or ErrValidation of its own. A bare reference
// therefore does not compile — but it is the kind of mistake a template makes
// silently while it is being edited, and only the fixture build would catch it.
// This catches it in the source.
//
// It used to be asserted against base_service.tmpl alone, together with the
// converse rule for IsConstraintError. That template is gone (#29) and nothing
// generated classifies errors any more — that belongs to the runtime and is
// #13 — so IsConstraintError has no call site left to pin. The one remaining
// unqualified Ent predicate, dto.tmpl's IsNotFound, is pinned below.
func TestTemplatesQualifyEntdomainSentinels(t *testing.T) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("reading embedded templates failed: %v", err)
	}

	scanned := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}
		src, err := templateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			t.Fatalf("reading embedded %s failed: %v", entry.Name(), err)
		}
		scanned++
		text := templateComment.ReplaceAllString(string(src), "")

		for _, name := range entdomainSymbols {
			bare := regexp.MustCompile(`([^.\w])` + name + `\b`)
			for _, m := range bare.FindAllStringSubmatch(text, -1) {
				t.Errorf("templates/%s references %s without the entdomain qualifier (matched %q); package ent has no such symbol", entry.Name(), name, m[0])
			}
		}
	}

	if scanned == 0 {
		t.Fatal("no embedded templates were scanned; this test would pass vacuously")
	}
}

// TestDTOTemplateResolvesIsNotFoundToEnt extends the same rule to dto.tmpl,
// which acquired an IsNotFound call with the response constructors: a to-one
// edge that was loaded but matched no row comes back from <Edge>OrErr() as
// Ent's *NotFoundError, and telling that apart from a not-loaded edge is the
// whole contract. entdomain.IsNotFound tests this package's sentinels instead,
// so qualifying the call would compile and silently route every loaded-but-
// absent edge into the error branch.
func TestDTOTemplateResolvesIsNotFoundToEnt(t *testing.T) {
	src, err := templateFS.ReadFile("templates/dto.tmpl")
	if err != nil {
		t.Fatalf("reading embedded dto.tmpl failed: %v", err)
	}
	text := templateComment.ReplaceAllString(string(src), "")

	if !strings.Contains(text, "IsNotFound(err)") {
		t.Error("dto.tmpl no longer calls unqualified IsNotFound(err); if the edge contract changed, update this test")
	}
	qualified := regexp.MustCompile(`[\w.]+\.IsNotFound\b`)
	if m := qualified.FindAllString(text, -1); len(m) > 0 {
		t.Errorf("dto.tmpl qualifies IsNotFound as %v; it must stay unqualified so it binds to Ent's generated predicate in package ent", m)
	}
}
