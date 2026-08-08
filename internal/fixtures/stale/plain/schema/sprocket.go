// Package schema is the SECOND of the two schemas behind the "stale" fixture:
// byte-for-byte the entity shapes of internal/fixtures/stale/annotated/schema
// with Sprocket's entapi annotations removed.
//
// Generating this one over the target directory the annotated schema was
// generated into is the "annotations removed between runs" case: Sprocket no
// longer qualifies, and the files the previous run wrote for it must be
// removed. The committed contents of internal/fixtures/stale/staleent are the result
// of this second run, which is why running the test leaves git clean.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Sprocket is the same entity as in schema_annotated, without the annotations.
type Sprocket struct {
	ent.Schema
}

// Fields of the Sprocket.
func (Sprocket) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),

		field.String("name"),
	}
}

// Trinket is unchanged: never annotated in either schema.
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
