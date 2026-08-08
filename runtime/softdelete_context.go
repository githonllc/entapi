package entapi

import "context"

// This file is the runtime half of soft delete. The schema half —
// SoftDeleteMixin and the DomainSoftDelete annotation — stays in the root
// package, because it is embedded by a consumer's SCHEMA and therefore only
// ever loaded at generation time. These four functions are the opposite: the
// generated traverser and hook call them on every query and every delete, in
// production. Keeping them in the schema half would have made soft delete the
// one feature that drags ent's schema packages into a runtime binary.

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
