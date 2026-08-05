package entdomain

import (
	"testing"
)

func TestTemplateFuncs(t *testing.T) {
	funcs := templateFuncs()

	// Test that all expected functions are present
	expectedFuncs := []string{
		"domainFields", "createFields", "updateFields", "responseFields",
		"isDomainRequired",
		"searchMethod", "findByMethod", "generateIdOperation",
		"camelCase",
	}

	for _, funcName := range expectedFuncs {
		if _, exists := funcs[funcName]; !exists {
			t.Errorf("Expected template function %s not found", funcName)
		}
	}
}

func TestGetDomainFieldAnnotationFromMap(t *testing.T) {
	// Test with map[string]interface{} annotation (runtime format)
	mapAnnotation := map[string]interface{}{
		"scopes":      []interface{}{"create", "update", "response"},
		"required":    map[string]interface{}{"create": true},
		"searchable":  true,
		"filterable":  true,
		"sortable":    true,
		"description": "Test field description",
	}

	annotation := convertMapToDomainField(mapAnnotation)

	if annotation == nil {
		t.Fatal("Expected annotation to be converted")
	}

	if annotation.Description != "Test field description" {
		t.Errorf("Expected description 'Test field description', got '%s'", annotation.Description)
	}

	if !annotation.Searchable {
		t.Error("Field should be searchable")
	}

	if !annotation.Filterable {
		t.Error("Field should be filterable")
	}

	if !annotation.Sortable {
		t.Error("Field should be sortable")
	}

	// Test scopes conversion
	expectedScopes := []FieldScope{ScopeCreate, ScopeUpdate, ScopeResponse}
	if len(annotation.Scopes) != len(expectedScopes) {
		t.Errorf("Expected %d scopes, got %d", len(expectedScopes), len(annotation.Scopes))
	}

	// Test required map conversion
	if annotation.Required == nil {
		t.Error("Required map should be initialized")
	}

	if !annotation.Required[ScopeCreate] {
		t.Error("Field should be required for create scope")
	}
}

func TestConvertInterfaceSliceToFieldScopes(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected []FieldScope
	}{
		{
			name:     "string scopes",
			input:    []interface{}{"create", "update", "response"},
			expected: []FieldScope{ScopeCreate, ScopeUpdate, ScopeResponse},
		},
		{
			name:     "empty slice",
			input:    []interface{}{},
			expected: []FieldScope{},
		},
		{
			name:     "mixed types (should handle gracefully)",
			input:    []interface{}{"create", 123, "update"},
			expected: []FieldScope{ScopeCreate, ScopeUpdate},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertInterfaceSliceToFieldScopes(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d scopes, got %d", len(tt.expected), len(result))
			}

			for i, expected := range tt.expected {
				if i >= len(result) || result[i] != expected {
					t.Errorf("Expected scope %s at index %d, got %s", expected, i, result[i])
				}
			}
		})
	}
}

func TestConvertInterfaceMapToBoolMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[FieldScope]bool
	}{
		{
			name: "valid bool map",
			input: map[string]interface{}{
				"create": true,
				"update": false,
			},
			expected: map[FieldScope]bool{
				ScopeCreate: true,
				ScopeUpdate: false,
			},
		},
		{
			name:     "empty map",
			input:    map[string]interface{}{},
			expected: map[FieldScope]bool{},
		},
		{
			name: "mixed types (should handle gracefully)",
			input: map[string]interface{}{
				"create": true,
				"update": "not a bool",
				"query":  false,
			},
			expected: map[FieldScope]bool{
				ScopeCreate: true,
				ScopeQuery:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertInterfaceMapToBoolMap(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d entries, got %d", len(tt.expected), len(result))
			}

			for scope, expected := range tt.expected {
				if result[scope] != expected {
					t.Errorf("Expected %s = %v, got %v", scope, expected, result[scope])
				}
			}
		})
	}
}

// Helper functions for testing
func convertInterfaceSliceToFieldScopes(slice []interface{}) []FieldScope {
	var scopes []FieldScope
	for _, item := range slice {
		if str, ok := item.(string); ok {
			scopes = append(scopes, FieldScope(str))
		}
	}
	return scopes
}

func convertInterfaceMapToBoolMap(m map[string]interface{}) map[FieldScope]bool {
	result := make(map[FieldScope]bool)
	for key, value := range m {
		if boolVal, ok := value.(bool); ok {
			result[FieldScope(key)] = boolVal
		}
	}
	return result
}

func convertMapToDomainField(m map[string]interface{}) *DomainField {
	field := &DomainField{}

	// Convert scopes
	if scopesInterface, ok := m["scopes"]; ok {
		if scopesSlice, ok := scopesInterface.([]interface{}); ok {
			field.Scopes = convertInterfaceSliceToFieldScopes(scopesSlice)
		}
	}

	// Convert required map
	if requiredInterface, ok := m["required"]; ok {
		if requiredMap, ok := requiredInterface.(map[string]interface{}); ok {
			field.Required = convertInterfaceMapToBoolMap(requiredMap)
		}
	}

	// Convert boolean fields
	if searchable, ok := m["searchable"].(bool); ok {
		field.Searchable = searchable
	}

	if filterable, ok := m["filterable"].(bool); ok {
		field.Filterable = filterable
	}

	if sortable, ok := m["sortable"].(bool); ok {
		field.Sortable = sortable
	}

	if sensitive, ok := m["sensitive"].(bool); ok {
		field.Sensitive = sensitive
	}

	// Convert string fields
	if description, ok := m["description"].(string); ok {
		field.Description = description
	}

	return field
}
