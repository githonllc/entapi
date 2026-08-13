// Package schema holds the hand-written ent schema for the runnable pet-store
// service. Its four entities demonstrate relations, expansion, query controls,
// operation exclusion, and soft deletion in a small but complete API.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// Category groups pets in the store catalog.
type Category struct {
	ent.Schema
}

// Fields of the Category.
func (Category) Fields() []ent.Field {
	return []ent.Field{
		// Searchable feeds the free-text `_q` parameter, Filterable the
		// per-field `name` parameter, Sortable the `_sort` allow-list. Unique
		// rejects two catalog categories with the same public name.
		field.String("name").
			Unique().
			Annotations(api.Searchable(), api.Filterable(), api.Sortable()),
	}
}

// Edges of the Category.
func (Category) Edges() []ent.Edge {
	return []ent.Edge{
		// This inverse edge lets ent traverse a category to its pets; without
		// Expand it does not add pets to category response DTOs.
		edge.From("pets", Pet.Type).
			Ref("category"),
	}
}

// Annotations of the Category. api.Resource() is the single entity switch:
// without it this schema would get no EntAPI files at all.
func (Category) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}
