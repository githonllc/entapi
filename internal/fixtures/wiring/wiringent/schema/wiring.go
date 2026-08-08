// Package schema holds the hand-written ent schemas for the "wiring" codegen
// fixture: the shapes the generated one-line wiring has to survive (#28).
//
// Three entities, chosen for the three branches wiring generation has:
//
//	Author   a to-many response edge, so its response cannot be built from the
//	         entity a mutation builder returns — Save loads no edges.
//	Article  a to-one response edge plus the full query surface (filterable,
//	         searchable, sortable), so List exercises predicates and ordering.
//	Note     no edge at all, which is the branch where the response converter is
//	         used directly and nothing is re-fetched.
//
// The behavioural half lives in ../e2e, a separate module so that its SQLite
// driver stays out of this library's dependency graph. See ../../README.md.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi"
)

// Author is the far end of a to-many response edge.
type Author struct {
	ent.Schema
}

// Fields of the Author.
func (Author) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		// Unique, so a duplicate create is a UNIQUE constraint failure rather
		// than a second row. Together with Article's required author edge —
		// whose foreign key is declared ON DELETE NO ACTION — this fixture can
		// produce BOTH kinds of constraint violation through the generated
		// wiring, which is what #13's rule has to tell apart: ent reports both
		// as *ent.ConstraintError.
		field.String("name").
			Unique().
			Annotations(entapi.DefaultField().AsFilterable().AsSearchable().AsSortable()),
	}
}

// Edges of the Author.
func (Author) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("articles", Article.Type).
			Annotations(entapi.Edge().InResponse()),
	}
}

// Article carries the foreign key and the full query surface.
type Article struct {
	ent.Schema
}

// Fields of the Article.
func (Article) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		field.String("title").
			Annotations(entapi.DefaultField().AsFilterable().AsSearchable().AsSortable()),

		// Optional numeric: filterable, and the field a patch can clear.
		field.Int("rank").
			Optional().
			Nillable().
			Annotations(entapi.DefaultField().AsFilterable()),

		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(entapi.OutputOnlyField().AsFilterable().AsSortable()),

		field.UUID("author_id", uuid.UUID{}).
			Annotations(entapi.DefaultField()),
	}
}

// Edges of the Article.
func (Article) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("author", Author.Type).
			Ref("articles").
			Unique().
			Required().
			Field("author_id").
			Annotations(entapi.Edge().InResponse()),
	}
}

// Note declares no edge, so its response is built straight from the entity a
// mutation builder returns.
type Note struct {
	ent.Schema
}

// Fields of the Note.
func (Note) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		field.String("body").
			Annotations(entapi.DefaultField().AsFilterable().AsSortable()),
	}
}
