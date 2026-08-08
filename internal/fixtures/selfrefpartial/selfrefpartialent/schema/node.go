// Package schema holds the hand-written ent schema for the "selfrefpartial"
// codegen fixture. It proves that a deliberately one-way self-referential pair
// generates, while the counterpart "selfref" fixture proves that the truly
// bare chained form is still refused.
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
		// Deliberately not expanded; api.EdgeAnnotation{} spells that decision.
		edge.To("children", Node.Type).
			Annotations(api.EdgeAnnotation{}),

		edge.From("parent", Node.Type).
			Ref("children").
			Unique().
			Field("parent_id").
			Annotations(api.Expand()),
	}
}

func (Node) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
