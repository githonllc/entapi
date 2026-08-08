// Package schema holds the hand-written ent schema for the "basic" codegen
// fixture. It is compiled by the module's own test binary, generated over by
// TestCodegenFixtures, and the result is compiled by `go build`.
//
// Keep this schema on the happy path. Hostile shapes (nillable/immutable/GoType
// combinations, soft delete, non-UUID ids, edges) belong to the issues that fix
// the bugs they expose, each bringing its own fixture directory.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Widget is the single happy-path fixture entity. Its fields cover the three
// scopes the generated DTOs are built from: create, update and response.
type Widget struct {
	ent.Schema
}

// Fields of the Widget.
func (Widget) Fields() []ent.Field {
	return []ent.Field{
		// UUID primary key — BaseService hardcodes uuid.UUID in its signatures.
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		// create + update + response, required on create.
		field.String("name"),

		// create + update + response, optional — exercises the pointer branches.
		field.String("description").Optional(),

		// response only — never settable from an HTTP request.
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(api.ReadOnly()),
	}
}

// Annotations opts Widget into entapi generation.
func (Widget) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
