package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entdomain"
)

// Category is self-referential, which is the case where any fixed expansion
// depth is wrong and therefore the case that decides whether two-tier response
// types are sufficient rather than merely convenient.
type Category struct {
	ent.Schema
}

// Fields of the Category.
func (Category) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		field.String("name").
			Annotations(entdomain.DefaultField()),

		field.UUID("parent_id", uuid.UUID{}).
			Optional().
			Annotations(entdomain.DefaultField()),
	}
}

// Edges of the Category.
//
// The two edges are declared SEPARATELY on purpose. Written in the chained form
//
//	edge.To("children", Category.Type).From("parent").Unique().Field("parent_id").Annotations(a)
//
// the annotation attaches to the inverse edge only; the assoc edge is left
// unannotated, and before #30 nothing was reported and "children" silently
// never appeared in a response. That asymmetry is now a generation error —
// internal/fixtures/selfref is the refused shape, and internal/fixtures/selfrefpartial
// is how exposing one end only is written down on purpose.
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
