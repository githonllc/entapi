package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

type Post struct {
	ent.Schema
}

func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),

		field.String("title").
			Annotations(api.Filterable(), api.Sortable()),

		// The foreign key is exposed as a scalar. Whether the nested author
		// object is also exposed is a separate decision, made on the edge.
		field.UUID("author_id", uuid.UUID{}),
	}
}

func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("author", User.Type).
			Ref("posts").
			Unique().
			Required().
			Field("author_id").
			Annotations(api.Expand()),
	}
}

func (Post) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}
