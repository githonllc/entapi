// Package schema holds the fixture ent schema used to verify that generated
// output actually compiles and runs. It is a separate Go module so that its
// database driver never enters the library's dependency graph.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/githonllc/entapi"
	"github.com/google/uuid"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		field.String("name").
			Annotations(entapi.DefaultField().AsFilterable().AsSortable().AsSearchable()),

		field.String("email").
			Unique().
			Annotations(entapi.DefaultField().AsFilterable().AsSearchable()),

		field.Enum("status").
			Values("active", "banned").
			Default("active").
			Annotations(entapi.DefaultField().AsFilterable()),

		field.Int("age").
			Optional().
			Nillable().
			Annotations(entapi.DefaultField().AsFilterable()),

		field.String("nickname").
			Optional().
			Nillable().
			Annotations(entapi.DefaultField()),

		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(entapi.OutputOnlyField().AsFilterable().AsSortable()),

		// Never filterable, searchable or sortable, and never in a response.
		field.String("password_hash").
			Annotations(entapi.InputOnlyField()),
	}
}

// Edges exercises the to-many case: the foreign key lives on Post, so
// edge.Field() is nil here and the FK-derived rule could never expose it.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("posts", Post.Type).
			Annotations(entapi.Edge().InResponse()),
	}
}
