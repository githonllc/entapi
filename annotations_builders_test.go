package entapi

import (
	"testing"
)

// --- Metadata builders ---

func TestWithMetadata(t *testing.T) {
	meta := FieldMetadata{
		Title:      "User Email",
		Format:     "email",
		ReadOnly:   true,
		Deprecated: false,
		Tags:       []string{"user", "contact"},
	}
	field := NewDomainField().WithMetadata(meta)

	if field.Metadata == nil {
		t.Fatal("WithMetadata() should set Metadata, got nil")
	}
	if field.Metadata.Title != "User Email" {
		t.Errorf("Metadata.Title = %q, want %q", field.Metadata.Title, "User Email")
	}
	if field.Metadata.Format != "email" {
		t.Errorf("Metadata.Format = %q, want %q", field.Metadata.Format, "email")
	}
	if !field.Metadata.ReadOnly {
		t.Error("Metadata.ReadOnly should be true")
	}
	if field.Metadata.Deprecated {
		t.Error("Metadata.Deprecated should be false")
	}
	if len(field.Metadata.Tags) != 2 {
		t.Errorf("Metadata.Tags length = %d, want 2", len(field.Metadata.Tags))
	}
}

func TestWithTitle(t *testing.T) {
	t.Run("initializes nil metadata", func(t *testing.T) {
		field := NewDomainField()
		if field.Metadata != nil {
			t.Fatal("precondition: Metadata should be nil before WithTitle")
		}
		field = field.WithTitle("Username")
		if field.Metadata == nil {
			t.Fatal("WithTitle() should initialize Metadata when nil")
		}
		if field.Metadata.Title != "Username" {
			t.Errorf("Metadata.Title = %q, want %q", field.Metadata.Title, "Username")
		}
	})

	t.Run("updates existing metadata", func(t *testing.T) {
		field := NewDomainField().WithFormat("email").WithTitle("Email Address")
		if field.Metadata.Title != "Email Address" {
			t.Errorf("Metadata.Title = %q, want %q", field.Metadata.Title, "Email Address")
		}
		// Verify existing metadata is preserved
		if field.Metadata.Format != "email" {
			t.Errorf("Metadata.Format = %q, want %q after WithTitle; existing metadata should be preserved", field.Metadata.Format, "email")
		}
	})
}

func TestWithFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		{"email format", "email"},
		{"date-time format", "date-time"},
		{"uuid format", "uuid"},
		{"uri format", "uri"},
		{"empty format", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := NewDomainField().WithFormat(tt.format)
			if field.Metadata == nil {
				t.Fatal("WithFormat() should initialize Metadata")
			}
			if field.Metadata.Format != tt.format {
				t.Errorf("Metadata.Format = %q, want %q", field.Metadata.Format, tt.format)
			}
		})
	}
}

func TestWithPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
	}{
		{"email regex", `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`},
		{"phone regex", `^\+?[1-9]\d{1,14}$`},
		{"empty pattern", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := NewDomainField().WithPattern(tt.pattern)
			if field.Metadata == nil {
				t.Fatal("WithPattern() should initialize Metadata")
			}
			if field.Metadata.Pattern != tt.pattern {
				t.Errorf("Metadata.Pattern = %q, want %q", field.Metadata.Pattern, tt.pattern)
			}
		})
	}
}

