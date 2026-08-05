// Package schema holds the hand-written ent schema for the "immutable" codegen
// fixture: fields that ent marks Immutable while carrying ScopeUpdate, which
// entdomain.DefaultField() grants by default.
//
// ent generates no update setter for an immutable field (Update/UpdateOne
// iterate MutableFields), so no template can emit compiling code for this
// combination. The fixture therefore expects generation to FAIL with a message
// naming the entity, the field and both conflicting facts — it is the harness's
// only expected-generation-failure case.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entdomain"
)

// Doc carries two immutable fields annotated for update — one required, one
// optional — so the reported error has to name both.
type Doc struct {
	ent.Schema
}

// Fields of the Doc.
func (Doc) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		field.String("title").
			Annotations(entdomain.DefaultField().WithRequired(entdomain.ScopeCreate)),

		// Immutable + required, carrying the default annotation, which includes
		// ScopeUpdate.
		field.String("origin").
			Immutable().
			Annotations(entdomain.DefaultField()),

		// Immutable + optional, same annotation.
		field.String("source").
			Optional().
			Immutable().
			Annotations(entdomain.DefaultField()),

		// Immutable but only ever an output — no conflict, must not be reported.
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(entdomain.OutputOnlyField()),
	}
}
