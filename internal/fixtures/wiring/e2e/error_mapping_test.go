package e2e

import (
	"errors"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	// Aliased back to `ent`. The fixture package is named wiringent so that
	// no two packages in the generator's own module answer to the name
	// `ent` (#49);
	// a real consumer's generated package IS named `ent`, and these tests are
	// written from the consumer's side, so the alias keeps them reading the way
	// the code they stand in for does. Being spelled out, it is also nothing
	// goimports has to resolve.
	ent "github.com/githonllc/entapi/internal/fixtures/wiring/wiringent"
	entapi "github.com/githonllc/entapi/runtime"
)

// This file is the behavioural proof for #13: error classification in the
// GENERATED wiring, driven against real ent and real SQLite.
//
// It cannot be replaced by a unit test over ErrorMapper. The runtime's own
// tests use fake predicates — they have to, because the runtime must stay
// linkable without ent — so they can only show that the mapper obeys the
// predicates it was given. The claim this issue makes is about the predicates
// ent actually supplies, and there is exactly one way to find out what
// ent.IsConstraintError returns for a foreign-key violation: cause one.
//
// Four axes, with validation and the deliberately unclassified residue added
// by #74:
//
//	a missing row            -> entapi.ErrNotFound, through every operation
//	a UNIQUE violation       -> entapi.ErrAlreadyExists
//	a FOREIGN KEY violation  -> NOT already-exists, NOT not-found, not swallowed
//	a Save validation        -> entapi.ErrValidation with its field name
//	a bare required-edge error -> no sentinel, not swallowed
//
// Ent reports UNIQUE and FOREIGN KEY failures identically as
// *ent.ConstraintError. The validation and bare-error axes prove the two
// separate Save-time shapes added by #74.

// installUniqueViolation is the one line a consumer writes to get already-exists
// classification, and t.Cleanup restores the default so the test that asserts
// the UNINSTALLED behaviour is not order-dependent.
func installUniqueViolation(t *testing.T) {
	t.Helper()
	prev := ent.ErrorMap
	isUnique, ok := entapi.UniqueViolation(dialect.SQLite)
	if !ok {
		t.Fatalf("no uniqueness determination for %q", dialect.SQLite)
	}
	ent.ErrorMap = ent.ErrorMap.WithUniqueViolation(isUnique)
	t.Cleanup(func() { ent.ErrorMap = prev })
}

func validAuthorPatch(t *testing.T, body string) *ent.ValidAuthorPatchRequest {
	t.Helper()
	var p ent.AuthorPatchRequest
	if err := p.UnmarshalJSON([]byte(body)); err != nil {
		t.Fatalf("unmarshal author patch %s: %v", body, err)
	}
	v, err := p.Validate()
	if err != nil {
		t.Fatalf("validate author patch %s: %v", body, err)
	}
	return v
}

func validArticlePatch(t *testing.T, body string) *ent.ValidArticlePatchRequest {
	t.Helper()
	var p ent.ArticlePatchRequest
	if err := p.UnmarshalJSON([]byte(body)); err != nil {
		t.Fatalf("unmarshal article patch %s: %v", body, err)
	}
	v, err := p.Validate()
	if err != nil {
		t.Fatalf("validate article patch %s: %v", body, err)
	}
	return v
}

func validNotePatch(t *testing.T, body string) *ent.ValidNotePatchRequest {
	t.Helper()
	var p ent.NotePatchRequest
	if err := p.UnmarshalJSON([]byte(body)); err != nil {
		t.Fatalf("unmarshal note patch %s: %v", body, err)
	}
	v, err := p.Validate()
	if err != nil {
		t.Fatalf("validate note patch %s: %v", body, err)
	}
	return v
}

// TestMissingRowMapsToNotFoundEverywhere is acceptance criteria 1 and 3: every
// generated operation that takes an identifier maps a missing row to the same
// sentinel, so a caller can branch on entapi.IsNotFound without knowing
// which operation produced the error.
//
// Three entities, because the wiring has three shapes: Author re-reads through
// its to-many eager-load plan after a mutation, Article through a to-one, and
// Note converts the builder's own return value with no re-read at all. A
// mapping that only survived one of those paths would be a mapping that depends
// on the schema.
func TestMissingRowMapsToNotFoundEverywhere(t *testing.T) {
	c, ctx := newClient(t)
	missing := uuid.New()

	ops := []struct {
		name string
		run  func() error
	}{
		{"GetAuthor", func() error { _, err := ent.GetAuthor(ctx, c, missing); return err }},
		{"GetArticle", func() error { _, err := ent.GetArticle(ctx, c, missing); return err }},
		{"GetNote", func() error { _, err := ent.GetNote(ctx, c, missing); return err }},

		{"PatchAuthor", func() error {
			_, err := ent.PatchAuthor(ctx, c, missing, validAuthorPatch(t, `{"name":"ghost"}`))
			return err
		}},
		{"PatchArticle", func() error {
			_, err := ent.PatchArticle(ctx, c, missing, validArticlePatch(t, `{"title":"ghost"}`))
			return err
		}},
		{"PatchNote", func() error {
			_, err := ent.PatchNote(ctx, c, missing, validNotePatch(t, `{"body":"ghost"}`))
			return err
		}},

		{"DeleteAuthor", func() error { return ent.DeleteAuthor(ctx, c, missing) }},
		{"DeleteArticle", func() error { return ent.DeleteArticle(ctx, c, missing) }},
		{"DeleteNote", func() error { return ent.DeleteNote(ctx, c, missing) }},
	}

	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			err := op.run()
			if err == nil {
				t.Fatalf("%s on a missing row returned no error", op.name)
			}
			if !entapi.IsNotFound(err) {
				t.Errorf("%s: err = %v, want entapi.ErrNotFound in the chain", op.name, err)
			}
			if entapi.IsAlreadyExists(err) {
				t.Errorf("%s: a missing row must not read as already-exists: %v", op.name, err)
			}
			// The sentinel is added to the chain, not substituted for it: a
			// caller that wants ent's own error still has it.
			if !ent.IsNotFound(err) {
				t.Errorf("%s: the ent error left the chain: %v", op.name, err)
			}
		})
	}
}

