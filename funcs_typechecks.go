package entdomain

import (
	"reflect"
	"strings"

	"entgo.io/ent/entc/gen"
)

// hasSoftDelete and isTimeField used to live here. They implemented the
// convention that an entity is soft-deletable iff it owns a Nillable time.Time
// field literally named "deleted_at", and drove base_service.tmpl's Delete and
// DeleteBatch to write their own tombstone.
//
// #18 retired both. Soft delete is now declared by embedding
// entdomain.SoftDeleteMixin (softdelete.go) and detected through the annotation
// the mixin attaches (funcs_softdelete.go), and the tombstone write happens in
// one place: the generated hook. The convention could not tell an entity that
// opted in from one that merely owns a column with that name, and the service
// layer could not enforce the read half at all — Base<X>Service.DB is an
// exported *Client.

// isComplexFieldType reports whether a field's Go type is one Go's comparable
// constraint rejects — slices, maps and functions, including named types whose
// underlying type is one of those.
//
// It decides which pointer helper the generated response constructor calls:
// entdomain.PtrOrNil is [T comparable] and does not compile for such a type,
// so those fields must go to entdomain.PtrNilSafe instead.
//
// The resolved type is the authority, not its rendered name. A named type
// declared as `type Tags []string` renders as "schema.Tags" and matches no
// prefix, which is exactly how it used to reach PtrOrNil and fail the
// consumer's build (#10). ent records the reflect kind of a Go type on
// Type.RType, so ask that first and fall back to the rendered name only for
// fields that carry no RType (built-in JSON descriptors declared through the
// type string alone).
func isComplexFieldType(f *gen.Field) bool {
	if f == nil || f.Type == nil {
		return false
	}
	if rt := f.Type.RType; rt != nil {
		switch rt.Kind {
		case reflect.Slice, reflect.Map, reflect.Func:
			return true
		}
	}
	name := f.Type.String()
	return strings.HasPrefix(name, "[]") ||
		strings.HasPrefix(name, "map[") ||
		strings.Contains(name, "json.")
}
