// Package schema holds the hand-written ent schema for the "strictquery"
// codegen fixture. Its two query fields pin both the benefit and the cost of
// opting into strict operator-prefix validation.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// Record is the resource used to exercise strict query parsing.
type Record struct {
	ent.Schema
}

// Fields of the Record.
func (Record) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Annotations(api.Filterable()),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(api.ReadOnly(), api.Filterable()),
	}
}

// Annotations of the Record.
func (Record) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}
