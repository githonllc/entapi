package entapi

import (
	"errors"
	"testing"
)

// The predicates are fakes on purpose. The mapper must be testable — and
// linkable — without ent, which is the whole reason it takes function values
// instead of importing ent and calling ent.IsNotFound directly.

var (
	errMissingRow = errors.New("driver: no rows in result set")
	errUnique     = errors.New("driver: UNIQUE constraint failed: tags.name")
	errForeignKey = errors.New("driver: FOREIGN KEY constraint failed")
	errCheck      = errors.New("driver: CHECK constraint failed: age_positive")
	errUnrelated  = errors.New("driver: connection reset")
)

func fakeIsNotFound(err error) bool { return errors.Is(err, errMissingRow) }

// fakeIsConstraint stands in for ent.IsConstraintError, which — verified
// against real ent and SQLite — is true for UNIQUE *and* FOREIGN KEY *and*
// CHECK failures alike. That is exactly why it cannot be the already-exists
// signal on its own.
func fakeIsConstraint(err error) bool {
	return errors.Is(err, errUnique) || errors.Is(err, errForeignKey) || errors.Is(err, errCheck)
}

func fakeIsUnique(err error) bool { return errors.Is(err, errUnique) }

func TestErrorMapper_NotFound(t *testing.T) {
	m := NewErrorMapper(fakeIsNotFound, fakeIsConstraint)
	got := m.MapError(errMissingRow)

	if !IsNotFound(got) {
		t.Fatalf("a missing row must satisfy IsNotFound, got %v", got)
	}
	if !errors.Is(got, errMissingRow) {
		t.Error("the original error must stay in the chain, not be replaced")
	}
	if IsAlreadyExists(got) {
		t.Error("a missing row must not also read as already-exists")
	}
}

func TestErrorMapper_UniquenessNeedsItsOwnPredicate(t *testing.T) {
	t.Run("with a uniqueness predicate a duplicate maps to ErrAlreadyExists", func(t *testing.T) {
		m := NewErrorMapper(fakeIsNotFound, fakeIsConstraint).WithUniqueViolation(fakeIsUnique)
		got := m.MapError(errUnique)

		if !IsAlreadyExists(got) {
			t.Fatalf("a uniqueness violation must satisfy IsAlreadyExists, got %v", got)
		}
		if !errors.Is(got, errUnique) {
			t.Error("the original error must stay in the chain")
		}
	})

	t.Run("without one, nothing is claimed to be a duplicate", func(t *testing.T) {
		m := NewErrorMapper(fakeIsNotFound, fakeIsConstraint)
		if got := m.MapError(errUnique); IsAlreadyExists(got) {
			t.Fatalf("two predicates cannot tell UNIQUE from FOREIGN KEY; "+
				"claiming already-exists here is the #13 bug, got %v", got)
		}
	})
}

// TestErrorMapper_OtherConstraintsAreNotDuplicates is the defect #13 was filed
// for: ent.IsConstraintError alone drove the mapping, so a foreign-key
// violation was reported to the caller as a duplicate key.
func TestErrorMapper_OtherConstraintsAreNotDuplicates(t *testing.T) {
	m := NewErrorMapper(fakeIsNotFound, fakeIsConstraint).WithUniqueViolation(fakeIsUnique)

	for name, err := range map[string]error{
		"foreign key": errForeignKey,
		"check":       errCheck,
	} {
		t.Run(name, func(t *testing.T) {
			got := m.MapError(err)
			if IsAlreadyExists(got) {
				t.Fatalf("%s violation must not read as already-exists, got %v", name, got)
			}
			if IsNotFound(got) {
				t.Fatalf("%s violation must not read as not-found, got %v", name, got)
			}
			if got == nil || !errors.Is(got, err) {
				t.Fatalf("%s violation must not be swallowed, got %v", name, got)
			}
		})
	}
}

func TestErrorMapper_PassesThroughWhatItCannotClassify(t *testing.T) {
	m := NewErrorMapper(fakeIsNotFound, fakeIsConstraint).WithUniqueViolation(fakeIsUnique)

	if got := m.MapError(nil); got != nil {
		t.Fatalf("nil must map to nil, got %v", got)
	}
	got := m.MapError(errUnrelated)
	if !errors.Is(got, errUnrelated) {
		t.Fatalf("an unclassifiable error must survive, got %v", got)
	}
	if IsNotFound(got) || IsAlreadyExists(got) || IsValidation(got) {
		t.Fatalf("an unclassifiable error must carry no sentinel, got %v", got)
	}
}

// TestErrorMapper_ZeroValueIsSafe: a mapper nobody wired must not panic and
// must not invent classifications.
func TestErrorMapper_ZeroValueIsSafe(t *testing.T) {
	var m ErrorMapper
	if got := m.MapError(errMissingRow); !errors.Is(got, errMissingRow) || IsNotFound(got) {
		t.Fatalf("zero-value mapper must pass errors through unchanged, got %v", got)
	}
	if got := m.MapError(nil); got != nil {
		t.Fatalf("zero-value mapper must map nil to nil, got %v", got)
	}
}

// TestErrorMapper_BuilderReturnsACopy pins the same contract the annotation
// builders declare: a builder never mutates its receiver.
func TestErrorMapper_BuilderReturnsACopy(t *testing.T) {
	base := NewErrorMapper(fakeIsNotFound, fakeIsConstraint)
	derived := base.WithUniqueViolation(fakeIsUnique)

	if IsAlreadyExists(base.MapError(errUnique)) {
		t.Fatal("WithUniqueViolation mutated its receiver")
	}
	if !IsAlreadyExists(derived.MapError(errUnique)) {
		t.Fatal("WithUniqueViolation did not take effect on the copy")
	}
}

// TestErrorMapper_NotFoundWinsOverConstraint fixes the precedence rather than
// leaving it to predicate order, since a caller can supply overlapping ones.
func TestErrorMapper_NotFoundWinsOverConstraint(t *testing.T) {
	always := func(error) bool { return true }
	m := NewErrorMapper(always, always).WithUniqueViolation(always)
	got := m.MapError(errUnrelated)

	if !IsNotFound(got) {
		t.Fatalf("not-found must be decided first, got %v", got)
	}
	if IsAlreadyExists(got) {
		t.Fatalf("an error must not carry two sentinels, got %v", got)
	}
}
