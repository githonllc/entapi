package entapi

import (
	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// newField creates a gen.Field with an optional api field annotation.
func newField(name string, ti *field.TypeInfo, annotation *api.FieldAnnotation) *gen.Field {
	f := &gen.Field{
		Name: name,
		Type: ti,
	}
	if annotation != nil {
		f.Annotations = gen.Annotations{fieldAnnotationName: annotation}
	}
	return f
}

func newStringField(name string, annotation *api.FieldAnnotation) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeString, Ident: "string"}, annotation)
}

func newIntField(name string, annotation *api.FieldAnnotation) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeInt, Ident: "int"}, annotation)
}

func newInt64Field(name string, annotation *api.FieldAnnotation) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeInt64, Ident: "int64"}, annotation)
}

func newTimeField(name string, annotation *api.FieldAnnotation) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeTime, Ident: "time.Time"}, annotation)
}

// newUUIDField creates a gen.Field with UUID type.
func newUUIDField(name string, annotation *api.FieldAnnotation) *gen.Field {
	return newField(name, &field.TypeInfo{Type: field.TypeUUID, Ident: "uuid.UUID"}, annotation)
}

// newTestType creates a Resource with an int64 ID and the provided fields.
func newTestType(name string, fields ...*gen.Field) *gen.Type {
	idField := newInt64Field("id", nil)
	return &gen.Type{
		Name:        name,
		ID:          idField,
		Fields:      fields,
		Annotations: gen.Annotations{resourceAnnotationName: resourcePtr(api.Resource())},
	}
}

func fieldPtr(a api.FieldAnnotation) *api.FieldAnnotation { return &a }

func edgePtr(a api.EdgeAnnotation) *api.EdgeAnnotation { return &a }

func resourcePtr(a api.ResourceAnnotation) *api.ResourceAnnotation { return &a }
