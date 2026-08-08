// Package schema holds the hand-written ent schema for the "presence" codegen
// fixture: the field shapes whose create and patch behaviour depends on being
// able to tell an absent key from an explicit null from a value (#26).
//
// Every obligation transferred onto #26 from #14 has a field here: a field with
// a schema Default(), an optional field that can be omitted, a field that can be
// cleared with an explicit null, a required NON-string field, and an Immutable()
// field. The last one is the case the generator cannot police — see
// account_presence_test.go.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi"
)

// Account is the presence fixture's single entity.
type Account struct {
	ent.Schema
}

// Fields of the Account.
func (Account) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		// ent-mandatory with no default: required, and a string, so the v1
		// emptiness check covers it. Every other required field below is the
		// shape v1 emitted no check at all for.
		field.String("email").
			Annotations(entapi.DefaultField()),

		// Required and NOT a string: #14's first fixture obligation. A zero
		// value is a legal seat count, so "required" here can only mean "the
		// caller sent the key".
		field.Int("seats").
			Annotations(entapi.DefaultField()),

		// Required and a bool: the shape with no correct zero-value check at
		// all, since false is exactly as meaningful as true.
		field.Bool("accepted_terms").
			Annotations(entapi.DefaultField()),

		// Carries a schema Default(). Omitting it on create must leave the
		// default in effect rather than write the zero value.
		field.Enum("plan").
			Values("free", "pro").
			Default("free").
			Annotations(entapi.DefaultField()),

		// Optional + Nillable: clearable in a patch with an explicit null.
		field.String("nickname").
			Optional().
			Nillable().
			Annotations(entapi.DefaultField()),

		// Optional without Nillable: also clearable, and the entity holds the
		// value type rather than a pointer.
		field.Int("quota").
			Optional().
			Annotations(entapi.DefaultField()),

		// Immutable and create-only. It reaches the create request and must not
		// reach the patch request: ent's update builders iterate MutableFields,
		// which excludes it, so there is no setter to call.
		field.String("origin").
			Immutable().
			Annotations(entapi.CreateOnlyField()),
	}
}
