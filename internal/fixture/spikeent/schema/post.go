package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/githonllc/entapi"
	"github.com/google/uuid"
)

type Post struct {
	ent.Schema
}

func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		field.String("title").
			Annotations(entapi.DefaultField().AsFilterable().AsSortable()),

		// The foreign key is exposed as a scalar. Whether the nested author
		// object is also exposed is a separate decision, made on the edge.
		field.UUID("author_id", uuid.UUID{}).
			Annotations(entapi.DefaultField()),
	}
}

func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("author", User.Type).
			Ref("posts").
			Unique().
			Required().
			Field("author_id").
			Annotations(entapi.Edge().InResponse()),
	}
}
