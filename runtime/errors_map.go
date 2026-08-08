package entapi

import "fmt"

// ErrorMapper translates a persistence layer's errors into this package's
// sentinels: [ErrNotFound], [ErrValidation], and [ErrAlreadyExists].
//
// It takes its classification as function values rather than importing ent and
// calling ent.IsNotFound directly, for two reasons. The runtime core must stay
// ent-free — see the package documentation — and ent's error types are
// generated per consumer, so a framework-level errors.As against them is not
// expressible here anyway.
//
// The generated wiring supplies the predicates in one line:
//
//	var mapper = entapi.NewErrorMapper(ent.IsNotFound, ent.IsConstraintError)
//
// # Why uniqueness is a separate predicate
//
// ent.IsConstraintError cannot serve as the already-exists signal. Verified
// against real ent and SQLite, both of these produce an *ent.ConstraintError
// for which it returns true:
//
//	UNIQUE constraint failed: tags.name (2067)
//	FOREIGN KEY constraint failed (787)
//
// Mapping that predicate to [ErrAlreadyExists] therefore reports a foreign-key
// violation as a duplicate key, which is the defect tracked in issue #13. The
// distinction lives in the driver error wrapped by *ent.ConstraintError and is
// dialect-specific, so this package does not guess it: supply it with
// [ErrorMapper.WithUniqueViolation] or get no already-exists classification at
// all. Silence is the safe default here; a wrong 409 is not.
//
// The zero value is usable and classifies nothing.
type ErrorMapper struct {
	isNotFound      func(error) bool
	isValidation    func(error) bool
	validationField func(error) (string, bool)
	isConstraint    func(error) bool
	isUnique        func(error) bool
}

// NewErrorMapper returns a mapper driven by the given predicates.
//
// isConstraint does not by itself produce a classification. It gates the
// uniqueness test, so a uniqueness predicate is only ever consulted for errors
// the persistence layer already agrees are constraint violations — which keeps
// a dialect-specific sniffer from firing on an unrelated error that happens to
// mention a constraint.
func NewErrorMapper(isNotFound, isConstraint func(error) bool) ErrorMapper {
	return ErrorMapper{isNotFound: isNotFound, isConstraint: isConstraint}
}

// WithUniqueViolation returns a copy that classifies uniqueness violations as
// [ErrAlreadyExists]. Value receiver: the builder never mutates its receiver,
// matching the annotation builders' contract.
//
// isUnique is only consulted for errors isConstraint accepts.
func (m ErrorMapper) WithUniqueViolation(isUnique func(error) bool) ErrorMapper {
	m.isUnique = isUnique
	return m
}

// HasUniqueViolation reports whether a uniqueness determination is installed.
// It lets generated HTTP wiring preserve a determination the consumer installed
// before constructing the API.
func (m ErrorMapper) HasUniqueViolation() bool {
	return m.isUnique != nil
}

// WithValidation returns a copy that classifies validation errors as
// [ErrValidation]. When field extracts a name, the mapped error also carries a
// [FieldError] so [WriteProblem] can expose that name on the wire.
//
// Both functions receive the original error and may inspect its wrap chain.
func (m ErrorMapper) WithValidation(isValidation func(error) bool, field func(error) (string, bool)) ErrorMapper {
	m.isValidation = isValidation
	m.validationField = field
	return m
}

// MapError classifies err, wrapping it so that both the sentinel and the
// original error remain in the chain and errors.Is finds either.
//
// The order is fixed rather than left to whichever predicate happens to match
// first, because a caller can supply overlapping predicates:
//
//  1. nil maps to nil.
//  2. A missing row maps to [ErrNotFound].
//  3. A validation failure maps to [ErrValidation]; a field name extracted
//     from it is carried by [FieldError].
//  4. A constraint violation the uniqueness predicate recognises maps to
//     [ErrAlreadyExists].
//  5. Anything else — including a constraint violation of an unidentified kind
//     — is returned unchanged. Unclassified, never swallowed, and never
//     labelled with a sentinel that was not established.
func (m ErrorMapper) MapError(err error) error {
	if err == nil {
		return nil
	}
	if m.isNotFound != nil && m.isNotFound(err) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	if m.isValidation != nil && m.isValidation(err) {
		mapped := fmt.Errorf("%w: %w", ErrValidation, err)
		if m.validationField != nil {
			if field, ok := m.validationField(err); ok {
				return &FieldError{Field: field, Err: mapped}
			}
		}
		return mapped
	}
	if m.isConstraint != nil && m.isConstraint(err) {
		if m.isUnique != nil && m.isUnique(err) {
			return fmt.Errorf("%w: %w", ErrAlreadyExists, err)
		}
	}
	return err
}
