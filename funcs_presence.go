package entapi

import (
	"entgo.io/ent/entc/gen"
)

// The request half of the DTO layer is three predicates over one field, and all
// three read ent's own schema rather than a parallel set of entapi booleans
// (#26). That is not a style preference: ent decides which setters exist, so any
// second opinion this package forms about a field's shape shows up as a call to
// a method that was never generated.
//
//   - isCreatePointer decides *T versus T in the create request.
//   - isCreateRequired decides whether Validate demands a value.
//   - isPatchClearable decides whether an explicit null is a legal way to say
//     "clear this".

// isCreatePointer reports whether the create request carries this field as *T.
//
// A pointer is required exactly when ent can fill the field without the caller:
// an Optional field may be left unset, and a field with a Default() must be
// leavable unset or the default can never apply. Nillable is included because
// #10's non-compiling shape was a required Nillable field emitted as a value
// type: the entity holds *T there, and passing T where the generated code
// expects a pointer does not build.
//
// The value the pointer distinguishes is "the caller said nothing", which is the
// only state a create request needs — it has no way to express "clear", so an
// explicit null means the same thing as an omitted key.
func isCreatePointer(f *gen.Field) bool {
	if f == nil {
		return false
	}
	return f.Optional || f.Default || f.Nillable
}

// isCreateRequired reports whether Validate must reject a create request that
// supplies no value for this field.
//
// The schema is the first authority: a field ent marks mandatory and cannot
// default has to be supplied, or the insert fails inside ent with an error that
// names a column rather than a request field. The annotation can only add to
// that — WithRequired(ScopeCreate) makes the HTTP layer stricter than the
// database, which is a legitimate thing to ask for and the reason a Nillable
// field can be both a pointer and required.
//
// It cannot subtract: there is no annotation that makes an ent-mandatory field
// optional in the request, because nothing would then produce the value.
func isCreateRequired(f *gen.Field) bool {
	if f == nil {
		return false
	}
	if isDomainRequired(f, ScopeCreate) {
		return true
	}
	return !f.Optional && !f.Default
}

// isPatchClearable reports whether an explicit null on this field clears it.
//
// ent generates Clear<Field>() on the update builders for Optional fields and
// for no others, so Optional is a hard precondition rather than a policy: a
// non-optional field has no clear method to call and the column is NOT NULL
// underneath it. Validate rejects the null instead, naming the field.
//
// WithRequired(ScopeUpdate) subtracts from the set. ent would accept the clear,
// but the annotation says this field must always hold a value, and that is
// exactly the promise a required marker makes at the HTTP layer. Absent still
// means "leave it alone" for such a field; only null is refused.
func isPatchClearable(f *gen.Field) bool {
	if f == nil {
		return false
	}
	return f.Optional && !isDomainRequired(f, ScopeUpdate)
}
