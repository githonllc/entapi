// Package schema holds the hand-written ent schema for the "intid" codegen
// fixture: a domain-annotated entity whose primary key is not a UUID.
//
// Every other fixture is UUID-keyed, which is exactly how the hardcoded
// uuid.UUID in base_service.tmpl and base_handler.tmpl survived for as long as
// it did — a template that only ever renders one identifier type never gets to
// be wrong about it.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi"
)

// Counter declares no id field, so ent supplies its default primary key: an
// autoincrementing int. The entity carries domain annotations, so generation
// does not skip it.
//
// Until #29 this fixture asserted a REFUSAL: the base service and base handler
// declared every identifier as uuid.UUID, so nothing correct could be emitted
// for an int-keyed entity. Both templates are gone and the remaining ones
// render the id through $.ID.Type, so the fixture now asserts the opposite —
// that generation succeeds and the output compiles. An int key is also the one
// shape that needs NO identifier import at all, which is the direction
// TestTemplatesDeclareTheirImports would otherwise never exercise.
type Counter struct {
	ent.Schema
}

// Fields of the Counter.
func (Counter) Fields() []ent.Field {
	return []ent.Field{
		field.String("label").
			Annotations(entapi.DefaultField().WithRequired(entapi.ScopeCreate)),
	}
}
