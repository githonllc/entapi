// Package schema holds the hand-written ent schema for the "selfrefpartial"
// codegen fixture: a self-referential pair where only ONE end is exposed, on
// purpose, and the other end says so with a bare entdomain.Edge().
//
// This is the escape hatch the #30 refusal points at. It exists as a fixture
// rather than as a unit test because the thing at risk is not the check's
// arithmetic but whether an EMPTY annotation survives the schema load at all:
// annotations reach codegen through a JSON round-trip, and a DomainEdge with no
// scopes and no JSON key marshals to {}. If ent dropped it, the refusal message
// would be recommending a fix that does not work.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entdomain"
)

// Node is self-referential and exposes the upward direction only.
type Node struct {
	ent.Schema
}

// Fields of the Node.
func (Node) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		field.String("label").
			Annotations(entdomain.DefaultField()),

		field.UUID("parent_id", uuid.UUID{}).
			Optional().
			Annotations(entdomain.DefaultField()),
	}
}

// Edges of the Node.
func (Node) Edges() []ent.Edge {
	return []ent.Edge{
		// Deliberately NOT in any response: a bare annotation grants no scope,
		// so nothing about the output changes. What it changes is that the
		// decision is written down, which is what tells it apart from the end
		// the chained declaration forgets.
		edge.To("children", Node.Type).
			Annotations(entdomain.Edge()),

		edge.From("parent", Node.Type).
			Ref("children").
			Unique().
			Field("parent_id").
			Annotations(entdomain.Edge().InResponse()),
	}
}
