package entapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// This file closes the last three rows of #17's inventory: DomainField's
// Validation, Description and Example, plus the DomainConfig type that #17 and
// #29 emptied of every field.
//
// The three knobs were recorded UNDECIDED because the owner's verdict on #17
// enumerated the query markers, the metadata block and EntityName but not
// these. Their disposition here applies the two rules that verdict already
// established, rather than inventing a third:
//
//   - A knob with a SUCCESSOR is deleted, not kept. #26 made Validate() the
//     only door to Apply on a Valid{E}…Request, so handler-layer validation now
//     lives somewhere it cannot be skipped. Validation restates a rule nothing
//     in this package enforces, which is the drift #24 removed from the struct
//     tags rather than widening.
//
//   - A knob that belongs to the OpenAPI schema family is kept, on
//     FieldMetadata, under the RESERVED doc comment #3's adjudication called "a
//     documented forward contract, not dead weight". Description and Example
//     are two of the most common OpenAPI schema fields; sitting on DomainField
//     while Title, Format, Pattern, Enum and the rest sit on FieldMetadata was
//     an inconsistency, not a placement decision.
//
// The builders are unchanged in shape: WithDescription and WithExample still
// exist and still chain. Only where the value is stored moved, which is why the
// #5 copy-on-write contract is re-asserted below against the new location.

// TestValidationKnobIsRemoved pins the deletion of DomainField.Validation and
// its builder. Validate() on the generated request types supersedes it.
func TestValidationKnobIsRemoved(t *testing.T) {
	if _, ok := reflect.TypeOf(DomainField{}).FieldByName("Validation"); ok {
		t.Error("DomainField still declares Validation; #26's Validate() supersedes it, and a knob " +
			"restating a rule nothing here enforces is the drift #24 removed from the struct tags")
	}

	_, funcs := packageDecls(t)
	if funcs["DomainField.WithValidation"] {
		t.Error("WithValidation is still declared; it has no replacement because Validate() on the " +
			"generated Valid{E}…Request is where handler-layer validation lives now")
	}
}

// TestDescriptionAndExampleLiveOnFieldMetadata pins the move rather than a
// deletion: both are OpenAPI schema fields and belong with their siblings.
func TestDescriptionAndExampleLiveOnFieldMetadata(t *testing.T) {
	df := reflect.TypeOf(DomainField{})
	for _, name := range []string{"Description", "Example"} {
		if _, ok := df.FieldByName(name); ok {
			t.Errorf("DomainField still declares %s; it belongs on FieldMetadata with the rest of the "+
				"OpenAPI schema family, under the RESERVED forward contract #3's adjudication upheld", name)
		}
	}

	fm := reflect.TypeOf(FieldMetadata{})

	desc, ok := fm.FieldByName("Description")
	if !ok {
		t.Fatal("FieldMetadata has no Description field; moving it there is the point of this change")
	}
	if desc.Type.Kind() != reflect.String {
		t.Errorf("FieldMetadata.Description is %s, want string", desc.Type)
	}

	example, ok := fm.FieldByName("Example")
	if !ok {
		t.Fatal("FieldMetadata has no Example field; moving it there is the point of this change")
	}
	if example.Type.Kind() != reflect.Interface {
		t.Errorf("FieldMetadata.Example is %s, want an interface type so any literal can be given", example.Type)
	}
}

// TestDomainConfigIsNotPublished pins the removal of the type itself. #17 took
// EntityName and #29 took the base-service and base-handler switches, leaving a
// published entity-level annotation with nothing to say.
func TestDomainConfigIsNotPublished(t *testing.T) {
	types, _ := packageDecls(t)
	if types["DomainConfig"] {
		t.Error("DomainConfig is still declared; it has carried no options since #29, and a published " +
			"annotation that changes no generated output is exactly what #17 exists to remove")
	}
}

// TestPendingKnobsRecordsTheMovedMetadataFields checks the bookkeeping half:
// the three UNDECIDED entries are gone, and the two moved fields carry the same
// pending status as the rest of FieldMetadata rather than a new reason of their
// own.
func TestPendingKnobsRecordsTheMovedMetadataFields(t *testing.T) {
	for _, name := range []string{"DomainField.Validation", "DomainField.Description", "DomainField.Example"} {
		if reason, ok := pendingKnobs[name]; ok {
			t.Errorf("pendingKnobs still lists %q as %q; the knob is gone from DomainField, so the entry "+
				"exempts nothing", name, reason)
		}
	}

	want, ok := pendingKnobs["FieldMetadata.Title"]
	if !ok {
		t.Fatal("pendingKnobs has no FieldMetadata.Title entry to match against")
	}
	for _, name := range []string{"FieldMetadata.Description", "FieldMetadata.Example"} {
		got, ok := pendingKnobs[name]
		if !ok {
			t.Errorf("pendingKnobs has no %q entry; the field moved onto FieldMetadata and inherits its "+
				"pending status", name)
			continue
		}
		if got != want {
			t.Errorf("pendingKnobs[%q] = %q, want %q — the moved fields wait on the same spec generation "+
				"as the rest of the block, not on a reason of their own", name, got, want)
		}
	}
}

