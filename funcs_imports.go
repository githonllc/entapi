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
	for _, f := range updateFields(node) {
		add(f)
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
