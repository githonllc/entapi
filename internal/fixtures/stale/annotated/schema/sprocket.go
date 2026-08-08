// Package schema is the FIRST of the two schemas behind the "stale" fixture:
// Sprocket carries entapi annotations, so generation writes DTO, base
// service and base handler files for it.
//
// The sibling package internal/fixtures/stale/plain/schema declares the same
// two entities with Sprocket's annotations removed. Generating that one over
// the same target directory is what the fixture asserts on: the files written
// for Sprocket here must be gone afterwards.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Sprocket is the entity that loses its annotations between the two runs.
type Sprocket struct {
	ent.Schema
}

// Fields of the Sprocket.
func (Sprocket) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("name"),
	}
}

func (Sprocket) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// Trinket never carries annotations in either schema, so entapi never
// generates anything for it. The fixture keeps a hand-written trinket_dto.go
// next to the generated output to prove cleanup does not touch it.
type Trinket struct {
	ent.Schema
}

// Fields of the Trinket.
func (Trinket) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),

		field.String("label"),
	}
}