func TestWithRange(t *testing.T) {
	floatPtr := func(v float64) *float64 { return &v }

	tests := []struct {
		name    string
		min     *float64
		max     *float64
		wantMin *float64
		wantMax *float64
	}{
		{
			name:    "both min and max",
			min:     floatPtr(0),
			max:     floatPtr(100),
			wantMin: floatPtr(0),
			wantMax: floatPtr(100),
		},
		{
			name:    "only min",
			min:     floatPtr(1.5),
			max:     nil,
			wantMin: floatPtr(1.5),
			wantMax: nil,
		},
		{
			name:    "only max",
			min:     nil,
			max:     floatPtr(99.9),
			wantMin: nil,
			wantMax: floatPtr(99.9),
		},
		{
			name:    "both nil",
			min:     nil,
			max:     nil,
			wantMin: nil,
			wantMax: nil,
		},
		{
			name:    "negative values",
			min:     floatPtr(-100),
			max:     floatPtr(-1),
			wantMin: floatPtr(-100),
			wantMax: floatPtr(-1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := NewDomainField().WithRange(tt.min, tt.max)
			if field.Metadata == nil {
				t.Fatal("WithRange() should initialize Metadata")
			}
			if tt.wantMin == nil {
				if field.Metadata.Minimum != nil {
					t.Errorf("Metadata.Minimum = %v, want nil", *field.Metadata.Minimum)
				}
			} else {
				if field.Metadata.Minimum == nil {
					t.Fatalf("Metadata.Minimum = nil, want %v", *tt.wantMin)
				}
				if *field.Metadata.Minimum != *tt.wantMin {
					t.Errorf("Metadata.Minimum = %v, want %v", *field.Metadata.Minimum, *tt.wantMin)
				}
			}
			if tt.wantMax == nil {
				if field.Metadata.Maximum != nil {
					t.Errorf("Metadata.Maximum = %v, want nil", *field.Metadata.Maximum)
				}
			} else {
				if field.Metadata.Maximum == nil {
					t.Fatalf("Metadata.Maximum = nil, want %v", *tt.wantMax)
				}
				if *field.Metadata.Maximum != *tt.wantMax {
					t.Errorf("Metadata.Maximum = %v, want %v", *field.Metadata.Maximum, *tt.wantMax)
				}
			}
		})
	}
}

func TestWithLength(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	tests := []struct {
		name    string
		min     *int
		max     *int
		wantMin *int
		wantMax *int
	}{
		{
			name:    "both min and max",
			min:     intPtr(1),
			max:     intPtr(255),
			wantMin: intPtr(1),
			wantMax: intPtr(255),
		},
		{
			name:    "only min",
			min:     intPtr(3),
			max:     nil,
			wantMin: intPtr(3),
			wantMax: nil,
		},
		{
			name:    "only max",
			min:     nil,
			max:     intPtr(100),
			wantMin: nil,
			wantMax: intPtr(100),
		},
		{
			name:    "both nil",
			min:     nil,
			max:     nil,
			wantMin: nil,
			wantMax: nil,
		},
		{
			name:    "zero min",
			min:     intPtr(0),
			max:     intPtr(50),
			wantMin: intPtr(0),
			wantMax: intPtr(50),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := NewDomainField().WithLength(tt.min, tt.max)
			if field.Metadata == nil {
				t.Fatal("WithLength() should initialize Metadata")
			}
			if tt.wantMin == nil {
				if field.Metadata.MinLength != nil {
					t.Errorf("Metadata.MinLength = %v, want nil", *field.Metadata.MinLength)
				}
			} else {
				if field.Metadata.MinLength == nil {
					t.Fatalf("Metadata.MinLength = nil, want %v", *tt.wantMin)
				}
				if *field.Metadata.MinLength != *tt.wantMin {
					t.Errorf("Metadata.MinLength = %v, want %v", *field.Metadata.MinLength, *tt.wantMin)
				}
			}
			if tt.wantMax == nil {
				if field.Metadata.MaxLength != nil {
					t.Errorf("Metadata.MaxLength = %v, want nil", *field.Metadata.MaxLength)
				}
			} else {
				if field.Metadata.MaxLength == nil {
					t.Fatalf("Metadata.MaxLength = nil, want %v", *tt.wantMax)
				}
				if *field.Metadata.MaxLength != *tt.wantMax {
					t.Errorf("Metadata.MaxLength = %v, want %v", *field.Metadata.MaxLength, *tt.wantMax)
				}
			}
		})
	}
}

