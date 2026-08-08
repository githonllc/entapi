package entapi

// This file is Layer 2: generic predicate assembly with no ent dependency.
// Generated code supplies both the values and entity-specific constructors.

// AppendEach appends one predicate for every occurrence of a scalar filter.
func AppendEach[T, P any](ps *[]P, values []T, f func(T) P) {
	for _, value := range values {
		*ps = append(*ps, f(value))
	}
}

// AppendEachSlice appends one variadic predicate for every occurrence of an
// in/not_in filter. Empty occurrences are retained: an explicit empty IN list
// is a real contradiction, not an absent parameter.
func AppendEachSlice[T, P any](ps *[]P, values [][]T, f func(...T) P) {
	for _, value := range values {
		*ps = append(*ps, f(value...))
	}
}