// TestOperationsThatCannotSeeAMissingRow accounts for the other three generated
// operations explicitly rather than leaving their absence above unexplained.
// None of them takes an identifier that has to resolve, so there is no
// not-found to map — and asserting that is what distinguishes "cannot happen"
// from "was forgotten".
func TestOperationsThatCannotSeeAMissingRow(t *testing.T) {
	c, ctx := newClient(t)
	missing := uuid.New()

	t.Run("CreateNote takes no identifier", func(t *testing.T) {
		v, err := (&ent.NoteCreateRequest{Body: "present"}).Validate()
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		if _, err := ent.CreateNote(ctx, c, v); err != nil {
			t.Fatalf("CreateNote: %v", err)
		}
	})

	t.Run("ListNotes returns an empty page", func(t *testing.T) {
		p, err := ent.ListNotes(ctx, c, &ent.NoteFilter{Body: []string{"absent"}}, entapi.ListRequest{})
		if err != nil {
			t.Fatalf("ListNotes: %v", err)
		}
		if p.Total != 0 {
			t.Errorf("total = %d, want 0", p.Total)
		}
	})

	t.Run("DeleteBatchNotes reports a zero count", func(t *testing.T) {
		n, err := ent.DeleteBatchNotes(ctx, c, []uuid.UUID{missing})
		if err != nil {
			t.Fatalf("DeleteBatchNotes: %v", err)
		}
		if n != 0 {
			t.Errorf("deleted %d rows, want 0", n)
		}
	})
}

// TestUniqueViolationMapsToAlreadyExists is acceptance criterion 2, first half.
func TestUniqueViolationMapsToAlreadyExists(t *testing.T) {
	c, ctx := newClient(t)
	installUniqueViolation(t)

	createAuthor(t, ctx, c, "ada")

	t.Run("CreateAuthor", func(t *testing.T) {
		v, err := (&ent.AuthorCreateRequest{Name: "ada"}).Validate()
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		_, err = ent.CreateAuthor(ctx, c, v)
		if err == nil {
			t.Fatal("a duplicate name was accepted; the fixture's unique constraint is missing")
		}
		if !entapi.IsAlreadyExists(err) {
			t.Errorf("err = %v, want entapi.ErrAlreadyExists in the chain", err)
		}
		if entapi.IsNotFound(err) {
			t.Errorf("a duplicate must not read as not-found: %v", err)
		}
		if !ent.IsConstraintError(err) {
			t.Errorf("the ent error left the chain: %v", err)
		}
	})

	t.Run("PatchAuthor", func(t *testing.T) {
		bob := createAuthor(t, ctx, c, "bob")
		_, err := ent.PatchAuthor(ctx, c, bob.ID, validAuthorPatch(t, `{"name":"ada"}`))
		if err == nil {
			t.Fatal("renaming onto an existing name was accepted")
		}
		if !entapi.IsAlreadyExists(err) {
			t.Errorf("err = %v, want entapi.ErrAlreadyExists in the chain", err)
		}
		if entapi.IsNotFound(err) {
			t.Errorf("a duplicate must not read as not-found: %v", err)
		}
	})
}

