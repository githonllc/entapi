// Package schema holds the hand-written ent schema for the "edges" codegen
// fixture.
//
// Where "basic" covers an entity with no edges at all, this fixture covers the
// four edge shapes the response half has to survive: to-one, to-many,
// self-referential, and an edge deliberately left unannotated next to a
// response-scoped foreign key.
//
// The shapes here mirror internal/fixture (SINGULAR) — the #22 spike, whose
// hand-written ent/dto/ package is the target this generator has to reproduce.
// Keep the two in step: that package is the specification, this one is the
// output.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// User is the far end of a to-many edge. The foreign key lives on Post, so
// edge.Field() is nil here — which is exactly why the old FK-derived rule could
// never expose User.posts, and why edges now carry their own annotation.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("name"),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		// To-many, annotated: appears in UserResponse as []*PostSummary.
		edge.To("posts", Post.Type).
			Annotations(api.Expand()),

		// The inverse end of Post.reviewer. Unannotated on both sides, so it
		// must not reach any response.
		edge.To("reviewed", Post.Type),
	}
}

func (User) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
