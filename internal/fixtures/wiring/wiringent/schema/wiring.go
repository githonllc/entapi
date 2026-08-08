// Package schema holds the hand-written ent schemas for the "wiring" codegen
// fixture: the shapes the generated one-line wiring has to survive (#28).
//
// Four entities, chosen for the branches wiring generation has:
//
//	Author   a to-many response edge, so its response cannot be built from the
//	         entity a mutation builder returns — Save loads no edges.
//	Article  a to-one response edge plus the full query surface (filterable,
//	         searchable, sortable), so List exercises predicates and ordering.
//	Note     no edge at all, which is the branch where the response converter is
//	         used directly and nothing is re-fetched.
//	Patchless no writable HTTP fields, proving what an empty patch does when Ent
//	          still has an UpdateDefault field to apply.
//
// The behavioural half lives in ../e2e, a separate module so that its SQLite
// driver stays out of this library's dependency graph. See ../../README.md.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Author is the far end of a to-many response edge.
type Author struct {
	ent.Schema
}

// Fields of the Author.
func (Author) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		// Unique, so a duplicate create is a UNIQUE constraint failure rather
		// than a second row. Together with Article's required author edge —
		// whose foreign key is declared ON DELETE NO ACTION — this fixture can
		// produce BOTH kinds of constraint violation through the generated
		// wiring, which is what #13's rule has to tell apart: ent reports both
		// as *ent.ConstraintError.
		field.String("name").
			Unique().
			Annotations(api.Filterable(), api.Searchable(), api.Sortable()),
	}
}

// Edges of the Author.
func (Author) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("articles", Article.Type).
			Annotations(api.Expand()),
	}
}

func (Author) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}

// Article carries the foreign key and the full query surface.
type Article struct {
	ent.Schema
}

// Fields of the Article.
func (Article) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("title").
			Annotations(api.Filterable(), api.Searchable(), api.Sortable()),

		// Optional numeric: filterable, and the field a patch can clear.
		field.Int("rank").
			Optional().
			Nillable().
			Annotations(api.Filterable()),

		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(api.ReadOnly(), api.Filterable(), api.Sortable()),

		field.UUID("author_id", uuid.UUID{}),
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
			Annotations(api.Expand()),
	}
}

func (Article) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource().Except(api.OpDelete)}
}

// Note declares no edge, so its response is built straight from the entity a
// mutation builder returns.
type Note struct {
	ent.Schema
}

// Fields of the Note.
func (Note) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("body").
			Annotations(api.Filterable(), api.Sortable()),
	}
}

func (Note) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// Patchless has no writable HTTP patch fields. Label is immutable, and
// UpdatedAt is read-only at the HTTP layer while remaining mutable to Ent so
// UpdateDefault can run.
type Patchless struct {
	ent.Schema
}

// Fields of Patchless.
func (Patchless) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("label").Default("fixed").Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Annotations(api.ReadOnly()),
	}
}

func (Patchless) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource().Except(api.OpPatch)}
}
