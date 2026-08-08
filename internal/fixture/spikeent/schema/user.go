// Package schema holds the fixture ent schema used to verify that generated
// output actually compiles and runs. It is a separate Go module so that its
// database driver never enters the library's dependency graph.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),

		field.String("name").
			Annotations(api.Filterable(), api.Sortable(), api.Searchable()),

		field.String("email").
			Unique().
			Annotations(api.Filterable(), api.Searchable()),

		field.Enum("status").
			Values("active", "banned").
			Default("active").
			Annotations(api.Filterable()),

		field.Int("age").
			Optional().
			Nillable().
			Annotations(api.Filterable()),

		field.String("nickname").
			Optional().
			Nillable(),

		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(api.ReadOnly(), api.Filterable(), api.Sortable()),

		// Never filterable, searchable or sortable, and never in a response.
		field.String("password_hash").
			Sensitive(),
	}
}

// Edges exercises the to-many case: the foreign key lives on Post, so
// edge.Field() is nil here and the FK-derived rule could never expose it.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("posts", Post.Type).
			Annotations(api.Expand()),
	}
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}
