package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/githonllc/entdomain"
	"github.com/google/uuid"
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
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		field.String("name").
			Annotations(entdomain.DefaultField().AsSortable().AsFilterable()),

		field.UUID("parent_id", uuid.UUID{}).
			Optional().
			Annotations(entdomain.DefaultField()),
	}
}

func (Category) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", Category.Type).
			Annotations(entdomain.Edge().InResponse()),
		edge.From("parent", Category.Type).
			Ref("children").
			Unique().
			Field("parent_id").
			Annotations(entdomain.Edge().InResponse()),
	}
}
