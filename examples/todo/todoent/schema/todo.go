// Package schema holds the hand-written ent schema for the runnable example
// service. One entity, ent's default int id, and just enough annotations that
// every generated endpoint and every query dimension has something to act on.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// Todo is the one entity this example exposes.
type Todo struct {
	ent.Schema
}

// Fields of the Todo.
func (Todo) Fields() []ent.Field {
	return []ent.Field{
		// Searchable feeds the free-text `_q` parameter, Filterable the
		// per-field query parameters, Sortable the `_sort` allow-list.
		field.String("title").
			Annotations(api.Searchable(), api.Filterable(), api.Sortable()),

		field.Bool("done").
			Optional().
			Default(false).
			Annotations(api.Filterable()),

		field.Int("priority").
			Optional().
			Default(0).
			Annotations(api.Filterable(), api.Sortable()),

		// ReadOnly keeps it out of the create and patch requests while it stays
		// in every response; Immutable + Default is what lets ent fill it.
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			Annotations(api.Sortable(), api.ReadOnly()),
	}
}

// Annotations of the Todo. api.Resource() is the single entity switch: without
// it this schema would get no EntAPI files at all.
func (Todo) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}
