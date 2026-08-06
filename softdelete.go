package entdomain

import (
	"context"

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
//		return []ent.Mixin{entdomain.SoftDeleteMixin{}}
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

// softDeletedKey and hardDeleteKey are the two context switches the generated
// code reads. They are separate on purpose.
//
// ent's published recipe uses one key for both, so a caller who wanted to read
// a tombstone also silently armed a real DELETE on the same context. These do
// one thing each: WithSoftDeleted changes what reads return and nothing else,
// WithHardDelete changes what a delete does and nothing else. Neither implies
// the other.
type softDeletedKey struct{}

type hardDeleteKey struct{}

// WithSoftDeleted returns a context whose reads include soft-deleted rows.
//
// It is the documented way back in, and it is per-call rather than per-client:
// the filter is off for queries issued with this context and unchanged for
// every other one. Deletes are unaffected — a delete on this context is still a
// soft delete.
func WithSoftDeleted(ctx context.Context) context.Context {
	return context.WithValue(ctx, softDeletedKey{}, true)
}

// SoftDeletedIncluded reports whether ctx carries WithSoftDeleted.
//
// It is exported because the generated traverser lives in the consumer's ent
// package and has to ask; consumers have no reason to call it.
func SoftDeletedIncluded(ctx context.Context) bool {
	skip, _ := ctx.Value(softDeletedKey{}).(bool)
	return skip
}

// WithHardDelete returns a context on which a delete removes the row instead of
// stamping it.
//
// Without it a soft-deleted row could never be purged, which turns "soft
// delete" into "storage grows forever". Reads are unaffected.
func WithHardDelete(ctx context.Context) context.Context {
	return context.WithValue(ctx, hardDeleteKey{}, true)
}

// HardDeleteRequested reports whether ctx carries WithHardDelete. Exported for
// the generated hook, like SoftDeletedIncluded.
func HardDeleteRequested(ctx context.Context) bool {
	hard, _ := ctx.Value(hardDeleteKey{}).(bool)
	return hard
}
