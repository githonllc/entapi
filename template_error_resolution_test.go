package entdomain

import (
	"regexp"
	"strings"
	"testing"
)

// entPredicates are error predicates that must stay UNQUALIFIED in the emitted
// source. base_service.tmpl renders into `package ent` (the file lands next to
// Ent's own generated code), so a bare IsNotFound binds to Ent's generated
// IsNotFound, which unwraps *ent.NotFoundError.
//
// This package also exports an IsNotFound (errors.go), and it is NOT the same
// predicate: it reports errors.Is(err, ErrNotFound) against this package's
// sentinels. Qualifying the call as entdomain.IsNotFound would still compile
// and would silently stop matching Ent's not-found error — a wrong-behaviour
// change with no compiler backstop. That is the regression this test pins.
var entPredicates = []string{"IsNotFound", "IsConstraintError"}

// entdomainSymbols are this package's sentinels, which have no counterpart in
// `package ent` and must therefore always carry the entdomain qualifier.
var entdomainSymbols = []string{"ErrNotFound", "ErrAlreadyExists", "ErrValidation"}

// templateComment matches a {{/* ... */}} block. Comments are stripped before
// the scan below: the assertion is about the Go source the template emits, and
// the comments explaining the resolution necessarily name the symbols they warn
// against.
var templateComment = regexp.MustCompile(`(?s)\{\{/\*.*?\*/\}\}`)

func TestGeneratedErrorPredicatesResolveUnambiguously(t *testing.T) {
	src, err := templateFS.ReadFile("templates/base_service.tmpl")
	if err != nil {
		t.Fatalf("reading embedded base_service.tmpl failed: %v", err)
	}
	text := templateComment.ReplaceAllString(string(src), "")

	for _, name := range entPredicates {
		qualified := regexp.MustCompile(`[\w.]+\.` + name + `\b`)
		if m := qualified.FindAllString(text, -1); len(m) > 0 {
			t.Errorf("base_service.tmpl qualifies %s as %v; it must stay unqualified so it binds to Ent's generated predicate in package ent, not to entdomain.%s (which tests this package's sentinels instead)", name, m, name)
		}
		if !strings.Contains(text, name+"(err)") {
			t.Errorf("base_service.tmpl no longer calls unqualified %s(err); if that is intentional, update this test and the note in errors.go", name)
		}
	}

	for _, name := range entdomainSymbols {
		bare := regexp.MustCompile(`([^.\w])` + name + `\b`)
		for _, m := range bare.FindAllStringSubmatch(text, -1) {
			t.Errorf("base_service.tmpl references %s without the entdomain qualifier (matched %q); package ent has no such symbol", name, m[0])
		}
	}
}