// TestWithDescriptionAndWithExampleStoreOnMetadata is the caller-facing half of
// the move: both builders still exist and still chain, and the value now lands
// on the metadata block.
func TestWithDescriptionAndWithExampleStoreOnMetadata(t *testing.T) {
	field := NewDomainField().
		WithDescription("Unique entity identifier").
		WithExample("7c9e6679-7425-40de-944b-e07fc1f90ae7")

	if got := metadataField(t, field, "Description"); got != "Unique entity identifier" {
		t.Errorf("Metadata.Description = %v, want %q", got, "Unique entity identifier")
	}
	if got := metadataField(t, field, "Example"); got != "7c9e6679-7425-40de-944b-e07fc1f90ae7" {
		t.Errorf("Metadata.Example = %v, want %q", got, "7c9e6679-7425-40de-944b-e07fc1f90ae7")
	}
}

// TestDescriptionAndExampleForkedChainsAreIndependent re-asserts #5 against the
// new storage location. Both builders now write through a *FieldMetadata, which
// is precisely the aliasing shape #5 was about: without ensureMetadata's
// copy-on-write, two chains forked from one base would write into one block and
// each would observe the other.
func TestDescriptionAndExampleForkedChainsAreIndependent(t *testing.T) {
	base := DefaultField().WithDescription("base").WithExample("base")
	a := base.WithDescription("a").WithExample("a")
	b := base.WithDescription("b").WithExample("b")

	if a.Metadata == b.Metadata {
		t.Errorf("forked chains share one *FieldMetadata (%p)", a.Metadata)
	}
	if a.Metadata == base.Metadata {
		t.Errorf("fork a shares the base's *FieldMetadata (%p)", a.Metadata)
	}

	for _, tc := range []struct {
		label string
		field DomainField
		want  string
	}{
		{"a", a, "a"},
		{"b", b, "b"},
		{"base", base, "base"},
	} {
		if got := metadataField(t, tc.field, "Description"); got != tc.want {
			t.Errorf("%s.Metadata.Description = %v, want %q; a fork must not see its sibling's writes",
				tc.label, got, tc.want)
		}
		if got := metadataField(t, tc.field, "Example"); got != tc.want {
			t.Errorf("%s.Metadata.Example = %v, want %q; a fork must not see its sibling's writes",
				tc.label, got, tc.want)
		}
	}
}

// metadataField reads a named field off the annotation's metadata block by
// reflection. It is written this way on purpose: it asserts *where* the value
// lives, so it compiles — and fails — against a layout that still keeps the
// field on DomainField.
func metadataField(t *testing.T, d DomainField, name string) interface{} {
	t.Helper()

	if d.Metadata == nil {
		t.Fatalf("annotation has no Metadata block, so %s was not stored on it", name)
	}
	v := reflect.ValueOf(*d.Metadata).FieldByName(name)
	if !v.IsValid() {
		t.Fatalf("FieldMetadata has no %s field; %s must live on the metadata block", name, name)
	}
	return v.Interface()
}

// packageDecls parses this package's non-test sources and returns the declared
// type names and the declared function names, methods keyed as "Receiver.Name".
//
// Reflection cannot answer "is this type still declared?" — naming the type is
// what makes the question compile — so the source is the only channel that can.
func packageDecls(t *testing.T) (types map[string]bool, funcs map[string]bool) {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package directory: %v", err)
	}

	types = make(map[string]bool)
	funcs = make(map[string]bool)
	fset := token.NewFileSet()
	parsed := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		parsed++

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						types[ts.Name.Name] = true
					}
				}
			case *ast.FuncDecl:
				funcs[declKey(d)] = true
			}
		}
	}

	if parsed == 0 {
		t.Fatal("parsed no package sources; this test would pass vacuously")
	}
	return types, funcs
}

func declKey(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}

	recv := d.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name + "." + d.Name.Name
	}
	return d.Name.Name
}
