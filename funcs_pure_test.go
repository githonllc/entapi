package entdomain

import (
	"reflect"
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"
)

// jsonField builds a gen.Field shaped the way ent shapes a field.JSON: an
// Ident that renders as written, plus the reflect kind of the Go type on RType.
// Named types are the point — "schema.Tags" says nothing about being a slice,
// and the RType is what has to carry the answer.
func jsonField(ident string, rtype *field.RType) *gen.Field {
	return &gen.Field{
		Name: "f",
		Type: &field.TypeInfo{Type: field.TypeJSON, Ident: ident, RType: rtype},
	}
}

func TestIsComplexFieldType(t *testing.T) {
	tests := []struct {
		name  string
		field *gen.Field
		want  bool
	}{
		// Simple types, no RType — the fallback path.
		{name: "string is simple", field: newStringField("name", nil), want: false},
		{name: "int is simple", field: newIntField("age", nil), want: false},
		{name: "time.Time is simple", field: newTimeField("created_at", nil), want: false},
		{name: "uuid.UUID is simple", field: newUUIDField("id", nil), want: false},

		// Types spelled out literally, still recognised without an RType.
		{name: "[]string is complex", field: jsonField("[]string", nil), want: true},
		{name: "map[string]any is complex", field: jsonField("map[string]any", nil), want: true},
		{name: "json.RawMessage is complex", field: jsonField("json.RawMessage", nil), want: true},

		// The #10 shapes: a named type whose spelling reveals nothing.
		{name: "named type over a slice is complex", field: jsonField("schema.Tags", &field.RType{Kind: reflect.Slice}), want: true},
		{name: "named type over a map is complex", field: jsonField("schema.Attrs", &field.RType{Kind: reflect.Map}), want: true},
		{name: "named type over a string is simple", field: jsonField("schema.Name", &field.RType{Kind: reflect.String}), want: false},
		{name: "named type over a struct is simple", field: jsonField("schema.Point", &field.RType{Kind: reflect.Struct}), want: false},

		// Degenerate inputs must not panic.
		{name: "nil field is simple", field: nil, want: false},
		{name: "field without type info is simple", field: &gen.Field{Name: "f"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isComplexFieldType(tt.field)
			if got != tt.want {
				t.Errorf("isComplexFieldType(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
