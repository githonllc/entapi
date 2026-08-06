package entdomain

import (
	"fmt"
	"path"
	"sort"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

// dtoImports returns the import specs the DTO file needs for the field types it
// renders, ready to be emitted inside an import block — `"time"`, or
// `alias "some/path"` when the package name does not match the last path
// element.
//
// The DTO writes field types verbatim ({{ $f.Type }}), so the imports it needs
// are a function of which fields are rendered, which is why this cannot be a
// fixed list in the template. goimports would infer them, but then generation
// only works while the formatter does — and the formatter is exactly what fails
// when a template regresses.
//
// Imports the template names itself (fmt, entdomain) are not returned here.
func dtoImports(node *gen.Type) []string {
	if node == nil {
		return nil
	}

	specs := make(map[string]bool)
	add := func(f *gen.Field) {
		if spec := fieldImportSpec(node, f); spec != "" {
			specs[spec] = true
		}
	}

	for _, f := range createFields(node) {
		add(f)
	}
	for _, f := range patchFields(node) {
		add(f)
	}

	// The generated validators call ent's <Field>Validator for every enum a
	// request carries, and those live in the entity's own subpackage — which is
	// not necessarily where the enum's Go type lives. An enum declared with a
	// GoType renders as somepkg.Status, so the type's import says nothing about
	// where the validator is. Ask for it explicitly, exactly when one is emitted.
	if spec := enumValidatorImport(node); spec != "" {
		specs[spec] = true
	}

	// The Response and Summary structs are emitted for EVERY entity the
	// generator handles, so the ID's import is unconditional. An entity whose
	// every annotated field is InputOnly has no response field at all and still
	// has a response carrying its ID — which is precisely the case that used to
	// slip through, because the ID was only added alongside a non-empty
	// responseFields.
	add(node.ID)
	for _, f := range responseFields(node) {
		add(f)
	}

	// Edges need nothing here. An edge renders as <Target>Summary, and every
	// entity type ent generates — along with everything this extension adds
	// beside it — lives in the one generated package, so the reference is
	// package-local by construction.

	out := make([]string, 0, len(specs))
	for spec := range specs {
		out = append(out, spec)
	}
	sort.Strings(out)
	return out
}

// wiringImports returns the import specs the wiring file needs.
//
// All three are unconditional, because every operation the wiring template
// emits is emitted for every annotated entity:
//
//	context                    every operation takes one
//	<the identifier's package>  the id parameter's type, e.g. github.com/google/uuid
//	<pkg>/<entity>             IDEQ, in the by-id lookup
//
// The identifier's package is asked for by field rather than hardcoded: the id
// type comes from the schema, and a builtin one (int, string) needs no import
// at all — fieldImportSpec returns "" for those, and adding an unused import
// would fail generation just as loudly as omitting a needed one.
func wiringImports(node *gen.Type) []string {
	if node == nil {
		return nil
	}

	specs := map[string]bool{`"context"`: true}
	if spec := fieldImportSpec(node, node.ID); spec != "" {
		specs[spec] = true
	}
	if node.Config != nil {
		specs[quoteImport("", node.Config.Package+"/"+node.Package())] = true
	}

	out := make([]string, 0, len(specs))
	for spec := range specs {
		out = append(out, spec)
	}
	sort.Strings(out)
	return out
}

// enumValidatorImport returns the import spec for the entity's own subpackage
// when a create or patch request carries an enum, and "" otherwise.
//
// The condition has to match the template's emission exactly in both
// directions: an import declared without a use is as fatal as a use without an
// import, because formatFile aborts on a formatting failure and
// TestTemplatesDeclareTheirImports fails on either.
func enumValidatorImport(node *gen.Type) string {
	if node.Config == nil {
		return ""
	}
	for _, f := range append(createFields(node), patchFields(node)...) {
		if f.IsEnum() {
			return quoteImport("", node.Config.Package+"/"+node.Package())
		}
	}
	return ""
}

// fieldImportSpec returns the import spec a field's rendered Go type needs, or
// "" when it needs none (builtin types, and types declared in the generated
// package itself).
func fieldImportSpec(node *gen.Type, f *gen.Field) string {
	if f == nil || f.Type == nil {
		return ""
	}

	pkgPath := f.Type.PkgPath
	if pkgPath == "" {
		// An enum without a custom Go type is declared in the entity's own
		// subpackage and renders as <entitypkg>.<Enum>. ent leaves PkgPath
		// empty for it, so the path has to be derived.
		if f.Type.Type == field.TypeEnum && node.Config != nil {
			return quoteImport("", node.Config.Package+"/"+node.Package())
		}
		return ""
	}

	return quoteImport(f.Type.PkgName, pkgPath)
}

// quoteImport formats one import spec, aliasing only when the package name
// differs from the last element of its path.
func quoteImport(pkgName, pkgPath string) string {
	if pkgName != "" && pkgName != path.Base(pkgPath) {
		return fmt.Sprintf("%s %q", pkgName, pkgPath)
	}
	return fmt.Sprintf("%q", pkgPath)
}
