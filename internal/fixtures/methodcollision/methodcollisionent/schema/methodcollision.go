// Package schema holds the hand-written ent schemas for the "methodcollision"
// codegen fixture (#113): a schema whose FIELD name collides with a method the
// patch DTO generates onto the same receiver.
//
// It is the method-level sibling of "reservednames", which covers entity names
// against package-level declarations. Neither check subsumes the other: a
// method name lives in its receiver's namespace, so reservedNameConflicts has
// nothing to say about it, and until #113 no generated method name was derived
// from a field's Go name in a way that could clash with another one.
//
//	Gadget  has a field called "apply". The value reader generated for it is
//	        `Apply() (string, bool)` on ValidGadgetPatchRequest, where
//	        `Apply(b *GadgetUpdateOne)` already lives. This collision is NEW —
//	        it is introduced by the readers themselves.
//	Widget  has the pair "x" and "has_x". Their Go names are X and HasX, and
//	        HasX is also the presence method generated for X. This one is
//	        OLDER than the readers: #98 put Has<Field>() on the RAW request,
//	        where has_x is a struct field of the same name, so
//	        `field and method with the same name HasX` was already a compile
//	        error in the consumer's package at 59d54b6. The readers add a
//	        second occurrence on the wrapper. Either way the author gets a
//	        broken build in a file they did not write, which is what this
//	        refusal replaces.
//
// Both entities are annotated and every field is Optional, so nothing else in
// the matrix fires: the graph is required to produce exactly two problems.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Gadget is the Apply collision. Nothing about it is unusual except one field
// name, which is the entire point: the schema is legal ent and legal entapi
// field by field, and only the generated receiver's method set says no.
type Gadget struct {
	ent.Schema
}

// Fields of the Gadget.
func (Gadget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("apply").Optional(),
	}
}

func (Gadget) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// Widget is the X / HasX collision. Both fields are needed: "has_x" on its own
// generates HasHasX() beside a HasX field and compiles fine.
type Widget struct {
	ent.Schema
}

// Fields of the Widget.
func (Widget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("x").Optional(),
		field.String("has_x").Optional(),
	}
}

func (Widget) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
