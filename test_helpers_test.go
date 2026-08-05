package entdomain

import (
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

// newField creates a gen.Field with the given name, type info, and optional DomainField annotation.
func newField(name string, ti *field.TypeInfo, df *DomainField) *gen.Field {
	f := &gen.Field{
		Name: name,
		Type: ti,
	}
	if df != nil {
		f.Annotations = gen.Annotations{"DomainField": df}
	}
	return f
}

func newStringField(name string, df *DomainField) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeString, Ident: "string"}, df)
}

func newIntField(name string, df *DomainField) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeInt, Ident: "int"}, df)
}

func newInt64Field(name string, df *DomainField) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeInt64, Ident: "int64"}, df)
}

func newTimeField(name string, df *DomainField) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeTime, Ident: "time.Time"}, df)
}

func newBoolField(name string, df *DomainField) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeBool, Ident: "bool"}, df)
}

func newEnumField(name string, df *DomainField) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeEnum, Ident: "string"}, df)
}

func newInt32Field(name string, df *DomainField) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeInt32, Ident: "int32"}, df)
}

// newUUIDField creates a gen.Field with UUID type.
func newUUIDField(name string, df *DomainField) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeUUID, Ident: "uuid.UUID"}, df)
}

// newTestType creates a gen.Type with given name, an int64 ID field, and the provided fields.
func newTestType(name string, fields ...*gen.Field) *gen.Type {
	idField := newInt64Field("id", nil)
	return &gen.Type{
		Name:   name,
		ID:     idField,
		Fields: fields,
	}
}

// ptr returns a pointer to a DomainField value.
func ptr(d DomainField) *DomainField {
	return &d
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, s)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected output NOT to contain %q, got:\n%s", substr, s)
	}
}
