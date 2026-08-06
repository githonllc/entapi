package entdomain

import (
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

func TestHasDomainScope(t *testing.T) {
	tests := []struct {
		name   string
		field  *gen.Field
		scope  FieldScope
		expect bool
	}{
		{
			name:   "field with matching scope",
			field:  newStringField("name", ptr(DomainFieldWithScopes(ScopeCreate, ScopeUpdate))),
			scope:  ScopeCreate,
			expect: true,
		},
		{
			name:   "field without matching scope",
			field:  newStringField("name", ptr(DomainFieldWithScopes(ScopeCreate))),
			scope:  ScopeUpdate,
			expect: false,
		},
		{
			name:   "field without annotation",
			field:  newStringField("name", nil),
			scope:  ScopeCreate,
			expect: false,
		},
		{
			name:   "field with empty scopes",
			field:  newStringField("name", ptr(DomainField{})),
			scope:  ScopeCreate,
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasDomainScope(tt.field, tt.scope)
			if got != tt.expect {
				t.Errorf("hasDomainScope() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestIsDomainRequired(t *testing.T) {
	tests := []struct {
		name   string
		field  *gen.Field
		scope  FieldScope
		expect bool
	}{
		{
			name: "required in scope",
			field: newStringField("name", ptr(DomainField{
				Scopes:   AllFieldScopes,
				Required: map[FieldScope]bool{ScopeCreate: true},
			})),
			scope:  ScopeCreate,
			expect: true,
		},
		{
			name: "not required in scope",
			field: newStringField("name", ptr(DomainField{
				Scopes:   AllFieldScopes,
				Required: map[FieldScope]bool{ScopeCreate: true},
			})),
			scope:  ScopeUpdate,
			expect: false,
		},
		{
			name:   "nil required map",
			field:  newStringField("name", ptr(DomainFieldWithScopes(ScopeCreate))),
			scope:  ScopeCreate,
			expect: false,
		},
		{
			name:   "no annotation",
			field:  newStringField("name", nil),
			scope:  ScopeCreate,
			expect: false,
		},
		{
			name: "required explicitly false",
			field: newStringField("name", ptr(DomainField{
				Required: map[FieldScope]bool{ScopeCreate: false},
			})),
			scope:  ScopeCreate,
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDomainRequired(tt.field, tt.scope)
			if got != tt.expect {
				t.Errorf("isDomainRequired() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestGetDomainFieldAnnotation_DirectPointer(t *testing.T) {
	df := ptr(DefaultField())
	f := newStringField("name", df)

	got := getDomainFieldAnnotation(f)
	if got == nil {
		t.Fatal("expected non-nil annotation")
	}
	if !got.Searchable {
		t.Error("expected Searchable to be true")
	}
}

func TestGetDomainFieldAnnotation_MapRoundTrip(t *testing.T) {
	// Simulate the map[string]interface{} format that arrives from serialized schemas
	f := &gen.Field{
		Name: "status",
		Type: &field.TypeInfo{Type: field.TypeString, Ident: "string"},
		Annotations: gen.Annotations{
			"DomainField": map[string]interface{}{
				"scopes":        []interface{}{"create", "update", "response"},
				"required":      map[string]interface{}{"create": true},
				"searchable":    true,
				"sortable":      true,
				"filterable":    true,
				"unique_lookup": true,
				"range_lookup":  true,
			},
		},
	}

	got := getDomainFieldAnnotation(f)
	if got == nil {
		t.Fatal("expected non-nil annotation from map")
	}

	// Verify all fields roundtripped correctly
	if len(got.Scopes) != 3 {
		t.Errorf("expected 3 scopes, got %d", len(got.Scopes))
	}
	if !got.Required[ScopeCreate] {
		t.Error("expected Required[ScopeCreate] = true")
	}
	if !got.Searchable {
		t.Error("expected Searchable = true")
	}
	if !got.Sortable {
		t.Error("expected Sortable = true")
	}
	if !got.Filterable {
		t.Error("expected Filterable = true")
	}
}

func TestGetDomainFieldAnnotation_NoKey(t *testing.T) {
	f := &gen.Field{
		Name:        "name",
		Type:        &field.TypeInfo{Type: field.TypeString, Ident: "string"},
		Annotations: gen.Annotations{"OtherAnnotation": "value"},
	}

	got := getDomainFieldAnnotation(f)
	if got != nil {
		t.Errorf("expected nil for missing DomainField key, got %v", got)
	}
}

func TestGetDomainFieldAnnotation_InvalidType(t *testing.T) {
	f := &gen.Field{
		Name:        "name",
		Type:        &field.TypeInfo{Type: field.TypeString, Ident: "string"},
		Annotations: gen.Annotations{"DomainField": 42}, // not a valid type
	}

	got := getDomainFieldAnnotation(f)
	if got != nil {
		t.Errorf("expected nil for invalid annotation type, got %v", got)
	}
}

func TestGetDomainFieldAnnotation_MapUnmarshalError(t *testing.T) {
	f := &gen.Field{
		Name: "bad",
		Type: &field.TypeInfo{Type: field.TypeString, Ident: "string"},
		Annotations: gen.Annotations{
			"DomainField": map[string]interface{}{
				"scopes": 12345, // cannot unmarshal int into []FieldScope
			},
		},
	}

	got := getDomainFieldAnnotation(f)
	if got != nil {
		t.Errorf("expected nil for unmarshalable map, got %v", got)
	}
}

func TestGetDomainFieldAnnotation_NilAnnotations(t *testing.T) {
	f := &gen.Field{
		Name: "name",
		Type: &field.TypeInfo{Type: field.TypeString, Ident: "string"},
	}

	got := getDomainFieldAnnotation(f)
	if got != nil {
		t.Errorf("expected nil for nil annotations, got %v", got)
	}
}