func TestWithEnum(t *testing.T) {
	t.Run("string values", func(t *testing.T) {
		field := NewDomainField().WithEnum("active", "inactive", "pending")
		if field.Metadata == nil {
			t.Fatal("WithEnum() should initialize Metadata")
		}
		if len(field.Metadata.Enum) != 3 {
			t.Fatalf("Metadata.Enum length = %d, want 3", len(field.Metadata.Enum))
		}
		expected := []interface{}{"active", "inactive", "pending"}
		for i, v := range expected {
			if field.Metadata.Enum[i] != v {
				t.Errorf("Metadata.Enum[%d] = %v, want %v", i, field.Metadata.Enum[i], v)
			}
		}
	})

	t.Run("integer values", func(t *testing.T) {
		field := NewDomainField().WithEnum(1, 2, 3)
		if len(field.Metadata.Enum) != 3 {
			t.Fatalf("Metadata.Enum length = %d, want 3", len(field.Metadata.Enum))
		}
		for i, v := range []interface{}{1, 2, 3} {
			if field.Metadata.Enum[i] != v {
				t.Errorf("Metadata.Enum[%d] = %v, want %v", i, field.Metadata.Enum[i], v)
			}
		}
	})

	t.Run("mixed types", func(t *testing.T) {
		field := NewDomainField().WithEnum("a", 1, true, 3.14)
		if len(field.Metadata.Enum) != 4 {
			t.Fatalf("Metadata.Enum length = %d, want 4", len(field.Metadata.Enum))
		}
	})

	t.Run("single value", func(t *testing.T) {
		field := NewDomainField().WithEnum("only")
		if len(field.Metadata.Enum) != 1 {
			t.Fatalf("Metadata.Enum length = %d, want 1", len(field.Metadata.Enum))
		}
		if field.Metadata.Enum[0] != "only" {
			t.Errorf("Metadata.Enum[0] = %v, want %q", field.Metadata.Enum[0], "only")
		}
	})

	t.Run("initializes nil metadata", func(t *testing.T) {
		field := NewDomainField()
		if field.Metadata != nil {
			t.Fatal("precondition: Metadata should be nil")
		}
		field = field.WithEnum("x")
		if field.Metadata == nil {
			t.Fatal("WithEnum() should initialize Metadata when nil")
		}
	})
}

func TestAsReadOnly(t *testing.T) {
	t.Run("sets ReadOnly to true", func(t *testing.T) {
		field := NewDomainField().AsReadOnly()
		if field.Metadata == nil {
			t.Fatal("AsReadOnly() should initialize Metadata")
		}
		if !field.Metadata.ReadOnly {
			t.Error("Metadata.ReadOnly should be true")
		}
	})

	t.Run("initializes nil metadata", func(t *testing.T) {
		field := NewDomainField()
		if field.Metadata != nil {
			t.Fatal("precondition: Metadata should be nil")
		}
		field = field.AsReadOnly()
		if field.Metadata == nil {
			t.Fatal("AsReadOnly() should initialize Metadata when nil")
		}
	})

	t.Run("preserves existing metadata", func(t *testing.T) {
		field := NewDomainField().WithTitle("Test").AsReadOnly()
		if field.Metadata.Title != "Test" {
			t.Errorf("Metadata.Title = %q, want %q after AsReadOnly; existing metadata should be preserved", field.Metadata.Title, "Test")
		}
		if !field.Metadata.ReadOnly {
			t.Error("Metadata.ReadOnly should be true")
		}
	})
}

func TestAsWriteOnly(t *testing.T) {
	t.Run("sets WriteOnly to true", func(t *testing.T) {
		field := NewDomainField().AsWriteOnly()
		if field.Metadata == nil {
			t.Fatal("AsWriteOnly() should initialize Metadata")
		}
		if !field.Metadata.WriteOnly {
			t.Error("Metadata.WriteOnly should be true")
		}
	})

	t.Run("initializes nil metadata", func(t *testing.T) {
		field := NewDomainField()
		if field.Metadata != nil {
			t.Fatal("precondition: Metadata should be nil")
		}
		field = field.AsWriteOnly()
		if field.Metadata == nil {
			t.Fatal("AsWriteOnly() should initialize Metadata when nil")
		}
	})

	t.Run("preserves existing metadata", func(t *testing.T) {
		field := NewDomainField().WithFormat("password").AsWriteOnly()
		if field.Metadata.Format != "password" {
			t.Errorf("Metadata.Format = %q, want %q after AsWriteOnly", field.Metadata.Format, "password")
		}
		if !field.Metadata.WriteOnly {
			t.Error("Metadata.WriteOnly should be true")
		}
	})
}

