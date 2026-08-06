package entdomain

import (
	"testing"
)

func TestDomainFieldAnnotation(t *testing.T) {
	tests := []struct {
		name            string
		field           DomainField
		expectedName    string
		wantSearchable  bool
		wantFilterable  bool
		wantSortable    bool
		wantDescription string
	}{
		{
			name: "basic field",
			field: DomainField{
				Scopes:      []FieldScope{ScopeCreate, ScopeUpdate, ScopeResponse},
				Searchable:  true,
				Filterable:  true,
				Sortable:    true,
				Description: "Test field description",
			},
			expectedName:    "DomainField",
			wantSearchable:  true,
			wantFilterable:  true,
			wantSortable:    true,
			wantDescription: "Test field description",
		},
		{
			name: "optional field",
			field: DomainField{
				Scopes:      []FieldScope{ScopeResponse},
				Searchable:  false,
				Filterable:  false,
				Sortable:    false,
				Description: "Optional field",
			},
			expectedName:    "DomainField",
			wantSearchable:  false,
			wantFilterable:  false,
			wantSortable:    false,
			wantDescription: "Optional field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.field.Name() != tt.expectedName {
				t.Errorf("Name() = %v, want %v", tt.field.Name(), tt.expectedName)
			}

			// Test field properties
			if len(tt.field.Scopes) == 0 {
				t.Error("Field should have at least one scope")
			}

			if tt.field.Searchable != tt.wantSearchable {
				t.Errorf("Field.Searchable = %v, want %v", tt.field.Searchable, tt.wantSearchable)
			}

			if tt.field.Filterable != tt.wantFilterable {
				t.Errorf("Field.Filterable = %v, want %v", tt.field.Filterable, tt.wantFilterable)
			}

			if tt.field.Sortable != tt.wantSortable {
				t.Errorf("Field.Sortable = %v, want %v", tt.field.Sortable, tt.wantSortable)
			}

			if tt.field.Description != tt.wantDescription {
				t.Errorf("Field.Description = %v, want %v", tt.field.Description, tt.wantDescription)
			}
		})
	}
}

func TestDomainFieldBuilders(t *testing.T) {
	t.Run("NewDomainField", func(t *testing.T) {
		field := NewDomainField()

		if field.Name() != "DomainField" {
			t.Errorf("Name() = %v, want %v", field.Name(), "DomainField")
		}
	})

	t.Run("DefaultField", func(t *testing.T) {
		field := DefaultField()

		if len(field.Scopes) != len(AllFieldScopes) {
			t.Errorf("DefaultField should have all scopes, got %d, want %d", len(field.Scopes), len(AllFieldScopes))
		}

		for i, scope := range AllFieldScopes {
			if i >= len(field.Scopes) || field.Scopes[i] != scope {
				t.Errorf("Expected scope %s at index %d", scope, i)
			}
		}
	})

	t.Run("InputOnlyField", func(t *testing.T) {
		field := InputOnlyField()

		expectedScopes := []FieldScope{ScopeCreate, ScopeUpdate}
		if len(field.Scopes) != len(expectedScopes) {
			t.Errorf("InputOnlyField should have %d scopes, got %d", len(expectedScopes), len(field.Scopes))
		}

		for i, scope := range expectedScopes {
			if i >= len(field.Scopes) || field.Scopes[i] != scope {
				t.Errorf("Expected scope %s at index %d", scope, i)
			}
		}

		// Withholding ScopeResponse is the whole mechanism: there is no
		// separate marker that keeps the field out of the response.
		for _, scope := range field.Scopes {
			if scope == ScopeResponse {
				t.Error("InputOnlyField must not carry ScopeResponse")
			}
		}
	})

	t.Run("OutputOnlyField", func(t *testing.T) {
		field := OutputOnlyField()

		expectedScopes := []FieldScope{ScopeQuery, ScopeResponse}
		if len(field.Scopes) != len(expectedScopes) {
			t.Errorf("OutputOnlyField should have %d scopes, got %d", len(expectedScopes), len(field.Scopes))
		}

		for i, scope := range expectedScopes {
			if i >= len(field.Scopes) || field.Scopes[i] != scope {
				t.Errorf("Expected scope %s at index %d", scope, i)
			}
		}
	})

	t.Run("CreateOnlyField", func(t *testing.T) {
		field := CreateOnlyField()

		expectedScopes := []FieldScope{ScopeCreate, ScopeQuery, ScopeResponse}
		if len(field.Scopes) != len(expectedScopes) {
			t.Errorf("CreateOnlyField should have %d scopes, got %d", len(expectedScopes), len(field.Scopes))
		}

		for i, scope := range expectedScopes {
			if i >= len(field.Scopes) || field.Scopes[i] != scope {
				t.Errorf("Expected scope %s at index %d", scope, i)
			}
		}
	})
}

func TestDomainFieldFluentAPI(t *testing.T) {
	t.Run("WithDescription", func(t *testing.T) {
		field := NewDomainField().WithDescription("Test description")

		if field.Description != "Test description" {
			t.Errorf("Description = %v, want %v", field.Description, "Test description")
		}
	})

	t.Run("AsSearchable", func(t *testing.T) {
		field := NewDomainField().AsSearchable()

		if !field.Searchable {
			t.Error("Field should be searchable")
		}
	})

	t.Run("AsFilterable", func(t *testing.T) {
		field := NewDomainField().AsFilterable()

		if !field.Filterable {
			t.Error("Field should be filterable")
		}
	})

	t.Run("AsSortable", func(t *testing.T) {
		field := NewDomainField().AsSortable()

		if !field.Sortable {
			t.Error("Field should be sortable")
		}
	})

	t.Run("WithRequired", func(t *testing.T) {
		field := NewDomainField().WithRequired(ScopeCreate)

		if field.Required == nil {
			t.Error("Required map should be initialized")
		}

		if !field.Required[ScopeCreate] {
			t.Error("Field should be required for create scope")
		}
	})

	t.Run("WithValidation", func(t *testing.T) {
		rules := map[string]interface{}{
			"min": 1,
			"max": 100,
		}
		field := NewDomainField().WithValidation(rules)

		if field.Validation == nil {
			t.Error("Validation map should be initialized")
		}

		if field.Validation["min"] != 1 {
			t.Errorf("Validation min = %v, want %v", field.Validation["min"], 1)
		}

		if field.Validation["max"] != 100 {
			t.Errorf("Validation max = %v, want %v", field.Validation["max"], 100)
		}
	})
}

func TestDomainFieldAnnotationName(t *testing.T) {
	field := DomainField{}
	if field.Name() != "DomainField" {
		t.Errorf("Name() = %v, want %v", field.Name(), "DomainField")
	}
}

func TestDomainConfigAnnotation(t *testing.T) {
	config := DomainConfig{}

	if config.Name() != "DomainConfig" {
		t.Errorf("Name() = %v, want %v", config.Name(), "DomainConfig")
	}
}
