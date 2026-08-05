// Package schema holds the hand-written ent schema for the "intid" codegen
// fixture: a domain-annotated entity whose primary key is not a UUID.
//
// Every other fixture is UUID-keyed, which is exactly how the hardcoded
// uuid.UUID in base_service.tmpl and base_handler.tmpl survived — a template
// that only ever renders one identifier type never gets to be wrong about it.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entdomain"
)

// Counter declares no id field, so ent supplies its default primary key: an
// autoincrementing int. The entity carries domain annotations, so generation
// does not skip it.
//
// The generated base service and base handler declare every identifier as
// uuid.UUID, so the emitted code cannot compile against an int-keyed entity.
// This fixture exists to make that a generation-time refusal naming the entity
// and the id type, rather than a compile error in the consumer's own package.
type Counter struct {
	ent.Schema
}

// Fields of the Counter.
func (Counter) Fields() []ent.Field {
	return []ent.Field{
		field.String("label").
			Annotations(entdomain.DefaultField().WithRequired(entdomain.ScopeCreate)),
	}
}
