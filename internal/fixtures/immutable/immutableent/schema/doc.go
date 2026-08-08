// Package schema holds the hand-written ent schema for the "immutable" codegen
// fixture: Ent Immutable fields derive out of PATCH without a second opinion.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Doc carries two immutable fields and one mutable title. Only title reaches
// the derived patch request.
type Doc struct {
	ent.Schema
}

// Fields of the Doc.
func (Doc) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("title"),

		// Immutable + required, carrying the default annotation, which includes
		// the generated patch request.
		field.String("origin").Immutable(),

		// Immutable + optional, same annotation.
		field.String("source").
			Optional().
			Immutable(),

		// Immutable but only ever an output — no conflict, must not be reported.
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(api.ReadOnly()),
	}
}

func (Doc) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
