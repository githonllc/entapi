package entapi

// This file is Layer 2, like query.go: hand-written once, generic, and free of
// any ent import. Generated filter code supplies the entity-specific parts —
// the predicate type and ent's own predicate constructors — as type parameters
// and function values.
//
// These two helpers exist because a filter carries one parameter per operator
// per field, and ent derives up to thirteen operators for a string. Emitting
// the four-line `if v != nil { ps = append(ps, f(*v)) }` for each of them turns
// a readable artifact into a wall of identical statements, and the wall is
// where a transposed field name hides.
//
// They are generic over the predicate type P rather than living beside the
// generated code because every entity in a package would otherwise declare its
// own copy under the same name, in the same package, and collide.

// AppendIf appends f(*v) to ps when v is non-nil.
//
// The nil check is the whole semantics of a filter parameter: absent means "do
// not constrain this column", which is not the same as constraining it to the
// zero value. That distinction is why every scalar parameter is a pointer.
func AppendIf[T, P any](ps *[]P, v *T, f func(T) P) {
	if v != nil {
		*ps = append(*ps, f(*v))
	}
}

// AppendIfSlice appends f(vs...) to ps when vs is non-empty.
//
// The empty case is skipped rather than passed through, because ent's variadic
// In/NotIn predicates called with no values render as a contradiction — an
// empty IN list matches nothing — and a caller who sent no values asked for no
// constraint, not for an empty result set.
func AppendIfSlice[T, P any](ps *[]P, vs []T, f func(...T) P) {
	if len(vs) > 0 {
		*ps = append(*ps, f(vs...))
	}
}