func TestAsDeprecated(t *testing.T) {
	t.Run("sets Deprecated to true", func(t *testing.T) {
		field := NewDomainField().AsDeprecated()
		if field.Metadata == nil {
			t.Fatal("AsDeprecated() should initialize Metadata")
		}
		if !field.Metadata.Deprecated {
			t.Error("Metadata.Deprecated should be true")
		}
	})

	t.Run("initializes nil metadata", func(t *testing.T) {
		field := NewDomainField()
		if field.Metadata != nil {
			t.Fatal("precondition: Metadata should be nil")
		}
		field = field.AsDeprecated()
		if field.Metadata == nil {
			t.Fatal("AsDeprecated() should initialize Metadata when nil")
		}
	})

	t.Run("preserves existing metadata", func(t *testing.T) {
		field := NewDomainField().WithTitle("OldField").AsDeprecated()
		if field.Metadata.Title != "OldField" {
			t.Errorf("Metadata.Title = %q, want %q after AsDeprecated", field.Metadata.Title, "OldField")
		}
		if !field.Metadata.Deprecated {
			t.Error("Metadata.Deprecated should be true")
		}
	})
}

func TestWithTags(t *testing.T) {
	t.Run("multiple tags", func(t *testing.T) {
		field := NewDomainField().WithTags("user", "profile", "public")
		if field.Metadata == nil {
			t.Fatal("WithTags() should initialize Metadata")
		}
		if len(field.Metadata.Tags) != 3 {
			t.Fatalf("Metadata.Tags length = %d, want 3", len(field.Metadata.Tags))
		}
		expected := []string{"user", "profile", "public"}
		for i, tag := range expected {
			if field.Metadata.Tags[i] != tag {
				t.Errorf("Metadata.Tags[%d] = %q, want %q", i, field.Metadata.Tags[i], tag)
			}
		}
	})

	t.Run("single tag", func(t *testing.T) {
		field := NewDomainField().WithTags("admin")
		if len(field.Metadata.Tags) != 1 {
			t.Fatalf("Metadata.Tags length = %d, want 1", len(field.Metadata.Tags))
		}
		if field.Metadata.Tags[0] != "admin" {
			t.Errorf("Metadata.Tags[0] = %q, want %q", field.Metadata.Tags[0], "admin")
		}
	})

	t.Run("initializes nil metadata", func(t *testing.T) {
		field := NewDomainField()
		if field.Metadata != nil {
			t.Fatal("precondition: Metadata should be nil")
		}
		field = field.WithTags("tag1")
		if field.Metadata == nil {
			t.Fatal("WithTags() should initialize Metadata when nil")
		}
	})

	t.Run("preserves existing metadata", func(t *testing.T) {
		field := NewDomainField().WithFormat("email").WithTags("contact")
		if field.Metadata.Format != "email" {
			t.Errorf("Metadata.Format = %q, want %q after WithTags", field.Metadata.Format, "email")
		}
	})
}

// --- IdField() builder ---

func TestIdField(t *testing.T) {
	field := IdField()

	// IdField is built on OutputOnlyField, so it should have Query and Response scopes
	expectedScopes := []FieldScope{ScopeQuery, ScopeResponse}
	if len(field.Scopes) != len(expectedScopes) {
		t.Fatalf("IdField should have %d scopes, got %d", len(expectedScopes), len(field.Scopes))
	}
	for i, scope := range expectedScopes {
		if i >= len(field.Scopes) || field.Scopes[i] != scope {
			t.Errorf("Expected scope %s at index %d, got %s", scope, i, field.Scopes[i])
		}
	}

	// Description and ReadOnly, both via Metadata. WithDescription writes
	// through ensureMetadata since #17, and AsReadOnly runs after it in
	// IdField's chain, so this also pins that the second builder preserves the
	// first one's write rather than starting a fresh block.
	if field.Metadata == nil {
		t.Fatal("IdField should have Metadata set")
	}
	if field.Metadata.Description != "Unique entity identifier" {
		t.Errorf("IdField Metadata.Description = %q, want %q", field.Metadata.Description, "Unique entity identifier")
	}
	if !field.Metadata.ReadOnly {
		t.Error("IdField Metadata.ReadOnly should be true")
	}

	// No preset grants a query marker (#27). An identifier that is filterable
	// or sortable is a decision the schema author makes explicitly, because the
	// sort allow-list is only worth having if it is narrower than "everything".
	if field.Searchable || field.Filterable || field.Sortable {
		t.Errorf("IdField grants a query marker (searchable=%v filterable=%v sortable=%v); the three markers are opt-in",
			field.Searchable, field.Filterable, field.Sortable)
	}
}

