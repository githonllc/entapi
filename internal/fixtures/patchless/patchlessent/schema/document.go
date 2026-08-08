package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// Document has no mutable field and does not exclude PATCH.
type Document struct{ ent.Schema }

func (Document) Fields() []ent.Field {
	return []ent.Field{
		field.String("slug").Immutable(),
		field.String("origin").Optional().Immutable(),
	}
}

func (Document) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
