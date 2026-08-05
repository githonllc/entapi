package entdomain

import (
	"strings"
	"text/template"
)

// templateFuncs returns the template function map for code generation.
// Only functions actually invoked by templates/*.tmpl are registered here.
// Internal helper functions used by Go code only are NOT registered.
//
// Source files:
//   - funcs_strings.go:    string manipulation utilities
//   - funcs_fields.go:     field filtering and selection
//   - funcs_scope.go:      scope and requirement checking
//   - funcs_typechecks.go: field type checking
//   - funcs_codegen.go:    code generation helpers
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// String manipulation
		"lower":     strings.ToLower,
		"camelCase": camelCase,
		"hasPrefix": hasPrefix,

		// Field selection (used in template range loops)
		"domainFields":       domainFields,
		"createFields":       createFields,
		"updateFields":       updateFields,
		"responseFields":     responseFields,
		"uniqueLookupFields": uniqueLookupFields,
		"rangeLookupFields":  rangeLookupFields,
		"responseEdges":      responseEdges,

		// Scope and requirement checking
		"isDomainRequired": isDomainRequired,

		// Field type checking
		"isUniqueField":      isUniqueField,
		"isUUIDType":         isUUIDType,
		"hasTimeFields":      hasTimeFields,
		"hasTimeField":       hasTimeField,
		"isComplexFieldType": isComplexFieldType,
		"hasSoftDelete":      hasSoftDelete,

		// Code generation helpers
		"setFieldCallReq": setFieldCallReq,
		"searchMethod":    searchMethod,
		"findByMethod":    findByMethod,
		"last":            last,

		// Utility functions
		"contains": contains,

		// Template code generation helpers
		"generateIdOperation":     generateIdOperation,
		"generateSearchCondition": generateSearchCondition,
	}
}
