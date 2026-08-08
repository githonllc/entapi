package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Category is self-referential on purpose: it is the case where any fixed
// expansion depth is wrong, and therefore the case that decides whether a
// two-tier response scheme is sufficient or merely convenient.
type Category struct {
	ent.Schema
}

func (Category) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),

		field.String("name").
			Annotations(api.Sortable(), api.Filterable()),

		field.UUID("parent_id", uuid.UUID{}).
			Optional(),
	}
}

func (Category) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", Category.Type).
			Annotations(api.Expand()),
		edge.From("parent", Category.Type).
			Ref("children").
			Unique().
			Field("parent_id").
			Annotations(api.Expand()),
	}
}

func (Category) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}
