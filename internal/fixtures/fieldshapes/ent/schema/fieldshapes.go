// Package schema holds the hand-written ent schemas for the "fieldshapes"
// codegen fixture: the field shapes that are legal ent but that the generator
// historically turned into source that does not compile (#10).
//
// Every entity here is expected to GENERATE and COMPILE. The one shape that is
// expected to be refused at generation time — an ent-Immutable field carrying
// ScopeUpdate — lives in the sibling "immutable" fixture, because a refused
// generation and a compiled generation cannot share a directory.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entdomain"
)

// Tags is a named type whose underlying type is a slice. Reaching the generated
// response constructor through entdomain.PtrOrNil (constraint: comparable) does
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
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		// Nillable + optional: the entity field is *string.
		field.String("nickname").
			Optional().
			Nillable().
			Annotations(entdomain.DefaultField()),

		// Nillable + required on create: the create request must carry *string,
		// because the create builder's setter is SetNillableHandle(*string).
		field.String("handle").
			Optional().
			Nillable().
			Annotations(entdomain.DefaultField().WithRequired(entdomain.ScopeCreate)),

		// Same, non-string, so the required-field validator's string special
		// case is not the only thing exercised.
		field.Int("quota").
			Optional().
			Nillable().
			Annotations(entdomain.DefaultField().WithRequired(entdomain.ScopeCreate)),
	}
}

// EnumWidget covers ent enums, whose Go type is a named string type generated
// into the entity's own package.
type EnumWidget struct {
	ent.Schema
}

// Fields of the EnumWidget.
func (EnumWidget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		field.Enum("status").
			Values("draft", "live").
			Annotations(entdomain.DefaultField().WithRequired(entdomain.ScopeCreate)),

		field.Enum("tier").
			Values("free", "paid").
			Optional().
			Annotations(entdomain.DefaultField()),
	}
}

// JSONWidget covers unnamed slice and map field types, optional and required.
type JSONWidget struct {
	ent.Schema
}

// Fields of the JSONWidget.
func (JSONWidget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		field.JSON("tags", []string{}).
			Optional().
			Annotations(entdomain.DefaultField()),

		field.JSON("meta", map[string]string{}).
			Optional().
			Annotations(entdomain.DefaultField()),

		field.JSON("required_tags", []string{}).
			Annotations(entdomain.DefaultField().WithRequired(entdomain.ScopeCreate)),

		field.JSON("required_meta", map[string]string{}).
			Annotations(entdomain.DefaultField().WithRequired(entdomain.ScopeCreate)),
	}
}

// NamedTypeWidget covers named Go types over a slice and over a map — the shape
// string-prefix type detection cannot see.
type NamedTypeWidget struct {
	ent.Schema
}

// Fields of the NamedTypeWidget.
func (NamedTypeWidget) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		field.JSON("labels", Tags{}).
			Optional().
			Annotations(entdomain.DefaultField()),

		field.JSON("attrs", Attrs{}).
			Optional().
			Annotations(entdomain.DefaultField()),

		field.JSON("required_labels", Tags{}).
			Annotations(entdomain.DefaultField().WithRequired(entdomain.ScopeCreate)),

		field.JSON("required_attrs", Attrs{}).
			Annotations(entdomain.DefaultField().WithRequired(entdomain.ScopeCreate)),
	}
}
