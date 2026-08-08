// Package schema defines the generated HTTP tracer-bullet fixture.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Article exercises all five generated endpoints and every query dimension.
type Article struct {
	ent.Schema
}

// Fields of Article.
func (Article) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("title").
			Unique().
			MaxLen(64).
			Annotations(api.Filterable(), api.Searchable(), api.Sortable()),
		field.Int("rank").
			Optional().
			Nillable().
			Annotations(api.Filterable(), api.Sortable()),
		field.String("slug").
			Default("generated").
			Immutable(),
		// Settable, never returned. It is what makes the generated document's
		// claim checkable in both directions: absent from every *Response and
		// *Summary schema, present in the create and patch request schemas.
		field.String("internal_note").
			Optional().
			Sensitive(),
	}
}

// Annotations of Article.
func (Article) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}

// AuditLog proves Except removes an endpoint, route, and Fn type together.
type AuditLog struct {
	ent.Schema
}

// Fields of AuditLog.
func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("message"),
	}
}

// Annotations of AuditLog.
func (AuditLog) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource().Except(api.OpDelete)}
}
