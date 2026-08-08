// Package schema holds the hand-written ent schema for the "sensitive"
// codegen fixture.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Account proves that ent's Sensitive fact overrides response-scoped
// annotations without narrowing the request surface.
type Account struct {
	ent.Schema
}

// Fields of the Account.
func (Account) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("password_hash").Sensitive(),

		field.String("name"),

		field.JSON("login_window", []time.Time{}).Sensitive(),
	}
}

func (Account) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
