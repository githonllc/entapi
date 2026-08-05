package entdomain

import "errors"

// Sentinel errors returned by generated repositories. Use errors.Is() or the
// provided Is* helpers to check error types without string matching.
var (
	// ErrNotFound indicates the requested entity does not exist.
	ErrNotFound = errors.New("entity not found")

	// ErrAlreadyExists indicates a uniqueness constraint violation.
	ErrAlreadyExists = errors.New("entity already exists")

	// ErrValidation indicates the input failed validation.
	ErrValidation = errors.New("validation failed")
)

// IsNotFound reports whether err (or any error in its chain) is ErrNotFound.
//
// This is NOT the IsNotFound that generated code calls. templates/base_service.tmpl
// emits `if IsNotFound(err)` unqualified, and the emitted file lands in the
// consumer's `package ent`, so that call binds to Ent's own generated
// IsNotFound, which unwraps *ent.NotFoundError. The generated code then
// translates the result into ErrNotFound, and it is that translated error this
// function recognises. The two are complementary, not interchangeable:
//
//	generated service:  Ent's IsNotFound(err)  →  wraps as entdomain.ErrNotFound
//	consumer code:      entdomain.IsNotFound(err)
//
// Qualifying the template's call as `{{ entdomainPkg }}.IsNotFound` would still
// compile and would silently stop matching Ent's not-found error.
// TestGeneratedErrorPredicatesResolveUnambiguously guards against that.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsAlreadyExists reports whether err (or any error in its chain) is ErrAlreadyExists.
func IsAlreadyExists(err error) bool { return errors.Is(err, ErrAlreadyExists) }

// IsValidation reports whether err (or any error in its chain) is ErrValidation.
func IsValidation(err error) bool { return errors.Is(err, ErrValidation) }
