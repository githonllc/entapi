// Package schema holds the hand-written ent schema for the "requirededge"
// codegen REFUSAL fixture (#110).
//
// The shape is the one the bug was reported against: a Resource whose create
// family is reachable, owning an edge that Ent marks Required() but that
// declares no edge.Field(). Ent then demands the edge on every create — its
// generated check() returns `missing required edge` before any SQL — while
// createFields only ever ranges the entity's Fields, so no setter for it can
// reach SessionCreateRequest. Every POST would fail, and nothing in generation
// would have said so.
//
// Session's "token" field is here to keep this fixture about that one row:
// without a mutable, non-Hidden field the empty-PATCH refusal would fire too
// and the assertion would no longer pin the message under test.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// Session is the refused entity.
type Session struct {
	ent.Schema
}

// Fields of the Session.
func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("token"),
	}
}

// Edges of the Session.
//
// Unique + no inverse makes this M2O, so Session's own table holds the foreign
// key and adding Field("user_id") here would be accepted by Ent — which is why
// the refusal must offer edge.Field(...) for this shape and must not offer it
// for an edge that does not hold the key.
//
// Deliberately unannotated: api.Expand() would trip the expand-to-non-resource
// row instead, and this fixture must have exactly one refusal.
func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Unique().
			Required(),
	}
}

func (Session) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// User is the far end. It carries no annotation, so nothing is generated for it
// and it contributes no second refusal.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}