// --- AuditLogField() builder ---

func TestAuditLogField(t *testing.T) {
	field := AuditLogField()

	// AuditLogField is built on OutputOnlyField, so it should have Query and Response scopes
	expectedScopes := []FieldScope{ScopeQuery, ScopeResponse}
	if len(field.Scopes) != len(expectedScopes) {
		t.Fatalf("AuditLogField should have %d scopes, got %d", len(expectedScopes), len(field.Scopes))
	}
	for i, scope := range expectedScopes {
		if i >= len(field.Scopes) || field.Scopes[i] != scope {
			t.Errorf("Expected scope %s at index %d, got %s", scope, i, field.Scopes[i])
		}
	}

	// ReadOnly via Metadata
	if field.Metadata == nil {
		t.Fatal("AuditLogField should have Metadata set")
	}
	if !field.Metadata.ReadOnly {
		t.Error("AuditLogField Metadata.ReadOnly should be true")
	}

	// No preset grants a query marker (#27).
	if field.Searchable || field.Filterable || field.Sortable {
		t.Errorf("AuditLogField grants a query marker (searchable=%v filterable=%v sortable=%v); the three markers are opt-in",
			field.Searchable, field.Filterable, field.Sortable)
	}

	// AuditLogField should NOT have a description (unlike IdField)
	if field.Metadata.Description != "" {
		t.Errorf("AuditLogField Metadata.Description = %q, want empty string", field.Metadata.Description)
	}
}

// --- DomainFieldWithScopes() ---

func TestDomainFieldWithScopes(t *testing.T) {
	tests := []struct {
		name           string
		scopes         []FieldScope
		expectedLength int
	}{
		{
			name:           "single scope: create",
			scopes:         []FieldScope{ScopeCreate},
			expectedLength: 1,
		},
		{
			name:           "two scopes: create and response",
			scopes:         []FieldScope{ScopeCreate, ScopeResponse},
			expectedLength: 2,
		},
		{
			name:           "three scopes: create, update, response",
			scopes:         []FieldScope{ScopeCreate, ScopeUpdate, ScopeResponse},
			expectedLength: 3,
		},
		{
			name:           "all scopes",
			scopes:         []FieldScope{ScopeCreate, ScopeUpdate, ScopeQuery, ScopeResponse},
			expectedLength: 4,
		},
		{
			name:           "no scopes",
			scopes:         []FieldScope{},
			expectedLength: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := DomainFieldWithScopes(tt.scopes...)
			if len(field.Scopes) != tt.expectedLength {
				t.Errorf("DomainFieldWithScopes() scopes length = %d, want %d", len(field.Scopes), tt.expectedLength)
			}
			for i, scope := range tt.scopes {
				if field.Scopes[i] != scope {
					t.Errorf("Scopes[%d] = %q, want %q", i, field.Scopes[i], scope)
				}
			}
			// Should not have any other properties set
			if field.Searchable {
				t.Error("DomainFieldWithScopes should not set Searchable")
			}
			if field.Filterable {
				t.Error("DomainFieldWithScopes should not set Filterable")
			}
			if field.Sortable {
				t.Error("DomainFieldWithScopes should not set Sortable")
			}
			if field.Metadata != nil {
				t.Error("DomainFieldWithScopes should not set Metadata")
			}
		})
	}
}

