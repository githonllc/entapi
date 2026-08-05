package entdomain

import (
	"reflect"
	"strings"

	"entgo.io/ent/entc/gen"
)

// isTimeField checks if a field is a time field.
func isTimeField(field *gen.Field) bool {
	return strings.Contains(field.Type.String(), "time.Time")
}

// hasTimeFields checks if the entity has any time fields.
func hasTimeFields(node *gen.Type) bool {
	for _, field := range domainFields(node) {
		if isTimeField(field) {
			return true
		}
	}
	return false
}

// hasSoftDelete checks if an entity has a deleted_at field (convention-based soft-delete detection).
// Returns true if the entity has a Nillable, Optional time.Time field named "deleted_at".
func hasSoftDelete(node *gen.Type) bool {
	for _, field := range node.Fields {
		if field.Name == "deleted_at" && isTimeField(field) && field.Nillable {
			return true
		}
	}
	return false
}

// isComplexFieldType reports whether a field's Go type is one Go's comparable
// constraint rejects — slices, maps and functions, including named types whose
// underlying type is one of those.
//
// It decides which pointer helper the generated response constructor calls:
// entdomain.PtrOrNil is [T comparable] and does not compile for such a type,
// so those fields must go to entdomain.PtrNilSafe instead.
//
// The resolved type is the authority, not its rendered name. A named type
// declared as `type Tags []string` renders as "schema.Tags" and matches no
// prefix, which is exactly how it used to reach PtrOrNil and fail the
// consumer's build (#10). ent records the reflect kind of a Go type on
// Type.RType, so ask that first and fall back to the rendered name only for
// fields that carry no RType (built-in JSON descriptors declared through the
// type string alone).
func isComplexFieldType(f *gen.Field) bool {
	if f == nil || f.Type == nil {
		return false
	}
	if rt := f.Type.RType; rt != nil {
		switch rt.Kind {
		case reflect.Slice, reflect.Map, reflect.Func:
			return true
		}
	}
	name := f.Type.String()
	return strings.HasPrefix(name, "[]") ||
		strings.HasPrefix(name, "map[") ||
		strings.Contains(name, "json.")
}
