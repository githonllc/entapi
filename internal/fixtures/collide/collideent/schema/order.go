// Package schema holds the hand-written ent schema for the "collide" codegen
// fixture. Its Order entity makes the generated entity package name collide
// with ordinary lowercase sorting locals.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// Order is the entity whose lowercase package name exercises local shadowing.
type Order struct {
	ent.Schema
}

// Fields of the Order.
func (Order) Fields() []ent.Field {
	return []ent.Field{
		field.String("reference").
			Annotations(api.Searchable(), api.Filterable(), api.Sortable()),
		field.String("description").Optional(),
	}
}

// Annotations opts Order into entapi generation.
func (Order) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
