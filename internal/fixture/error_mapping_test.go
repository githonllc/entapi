package fixture

import (
	"errors"
	"strings"
	"testing"

	"github.com/githonllc/entdomain"
	"github.com/githonllc/entdomain/internal/fixture/ent"
	"github.com/google/uuid"
)

// This is the wiring line the generator will emit, run against a real ent
// client and real SQLite. The root-package tests use fake predicates because
// the runtime must be linkable without ent; this file checks that the fakes
// were faithful to what ent actually does.
func newMapper() entdomain.ErrorMapper {
	return entdomain.NewErrorMapper(ent.IsNotFound, ent.IsConstraintError).
		// The dialect-specific half belongs to the consumer, not the library.
		// SQLite reports "UNIQUE constraint failed: tags.name (2067)"; Postgres
		// would use SQLSTATE 23505.
		WithUniqueViolation(func(err error) bool {
			return strings.Contains(err.Error(), "UNIQUE constraint failed")
		})
}

func TestMapErrorAgainstRealEnt(t *testing.T) {
	c, ctx := newClient(t)
	m := newMapper()

	t.Run("missing row maps to ErrNotFound", func(t *testing.T) {
		_, err := c.Tag.Get(ctx, 9999)
		if err == nil {
			t.Fatal("expected a not-found error")
		}
		got := m.MapError(err)
		if !entdomain.IsNotFound(got) {
			t.Fatalf("want ErrNotFound, got %v", got)
		}
		if !errors.Is(got, err) {
			t.Error("the ent error must stay in the chain")
		}
		if entdomain.IsAlreadyExists(got) {
			t.Error("a missing row must not read as already-exists")
		}
	})

	t.Run("uniqueness violation maps to ErrAlreadyExists", func(t *testing.T) {
		c.Tag.Create().SetName("dup").SaveX(ctx)
		_, err := c.Tag.Create().SetName("dup").Save(ctx)
		if err == nil {
			t.Fatal("expected a uniqueness violation")
		}
		got := m.MapError(err)
		if !entdomain.IsAlreadyExists(got) {
			t.Fatalf("want ErrAlreadyExists, got %v", got)
		}
		if entdomain.IsNotFound(got) {
			t.Error("a duplicate must not read as not-found")
		}
	})

	t.Run("foreign-key violation is not a duplicate", func(t *testing.T) {
		// author_id references a user that does not exist. ent reports this as
		// *ent.ConstraintError, exactly as it reports a duplicate key — which
		// is why ent.IsConstraintError cannot drive the mapping on its own.
		_, err := c.Post.Create().SetTitle("orphan").SetAuthorID(uuid.New()).Save(ctx)
		if err == nil {
			t.Fatal("expected a foreign-key violation")
		}
		if !ent.IsConstraintError(err) {
			t.Fatalf("precondition: ent must call this a constraint error, got %T", err)
		}
		got := m.MapError(err)
		if entdomain.IsAlreadyExists(got) {
			t.Fatalf("#13: a foreign-key violation must not be reported as a duplicate, got %v", got)
		}
		if entdomain.IsNotFound(got) {
			t.Fatalf("a foreign-key violation must not read as not-found, got %v", got)
		}
		if got == nil || !errors.Is(got, err) {
			t.Fatalf("it must not be swallowed either, got %v", got)
		}
	})
}

// TestTwoPredicatesCannotSeeUniqueness records against real ent why the
// uniqueness predicate exists at all: ent.IsConstraintError returns true for a
// duplicate key and a foreign-key violation alike, so a mapper wired with only
// the two predicates classifies neither rather than guessing.
func TestTwoPredicatesCannotSeeUniqueness(t *testing.T) {
	c, ctx := newClient(t)
	m := entdomain.NewErrorMapper(ent.IsNotFound, ent.IsConstraintError)

	c.Tag.Create().SetName("dup").SaveX(ctx)
	dup, err := c.Tag.Create().SetName("dup").Save(ctx)
	if err == nil {
		t.Fatalf("expected a uniqueness violation, created %v", dup)
	}
	_, fkErr := c.Post.Create().SetTitle("orphan").SetAuthorID(uuid.New()).Save(ctx)
	if fkErr == nil {
		t.Fatal("expected a foreign-key violation")
	}

	if !ent.IsConstraintError(err) || !ent.IsConstraintError(fkErr) {
		t.Fatal("precondition: ent calls both a constraint error")
	}
	if entdomain.IsAlreadyExists(m.MapError(err)) {
		t.Error("without a uniqueness predicate the mapper must not claim already-exists")
	}
	if entdomain.IsAlreadyExists(m.MapError(fkErr)) {
		t.Error("a foreign-key violation must never claim already-exists")
	}
}
