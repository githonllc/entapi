package entdomain

import (
	"testing"
)

func TestTemplateFuncs(t *testing.T) {
	funcs := templateFuncs()

	// Test that all expected functions are present
	expectedFuncs := []string{
		"domainFields", "createFields", "patchFields", "responseFields",
		"isCreatePointer", "isCreateRequired", "isPatchClearable",
		"responseEdges", "hasSoftDelete",
		"camelCase",
	}

	for _, funcName := range expectedFuncs {
		if _, exists := funcs[funcName]; !exists {
			t.Errorf("Expected template function %s not found", funcName)
		}
	}
}

// TestGetDomainFieldAnnotationFromMap exercises the map branch of
// getDomainFieldAnnotation, the shape a DomainField arrives in when the schema
// was loaded from its serialized form rather than built in-process. It goes
// through the production decoder deliberately: a test-local reimplementation
// would pass while the real one drifted.
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

	fld := newStringField("test_field", nil)
	fld.Annotations = map[string]interface{}{"DomainField": mapAnnotation}

	annotation := getDomainFieldAnnotation(fld)

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
