package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entdomain"
)

// Post carries both foreign keys, so it is where "expose the scalar" and
// "expose the nested object" are shown to be independent decisions.
type Post struct {
	ent.Schema
}

// Fields of the Post.
func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		field.String("title").
			Annotations(entdomain.DefaultField()),

		// Response-scoped scalar whose edge IS annotated.
		field.UUID("author_id", uuid.UUID{}).
			Annotations(entdomain.DefaultField()),

		// Response-scoped scalar whose edge is NOT annotated. PostResponse must
		// carry ReviewerID and must not carry Reviewer.
		field.UUID("reviewer_id", uuid.UUID{}).
			Optional().
			Annotations(entdomain.DefaultField()),
	}
}

// Edges of the Post.
func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		// To-one, annotated. Together with User.posts this is also the mutual
		// pair from QUALITY-REVIEW P1-7: both ends are response-scoped, so
		// unbounded recursion would be constructible if summaries had edges.
		edge.From("author", User.Type).
			Ref("posts").
			Unique().
			Required().
			Field("author_id").
			Annotations(entdomain.Edge().InResponse()),

		// Unannotated on purpose — see reviewer_id above.
		edge.From("reviewer", User.Type).
			Ref("reviewed").
			Unique().
			Field("reviewer_id"),
	}
}
