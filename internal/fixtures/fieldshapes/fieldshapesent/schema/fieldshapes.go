// Package schema holds the hand-written ent schemas for the "fieldshapes"
// codegen fixture: the field shapes that are legal ent but that the generator
// historically turned into source that does not compile (#10).
//
// Every entity here is expected to GENERATE and COMPILE. The one shape that is
// used to be refused — an Ent Immutable field — now derives out of PATCH and
// lives in the positive sibling "immutable" fixture.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Tags is a named type whose underlying type is a slice. Reaching the generated
// response constructor through entapi.PtrOrNil (constraint: comparable) does
// not compile, so complex-type detection has to see through the name.
type Tags []string

// Attrs is a named type whose underlying type is a map — same story as Tags.
type Attrs map[string]string

// NillableWidget covers ent's Nillable modifier, in both the plain-optional and
// the required-on-create forms.
type NillableWidget struct {
	ent.Schema
}

// Fields of the NillableWidget.
func (NillableWidget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		// Nillable + optional: the entity field is *string.
		field.String("nickname").
			Optional().
			Nillable(),

		// Nillable + optional: the create request carries *string because the
		// create builder's setter is SetNillableHandle(*string).
		field.String("handle").
			Optional().
			Nillable(),

		// Same, non-string, so the required-field validator's string special
		// case is not the only thing exercised.
		field.Int("quota").
			Optional().
			Nillable(),
	}
}

func (NillableWidget) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}

// EnumWidget covers ent enums, whose Go type is a named string type generated
// into the entity's own package.
type EnumWidget struct {
	ent.Schema
}

// Fields of the EnumWidget.
func (EnumWidget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.Enum("status").
			Values("draft", "live"),

		field.Enum("tier").
			Values("free", "paid").
			Optional(),
	}
}

func (EnumWidget) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// JSONWidget covers unnamed slice and map field types, optional and required.
type JSONWidget struct {
	ent.Schema
}

// Fields of the JSONWidget.
func (JSONWidget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.JSON("tags", []string{}).
			Optional(),

		field.JSON("meta", map[string]string{}).
			Optional(),

		field.JSON("required_tags", []string{}),

		field.JSON("required_meta", map[string]string{}),
	}
}

func (JSONWidget) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// NamedTypeWidget covers named Go types over a slice and over a map — the shape
// string-prefix type detection cannot see.
type NamedTypeWidget struct {
	ent.Schema
}

// Fields of the NamedTypeWidget.
func (NamedTypeWidget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.JSON("labels", Tags{}).
			Optional(),

		field.JSON("attrs", Attrs{}).
			Optional(),

		field.JSON("required_labels", Tags{}),

		field.JSON("required_attrs", Attrs{}),
	}
}

func (NamedTypeWidget) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}
