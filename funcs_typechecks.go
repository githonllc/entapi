package entdomain

import (
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

// isComplexFieldType checks if a field type is too complex for basic
// operations like sorting (slices, maps, JSON types).
func isComplexFieldType(fieldType string) bool {
	return strings.HasPrefix(fieldType, "[]") ||
		strings.HasPrefix(fieldType, "map[") ||
		strings.Contains(fieldType, "json.")
}