// DomainConfig was removed entirely on #17; TestDomainConfigIsNotPublished in
// annotation_placement_test.go is what keeps it from coming back.

// --- Complex builder chaining ---

func TestComplexBuilderChaining(t *testing.T) {
	t.Run("DefaultField with full chain", func(t *testing.T) {
		field := DefaultField().
			WithRequired(ScopeCreate).
			WithFormat("email").
			WithTitle("Email")

		// From DefaultField
		if len(field.Scopes) != len(AllFieldScopes) {
			t.Errorf("Should have all scopes, got %d", len(field.Scopes))
		}
		if field.Searchable || field.Filterable || field.Sortable {
			t.Errorf("DefaultField grants a query marker (searchable=%v filterable=%v sortable=%v); the three markers are opt-in (#27)",
				field.Searchable, field.Filterable, field.Sortable)
		}

		// From WithRequired
		if field.Required == nil || !field.Required[ScopeCreate] {
			t.Error("Should be required for ScopeCreate")
		}

		// From WithFormat and WithTitle (metadata)
		if field.Metadata == nil {
			t.Fatal("Should have Metadata set")
		}
		if field.Metadata.Format != "email" {
			t.Errorf("Metadata.Format = %q, want %q", field.Metadata.Format, "email")
		}
		if field.Metadata.Title != "Email" {
			t.Errorf("Metadata.Title = %q, want %q", field.Metadata.Title, "Email")
		}
	})

	t.Run("InputOnlyField with metadata chain", func(t *testing.T) {
		field := InputOnlyField().
			WithRequired(ScopeCreate).
			WithRequired(ScopeUpdate).
			AsWriteOnly().
			WithFormat("password").
			WithDescription("User password").
			WithLength(intPtr(8), intPtr(128))

		// From InputOnlyField
		expectedScopes := []FieldScope{ScopeCreate, ScopeUpdate}
		if len(field.Scopes) != len(expectedScopes) {
			t.Errorf("Should have %d scopes, got %d", len(expectedScopes), len(field.Scopes))
		}

		// Multiple WithRequired
		if !field.Required[ScopeCreate] {
			t.Error("Should be required for ScopeCreate")
		}
		if !field.Required[ScopeUpdate] {
			t.Error("Should be required for ScopeUpdate")
		}

		// Metadata
		if field.Metadata == nil {
			t.Fatal("Should have Metadata set")
		}
		if !field.Metadata.WriteOnly {
			t.Error("Metadata.WriteOnly should be true")
		}
		if field.Metadata.Format != "password" {
			t.Errorf("Metadata.Format = %q, want %q", field.Metadata.Format, "password")
		}

		// Description
		if field.Metadata.Description != "User password" {
			t.Errorf("Metadata.Description = %q, want %q", field.Metadata.Description, "User password")
		}

		// Length constraints
		if field.Metadata.MinLength == nil || *field.Metadata.MinLength != 8 {
			t.Errorf("Metadata.MinLength = %v, want 8", field.Metadata.MinLength)
		}
		if field.Metadata.MaxLength == nil || *field.Metadata.MaxLength != 128 {
			t.Errorf("Metadata.MaxLength = %v, want 128", field.Metadata.MaxLength)
		}
	})

	t.Run("OutputOnlyField with range and deprecation", func(t *testing.T) {
		field := OutputOnlyField().
			AsDeprecated().
			WithRange(floatPtr(0), floatPtr(1000)).
			WithDescription("Legacy score field").
			WithTags("legacy", "score")

		// From OutputOnlyField
		expectedScopes := []FieldScope{ScopeQuery, ScopeResponse}
		if len(field.Scopes) != len(expectedScopes) {
			t.Errorf("Should have %d scopes, got %d", len(expectedScopes), len(field.Scopes))
		}

		if field.Metadata == nil {
			t.Fatal("Should have Metadata set")
		}
		if !field.Metadata.Deprecated {
			t.Error("Should be Deprecated")
		}
		if field.Metadata.Minimum == nil || *field.Metadata.Minimum != 0 {
			t.Errorf("Metadata.Minimum = %v, want 0", field.Metadata.Minimum)
		}
		if field.Metadata.Maximum == nil || *field.Metadata.Maximum != 1000 {
			t.Errorf("Metadata.Maximum = %v, want 1000", field.Metadata.Maximum)
		}
		if field.Metadata.Description != "Legacy score field" {
			t.Errorf("Metadata.Description = %q, want %q", field.Metadata.Description, "Legacy score field")
		}
		if len(field.Metadata.Tags) != 2 {
			t.Fatalf("Tags length = %d, want 2", len(field.Metadata.Tags))
		}
		if field.Metadata.Tags[0] != "legacy" || field.Metadata.Tags[1] != "score" {
			t.Errorf("Tags = %v, want [legacy score]", field.Metadata.Tags)
		}
	})

	t.Run("CreateOnlyField with enum and pattern", func(t *testing.T) {
		field := CreateOnlyField().
			WithRequired(ScopeCreate).
			WithEnum("draft", "published", "archived").
			WithPattern(`^(draft|published|archived)$`).
			WithExample("draft").
			WithDescription("Content status")

		// From CreateOnlyField
		expectedScopes := []FieldScope{ScopeCreate, ScopeQuery, ScopeResponse}
		if len(field.Scopes) != len(expectedScopes) {
			t.Errorf("Should have %d scopes, got %d", len(expectedScopes), len(field.Scopes))
		}

		if !field.Required[ScopeCreate] {
			t.Error("Should be required for ScopeCreate")
		}

		if field.Metadata == nil {
			t.Fatal("Should have Metadata set")
		}
		if len(field.Metadata.Enum) != 3 {
			t.Fatalf("Enum length = %d, want 3", len(field.Metadata.Enum))
		}
		if field.Metadata.Pattern != `^(draft|published|archived)$` {
			t.Errorf("Pattern = %q, want %q", field.Metadata.Pattern, `^(draft|published|archived)$`)
		}
		if field.Metadata.Example != "draft" {
			t.Errorf("Metadata.Example = %v, want %q", field.Metadata.Example, "draft")
		}
		if field.Metadata.Description != "Content status" {
			t.Errorf("Metadata.Description = %q, want %q", field.Metadata.Description, "Content status")
		}
	})

	t.Run("DomainFieldWithScopes with full metadata chain", func(t *testing.T) {
		field := DomainFieldWithScopes(ScopeCreate, ScopeResponse).
			AsSearchable().
			AsFilterable().
			WithMetadata(FieldMetadata{
				Title:    "Custom Field",
				Format:   "custom",
				ReadOnly: false,
				Tags:     []string{"custom"},
			})

		if len(field.Scopes) != 2 {
			t.Errorf("Should have 2 scopes, got %d", len(field.Scopes))
		}
		if !field.Searchable {
			t.Error("Should be Searchable")
		}
		if !field.Filterable {
			t.Error("Should be Filterable")
		}
		if field.Metadata == nil {
			t.Fatal("Should have Metadata set")
		}
		if field.Metadata.Title != "Custom Field" {
			t.Errorf("Metadata.Title = %q, want %q", field.Metadata.Title, "Custom Field")
		}
		if field.Metadata.Format != "custom" {
			t.Errorf("Metadata.Format = %q, want %q", field.Metadata.Format, "custom")
		}
	})
}

// --- WithExample builder ---

func TestWithExample(t *testing.T) {
	tests := []struct {
		name    string
		example interface{}
	}{
		{"string example", "john@example.com"},
		{"int example", 42},
		{"float example", 3.14},
		{"bool example", true},
		{"nil example", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := NewDomainField().WithExample(tt.example)
			if field.Metadata == nil {
				t.Fatal("WithExample should have allocated a Metadata block")
			}
			if field.Metadata.Example != tt.example {
				t.Errorf("Metadata.Example = %v, want %v", field.Metadata.Example, tt.example)
			}
		})
	}
}

// --- Helper functions for pointer creation in tests ---

func floatPtr(v float64) *float64 { return &v }
func intPtr(v int) *int           { return &v }
