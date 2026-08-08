package entapi

import (
	"encoding/json"

	"entgo.io/ent/entc/gen"
)

// hasDomainScope checks if a field has a specific scope.
func hasDomainScope(field *gen.Field, scope FieldScope) bool {
	annotation := getDomainFieldAnnotation(field)
	if annotation == nil {
		return false
	}

	for _, s := range annotation.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// isDomainRequired checks if a field is required in a specific scope.
func isDomainRequired(field *gen.Field, scope FieldScope) bool {
	annotation := getDomainFieldAnnotation(field)
	if annotation == nil {
		return false
	}

	if annotation.Required == nil {
		return false
	}

	required, exists := annotation.Required[scope]
	return exists && required
}

// getDomainFieldAnnotation extracts a DomainField annotation from a gen.Field.
// Ent annotations arrive as *DomainField at codegen time, but as
// map[string]interface{} when loaded from a serialized schema. This function
// handles both cases using a JSON round-trip for the map case.
func getDomainFieldAnnotation(field *gen.Field) *DomainField {
	annotation, ok := field.Annotations["DomainField"]
	if !ok {
		return nil
	}

	// Direct type — codegen time
	if df, ok := annotation.(*DomainField); ok {
		return df
	}

	// map[string]interface{} — loaded from serialized schema.
	// JSON round-trip handles all fields uniformly.
	if m, ok := annotation.(map[string]interface{}); ok {
		data, err := json.Marshal(m)
		if err != nil {
			return nil
		}
		var df DomainField
		if err := json.Unmarshal(data, &df); err != nil {
			return nil
		}
		return &df
	}

	return nil
}
