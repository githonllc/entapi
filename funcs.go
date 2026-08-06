package entdomain

import (
	"text/template"
)

// templateFuncs returns the functions this package adds to the template
// namespace for code generation.
//
// Two properties hold, and both are enforced by tests rather than by this
// comment (template_funcs_consistency_test.go):
//
//   - Every entry is invoked by at least one templates/*.tmpl. An entry no
//     template calls is deleted, not kept "for later": several such entries
//     used to emit code for a repository-shaped API that the generated
//     BaseService no longer has, so reconnecting one would have produced wrong
//     queries rather than a compile error.
//   - No entry collides with an Ent builtin. templateFuncMap() overlays this
//     map onto gen.Funcs, so a same-named entry would silently replace Ent's
//     version for every template. Anything Ent already supplies — lower,
//     hasPrefix, camel, snake, plural, … — is used from gen.Funcs directly.
//
// Internal helpers used only by Go code are not registered.
//
// Source files:
//   - funcs_strings.go:    string manipulation utilities
//   - funcs_fields.go:     field filtering and selection
//   - funcs_scope.go:      scope and requirement checking
//   - funcs_softdelete.go: the soft-delete mixin's marker and tombstone field
//   - funcs_presence.go:   create/patch field shape and presence rules
//   - funcs_typechecks.go: field type checking
//   - funcs_codegen.go:    code generation helpers
//   - funcs_imports.go:    import specs the emitted files must declare
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// Import declaration
		"dtoImports":        dtoImports,
		"filterImports":     filterImports,
		"wiringImports":     wiringImports,
		"softDeleteImports": softDeleteImports,

		// String manipulation
		"camelCase": camelCase,

		// Field selection (used in template range loops)
		"domainFields":   domainFields,
		"createFields":   createFields,
		"patchFields":    patchFields,
		"responseFields": responseFields,
		"responseEdges":  responseEdges,
		"edgeJSONKey":    edgeJSONKey,

		// Request presence model
		"isCreatePointer":  isCreatePointer,
		"isCreateRequired": isCreateRequired,
		"isPatchClearable": isPatchClearable,

		// Query surface: the outer loop is the scope, the inner test is the
		// dimension (funcs_filter.go)
		"queryFields":  queryFields,
		"searchFields": searchFields,
		"isFilterable": isFilterable,
		"isSearchable": isSearchable,
		"isSortable":   isSortable,
		"filterParams": filterParams,

		// Field type checking
		"isComplexFieldType": isComplexFieldType,

		// Soft delete: the graph-level traverser (funcs_softdelete.go)
		"softDeleteTypes": softDeleteTypes,
		"softDeleteField": softDeleteField,

		// Code generation helpers
		"fieldValueExpr": fieldValueExpr,
	}
}
