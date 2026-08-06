// Package schema holds the hand-written ent schema for the "selfref" codegen
// fixture: a self-referential edge pair declared in the CHAINED form, which is
// the authoring trap #30 makes a generation error.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entdomain"
)

// Tree is self-referential, declared the way a schema author reaches for first.
type Tree struct {
	ent.Schema
}

// Fields of the Tree.
func (Tree) Fields() []ent.Field {
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

// Edges of the Tree, in the chained form. The annotation lands on "parent"
// only; "children" is left unannotated and nothing says so.
func (Tree) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", Tree.Type).
			From("parent").
			Unique().
			Field("parent_id").
			Annotations(entdomain.Edge().InResponse()),
	}
}
