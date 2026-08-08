// Package schema holds the hand-written ent schema for the "sensitive"
// codegen fixture.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi"
)

// Account proves that ent's Sensitive fact overrides response-scoped
// annotations without narrowing the request surface.
type Account struct {
	ent.Schema
}

// Fields of the Account.
func (Account) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		field.String("password_hash").
			Sensitive().
			Annotations(entapi.DefaultField()),

		field.String("name").
			Annotations(entapi.DefaultField()),

		field.JSON("login_window", []time.Time{}).
			Sensitive().
			Annotations(entapi.OutputOnlyField()),
	}
}
