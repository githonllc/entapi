// Package schema holds the hand-written ent schema for the "selfrefpartial"
// codegen fixture: a deliberately one-way self-referential pair. The one-word
// edge vocabulary cannot distinguish this intent from the chained-builder
// accident, so generation keeps refusing it under issue #79's owner review.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Node is self-referential and exposes the upward direction only.
type Node struct {
	ent.Schema
}

// Fields of the Node.
func (Node) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("label"),

		field.UUID("parent_id", uuid.UUID{}).Optional(),
	}
}

// Edges of the Node.
func (Node) Edges() []ent.Edge {
	return []ent.Edge{
		// Deliberately not expanded. This asymmetry is refused.
		edge.To("children", Node.Type),

		edge.From("parent", Node.Type).
			Ref("children").
			Unique().
			Field("parent_id").
			Annotations(api.Expand()),
	}
}

func (Node) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
