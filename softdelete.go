package entapi

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// SoftDeleteField is the column the mixin adds. It is a constant so the
// generator never has to spell it: DomainSoftDelete carries it through the
// schema, and the traverser template resolves it back to a *gen.Field.
//
// It is NOT a convention. Before #18 an entity was soft-deletable iff it
// happened to own a Nillable time.Time literally named "deleted_at"
// (funcs_typechecks.go, deleted in the same change), which conflated "has a
// column with this name" with "asked for row-level filtering" — two facts a
// generator has no business merging. What makes an entity soft-deletable now is
// embedding SoftDeleteMixin, and nothing else. An entity that declares its own
// deleted_at and no mixin is an ordinary hard-delete entity, whatever its
// modifiers.
const SoftDeleteField = "deleted_at"

// SoftDeleteAnnotationName is the key DomainSoftDelete occupies in a type's
// annotation map.
const SoftDeleteAnnotationName = "DomainSoftDelete"

// DomainSoftDelete marks an entity as soft-deletable and records the column
// carrying the tombstone.
//
// Schema authors do not write it. SoftDeleteMixin attaches it, and ent merges a
// mixin's annotations into the schema's own (entc/load/schema.go:314), so
// embedding the mixin is what puts it on the type.
type DomainSoftDelete struct {
	// Field is the ent field name of the tombstone column.
	Field string `json:"field,omitempty"`
}

// Name implements schema.Annotation.
func (DomainSoftDelete) Name() string { return SoftDeleteAnnotationName }

// SoftDeleteMixin adds a soft-delete tombstone column to a schema and marks the
// entity for the generated traverser.
//
//	func (Doc) Mixin() []ent.Mixin {
//		return []ent.Mixin{entapi.SoftDeleteMixin{}}
//	}
//
// It carries the field and the marker, and deliberately carries NO hooks and no
// interceptors. Two reasons, and both are load-bearing:
//
//  1. A mixin hook cannot rewrite a delete into an update from here. ent's own
//     soft-delete recipe re-dispatches through mutation.Client(), whose
//     signature is `Client() *ent.Client` — a type in the consumer's generated
//     package that this library cannot name, and therefore cannot reach through
//     an anonymous interface. The alternative is reflection on a method name,
//     which trades a compile error for a runtime one.
//  2. A schema that carries hooks or a policy switches ent's runtime generation
//     to the "empty-import" format (entc/gen/template/runtime.tmpl:12-17,50-63),
//     which obliges every consumer to add `_ "<project>/ent/runtime"` to their
//     main package. Carrying no hooks here means adopting soft delete does not
//     change how the consumer's project is generated. See README.
//
// Both halves are installed instead by one generated line at client
// construction — `ent.RegisterSoftDelete(client)`. That is a real cost, stated
// rather than hidden: a client built without it filters nothing, and a delete
// on it is an ordinary hard delete.
type SoftDeleteMixin struct {
	mixin.Schema
}

// Fields of the SoftDeleteMixin.
//
// Optional is what makes ent generate the <Entity>DeletedAtIsNil predicate the
// traverser is built from; Nillable is what lets a loaded entity report "not
// deleted" as nil rather than as the zero time.
func (SoftDeleteMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time(SoftDeleteField).
			Optional().
			Nillable(),
	}
}

// Annotations of the SoftDeleteMixin.
func (SoftDeleteMixin) Annotations() []schema.Annotation {
	return []schema.Annotation{
		DomainSoftDelete{Field: SoftDeleteField},
	}
}

var _ ent.Mixin = (*SoftDeleteMixin)(nil)