// TestForeignKeyViolationIsNeverADuplicate is acceptance criterion 2, second
// half, and the assertion the whole issue turns on. The uniqueness predicate IS
// installed here — the point is not that an unconfigured mapper stays quiet, it
// is that a configured one still refuses to call a foreign-key failure a
// duplicate, even though ent.IsConstraintError returns true for both.
func TestForeignKeyViolationIsNeverADuplicate(t *testing.T) {
	c, ctx := newClient(t)
	installUniqueViolation(t)

	author := createAuthor(t, ctx, c, "ada")
	article := createArticle(t, ctx, c, "alpha", ptr(1), author.ID)
	orphanID := uuid.New()

	assertConstraintButNotClassified := func(t *testing.T, op string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected a foreign-key violation, got none", op)
		}
		if !ent.IsConstraintError(err) {
			t.Fatalf("%s: precondition — ent must call this a constraint error, got %v", op, err)
		}
		if entapi.IsAlreadyExists(err) {
			t.Errorf("#13: %s reported a foreign-key violation as a duplicate: %v", op, err)
		}
		if entapi.IsNotFound(err) {
			t.Errorf("%s: a foreign-key violation must not read as not-found: %v", op, err)
		}
	}

	t.Run("CreateArticle with an unknown author", func(t *testing.T) {
		v, err := (&ent.ArticleCreateRequest{Title: "orphan", AuthorID: orphanID}).Validate()
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		_, err = ent.CreateArticle(ctx, c, v)
		assertConstraintButNotClassified(t, "CreateArticle", err)
	})

	t.Run("PatchArticle onto an unknown author", func(t *testing.T) {
		_, err := ent.PatchArticle(ctx, c, article.ID, validArticlePatch(t, `{"author_id":"`+orphanID.String()+`"}`))
		assertConstraintButNotClassified(t, "PatchArticle", err)
	})

	t.Run("DeleteAuthor that is still referenced", func(t *testing.T) {
		assertConstraintButNotClassified(t, "DeleteAuthor", ent.DeleteAuthor(ctx, c, author.ID))
	})

	t.Run("DeleteBatchAuthors that are still referenced", func(t *testing.T) {
		_, err := ent.DeleteBatchAuthors(ctx, c, []uuid.UUID{author.ID})
		assertConstraintButNotClassified(t, "DeleteBatchAuthors", err)
	})
}

// TestWithoutTheUniquenessPredicateNothingClaimsAlreadyExists pins the failure
// mode the design accepted: a consumer who never installs a uniqueness
// predicate loses the already-exists classification entirely and keeps
// everything else. That is the safe direction — a 500 on a duplicate is
// recoverable, a 409 on a foreign-key failure sends the caller to fix the wrong
// thing.
//
// It runs with the DEFAULT ErrorMap, which is what the generated wiring
// declares.
func TestWithoutTheUniquenessPredicateNothingClaimsAlreadyExists(t *testing.T) {
	c, ctx := newClient(t)

	createAuthor(t, ctx, c, "ada")
	v, err := (&ent.AuthorCreateRequest{Name: "ada"}).Validate()
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	_, err = ent.CreateAuthor(ctx, c, v)
	if err == nil {
		t.Fatal("a duplicate name was accepted; the fixture's unique constraint is missing")
	}
	if !ent.IsConstraintError(err) {
		t.Fatalf("the ent error was replaced rather than passed through: %v", err)
	}
	if entapi.IsAlreadyExists(err) {
		t.Errorf("without a uniqueness predicate the wiring must not claim already-exists: %v", err)
	}
	if entapi.IsNotFound(err) {
		t.Errorf("a duplicate must not read as not-found: %v", err)
	}
	// Not-found still works with the default mapper: the two-predicate form
	// classifies it, and only already-exists is given up.
	if _, err := ent.GetAuthor(ctx, c, uuid.New()); !entapi.IsNotFound(err) {
		t.Errorf("GetAuthor on a missing row = %v, want entapi.ErrNotFound", err)
	}
}

func TestPatchSaveValidationMapsFieldName(t *testing.T) {
	c, ctx := newClient(t)
	author := createAuthor(t, ctx, c, "ada")

	_, err := ent.PatchAuthor(ctx, c, author.ID,
		validAuthorPatch(t, `{"email":"`+strings.Repeat("x", 65)+`"}`))
	if err == nil {
		t.Fatal("an email longer than the schema maximum was accepted")
	}
	if !entapi.IsValidation(err) {
		t.Fatalf("err = %v, want entapi.ErrValidation in the chain", err)
	}
	var fieldErr *entapi.FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != "email" {
		t.Fatalf("field error = %+v, want field email; err = %v", fieldErr, err)
	}
	if entapi.IsNotFound(err) || entapi.IsAlreadyExists(err) {
		t.Fatalf("validation error carries a wrong sentinel: %v", err)
	}
}

func TestRequiredEdgeClearResidueCarriesNoSentinel(t *testing.T) {
	c, ctx := newClient(t)
	author := createAuthor(t, ctx, c, "ada")
	article := createArticle(t, ctx, c, "owned", nil, author.ID)

	err := c.Article.UpdateOneID(article.ID).ClearAuthor().Exec(ctx)
	if err == nil {
		t.Fatal("clearing the required Article.author edge returned no error")
	}
	mapped := ent.ErrorMap.MapError(err)
	if entapi.IsNotFound(mapped) || entapi.IsAlreadyExists(mapped) || entapi.IsValidation(mapped) {
		t.Fatalf("bare required-edge error carries a sentinel: %v", mapped)
	}
	if !errors.Is(mapped, err) {
		t.Fatalf("bare required-edge error was swallowed: %v", mapped)
	}
}
