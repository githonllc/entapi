package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	stdsql "database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	// Aliased back to `ent`. The fixture package is named wiringent so that
	// no two packages in the generator's own module answer to the name
	// `ent` (#49);
	// a real consumer's generated package IS named `ent`, and these tests are
	// written from the consumer's side, so the alias keeps them reading the way
	// the code they stand in for does. Being spelled out, it is also nothing
	// goimports has to resolve.
	ent "github.com/githonllc/entapi/internal/fixtures/wiring/wiringent"
	"github.com/githonllc/entapi/internal/fixtures/wiring/wiringent/article"
	entapi "github.com/githonllc/entapi/runtime"
)

// newClient opens a per-test in-memory database. A shared DSN would only work
// because tests happen to run sequentially, which is luck rather than isolation.
func newClient(t *testing.T, opts ...ent.Option) (*ent.Client, context.Context) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := stdsql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	opts = append(opts, ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	c := ent.NewClient(opts...)
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return c, ctx
}

func ptr[T any](v T) *T { return &v }

// createAuthor drives the generated create wiring and fails the test on error.
func createAuthor(t *testing.T, ctx context.Context, c *ent.Client, name string) *ent.AuthorResponse {
	t.Helper()
	v, err := (&ent.AuthorCreateRequest{Name: name}).Validate()
	if err != nil {
		t.Fatalf("validate author %q: %v", name, err)
	}
	got, err := ent.CreateAuthor(ctx, c, v)
	if err != nil {
		t.Fatalf("CreateAuthor(%q): %v", name, err)
	}
	return got
}

// createArticle drives the generated create wiring and fails the test on error.
func createArticle(t *testing.T, ctx context.Context, c *ent.Client, title string, rank *int, authorID uuid.UUID) *ent.ArticleResponse {
	t.Helper()
	v, err := (&ent.ArticleCreateRequest{Title: title, Rank: rank, AuthorID: authorID}).Validate()
	if err != nil {
		t.Fatalf("validate article %q: %v", title, err)
	}
	got, err := ent.CreateArticle(ctx, c, v)
	if err != nil {
		t.Fatalf("CreateArticle(%q): %v", title, err)
	}
	return got
}

func titles(p *entapi.Page[ent.ArticleResponse]) []string {
	out := make([]string, 0, len(p.Data))
	for _, r := range p.Data {
		out = append(out, r.Title)
	}
	return out
}

// TestWiringCreateReadListPatchDelete is the behavioural proof for #28: every
// generated operation is driven against a real database and its RESULT is
// asserted, not merely its existence. Wiring that compiles and returns the
// wrong page is exactly the failure the fixture exists to catch.
func TestWiringCreateReadListPatchDelete(t *testing.T) {
	c, ctx := newClient(t)

	author := createAuthor(t, ctx, c, "ada")

	// The create response is built through the eager-load plan, so a to-many
	// edge with no rows is an empty slice — not the "edge not loaded" error a
	// mutation builder's own return value would produce.
	if author.Articles == nil {
		t.Errorf("CreateAuthor returned Articles == nil; the response was not built through the eager-load plan")
	}
	if len(author.Articles) != 0 {
		t.Errorf("CreateAuthor returned %d articles, want 0", len(author.Articles))
	}

	first := createArticle(t, ctx, c, "beta", ptr(2), author.ID)
	if first.Author == nil {
		t.Fatalf("CreateArticle returned a nil Author; the to-one response edge was not loaded")
	}
	if first.Author.Name != "ada" {
		t.Errorf("CreateArticle author name = %q, want %q", first.Author.Name, "ada")
	}
	createArticle(t, ctx, c, "alpha", ptr(1), author.ID)
	createArticle(t, ctx, c, "gamma", nil, author.ID)

	t.Run("get reads it back with its edge", func(t *testing.T) {
		got, err := ent.GetArticle(ctx, c, first.ID)
		if err != nil {
			t.Fatalf("GetArticle: %v", err)
		}
		if got.Title != "beta" {
			t.Errorf("title = %q, want %q", got.Title, "beta")
		}
		if got.Author == nil || got.Author.ID != author.ID {
			t.Errorf("author = %+v, want the summary for %s", got.Author, author.ID)
		}
	})

	t.Run("author response now carries its to-many edge", func(t *testing.T) {
		got, err := ent.GetAuthor(ctx, c, author.ID)
		if err != nil {
			t.Fatalf("GetAuthor: %v", err)
		}
		if len(got.Articles) != 3 {
			t.Errorf("author articles = %d, want 3", len(got.Articles))
		}
	})

	t.Run("list applies the filter and the sort", func(t *testing.T) {
		p, err := ent.ListArticles(ctx, c, &ent.ArticleFilter{}, entapi.ListRequest{SortBy: "title", Order: "asc"})
		if err != nil {
			t.Fatalf("ListArticles: %v", err)
		}
		if p.Total != 3 {
			t.Errorf("total = %d, want 3", p.Total)
		}
		if got, want := titles(p), []string{"alpha", "beta", "gamma"}; !equal(got, want) {
			t.Errorf("ascending titles = %v, want %v", got, want)
		}

		p, err = ent.ListArticles(ctx, c, &ent.ArticleFilter{}, entapi.ListRequest{SortBy: "title", Order: "desc"})
		if err != nil {
			t.Fatalf("ListArticles desc: %v", err)
		}
		if got, want := titles(p), []string{"gamma", "beta", "alpha"}; !equal(got, want) {
			t.Errorf("descending titles = %v, want %v", got, want)
		}

		p, err = ent.ListArticles(ctx, c, &ent.ArticleFilter{TitleContains: ptr("a")}, entapi.ListRequest{SortBy: "title"})
		if err != nil {
			t.Fatalf("ListArticles filtered: %v", err)
		}
		if p.Total != 3 {
			t.Errorf("contains 'a' total = %d, want 3", p.Total)
		}

		p, err = ent.ListArticles(ctx, c, &ent.ArticleFilter{RankGTE: ptr(2)}, entapi.ListRequest{SortBy: "title"})
		if err != nil {
			t.Fatalf("ListArticles rank: %v", err)
		}
		if got, want := titles(p), []string{"beta"}; !equal(got, want) {
			t.Errorf("rank >= 2 titles = %v, want %v", got, want)
		}
	})

	t.Run("list pages", func(t *testing.T) {
		p, err := ent.ListArticles(ctx, c, nil, entapi.ListRequest{SortBy: "title", Size: 2, Page: 2})
		if err != nil {
			t.Fatalf("ListArticles paged: %v", err)
		}
		if p.Total != 3 || p.Page != 2 || p.Size != 2 {
			t.Errorf("page = %+v, want total 3 page 2 size 2", p)
		}
		if got, want := titles(p), []string{"gamma"}; !equal(got, want) {
			t.Errorf("second page = %v, want %v", got, want)
		}
	})

	// #65: the named, non-generic list shape a consumer annotates for
	// OpenAPI/swaggo is a plain conversion of the page the wiring returns, so
	// the two must be indistinguishable on the wire. The conversion already
	// makes a field-set, type or order drift a compile error; this asserts the
	// remaining half — that converting changes no byte of the payload.
	t.Run("the named list shape marshals byte-identically to the page", func(t *testing.T) {
		p, err := ent.ListArticles(ctx, c, nil, entapi.ListRequest{SortBy: "title"})
		if err != nil {
			t.Fatalf("ListArticles: %v", err)
		}

		fromPage, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshal page: %v", err)
		}
		fromNamed, err := json.Marshal(ent.NewArticleListResponse(p))
		if err != nil {
			t.Fatalf("marshal list response: %v", err)
		}
		if !bytes.Equal(fromPage, fromNamed) {
			t.Errorf("ArticleListResponse marshals to\n  %s\npage marshals to\n  %s", fromNamed, fromPage)
		}

		if ent.NewArticleListResponse(nil) != nil {
			t.Error("NewArticleListResponse(nil) must be nil, matching the nil convention of NewArticleResponse")
		}
	})

	t.Run("list refuses a sort key outside the allow-list", func(t *testing.T) {
		for _, key := range []string{"author_id", "title; DROP TABLE articles --", "id"} {
			_, err := ent.ListArticles(ctx, c, nil, entapi.ListRequest{SortBy: key})
			if !errors.Is(err, entapi.ErrValidation) {
				t.Errorf("sort by %q: err = %v, want ErrValidation", key, err)
			}
		}
	})

	t.Run("patch sets one field and clears another", func(t *testing.T) {
		var patch ent.ArticlePatchRequest
		if err := patch.UnmarshalJSON([]byte(`{"title":"beta II","rank":null}`)); err != nil {
			t.Fatalf("unmarshal patch: %v", err)
		}
		v, err := patch.Validate()
		if err != nil {
			t.Fatalf("validate patch: %v", err)
		}
		got, err := ent.PatchArticle(ctx, c, first.ID, v)
		if err != nil {
			t.Fatalf("PatchArticle: %v", err)
		}
		if got.Title != "beta II" {
			t.Errorf("patched title = %q, want %q", got.Title, "beta II")
		}
		if got.Rank != nil {
			t.Errorf("patched rank = %v, want nil (explicit null clears)", *got.Rank)
		}
		if got.Author == nil {
			t.Errorf("patched response lost its Author edge")
		}

		reread, err := ent.GetArticle(ctx, c, first.ID)
		if err != nil {
			t.Fatalf("GetArticle after patch: %v", err)
		}
		if reread.Title != "beta II" || reread.Rank != nil {
			t.Errorf("re-read = %+v, want title %q and a nil rank", reread, "beta II")
		}
	})

	t.Run("delete removes the row", func(t *testing.T) {
		if err := ent.DeleteArticle(ctx, c, first.ID); err != nil {
			t.Fatalf("DeleteArticle: %v", err)
		}
		if _, err := ent.GetArticle(ctx, c, first.ID); !ent.IsNotFound(err) {
			t.Errorf("GetArticle after delete: err = %v, want a not-found error", err)
		}
	})

	// The batch delete is the one Base<X>Service member the wiring did not
	// already supersede, so it is asserted here rather than assumed: it returns
	// ent's own affected-row count, and the rows it names are actually gone.
	t.Run("delete batch removes exactly the rows it names", func(t *testing.T) {
		keep := createArticle(t, ctx, c, "delta", ptr(4), author.ID)
		a := createArticle(t, ctx, c, "epsilon", ptr(5), author.ID)
		b := createArticle(t, ctx, c, "zeta", ptr(6), author.ID)

		n, err := ent.DeleteBatchArticles(ctx, c, []uuid.UUID{a.ID, b.ID})
		if err != nil {
			t.Fatalf("DeleteBatchArticles: %v", err)
		}
		if n != 2 {
			t.Errorf("DeleteBatchArticles deleted %d rows, want 2", n)
		}
		for _, gone := range []uuid.UUID{a.ID, b.ID} {
			if _, err := ent.GetArticle(ctx, c, gone); !ent.IsNotFound(err) {
				t.Errorf("GetArticle(%s) after batch delete: err = %v, want a not-found error", gone, err)
			}
		}
		if _, err := ent.GetArticle(ctx, c, keep.ID); err != nil {
			t.Errorf("the batch delete took a row it was not given: %v", err)
		}

		// An empty id list is a no-op, not "delete everything". ent turns an
		// empty IDIn into a predicate that matches nothing, and this asserts it
		// rather than trusting it — the failure mode is unrecoverable.
		n, err = ent.DeleteBatchArticles(ctx, c, nil)
		if err != nil {
			t.Fatalf("DeleteBatchArticles(nil): %v", err)
		}
		if n != 0 {
			t.Fatalf("DeleteBatchArticles(nil) deleted %d rows, want 0", n)
		}
		if _, err := ent.GetArticle(ctx, c, keep.ID); err != nil {
			t.Errorf("an empty batch delete removed rows: %v", err)
		}
	})
}

// TestWiringWithoutResponseEdges covers the other branch: Note declares no edge,
// so its response is built straight from the entity the builder returned.
func TestWiringWithoutResponseEdges(t *testing.T) {
	c, ctx := newClient(t)

	v, err := (&ent.NoteCreateRequest{Body: "first"}).Validate()
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	created, err := ent.CreateNote(ctx, c, v)
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if created.Body != "first" {
		t.Errorf("body = %q, want %q", created.Body, "first")
	}

	got, err := ent.GetNote(ctx, c, created.ID)
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("id = %s, want %s", got.ID, created.ID)
	}

	// body is Filterable but not Searchable, so the substring class is not on
	// this filter at all (ADR-0005). HasPrefix is the cheap-class operator, and
	// which operator narrows the page is incidental to what this asserts.
	p, err := ent.ListNotes(ctx, c, &ent.NoteFilter{BodyHasPrefix: ptr("fir")}, entapi.ListRequest{SortBy: "body"})
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if p.Total != 1 {
		t.Errorf("total = %d, want 1", p.Total)
	}
}

// TestEmptyPatchMeasuresEntUpdateDefault records the database operation rather
// than inferring it from generated code. Patchless has no HTTP-writable patch
// fields, but its Ent schema carries an UpdateDefault field.
func TestEmptyPatchMeasuresEntUpdateDefault(t *testing.T) {
	var sqlLog []string
	c, ctx := newClient(t, ent.Debug(), ent.Log(func(v ...any) {
		sqlLog = append(sqlLog, fmt.Sprint(v...))
	}))

	create, err := (&ent.PatchlessCreateRequest{}).Validate()
	if err != nil {
		t.Fatalf("validate create: %v", err)
	}
	created, err := ent.CreatePatchless(ctx, c, create)
	if err != nil {
		t.Fatalf("CreatePatchless: %v", err)
	}

	// Make a same-value result impossible at SQLite's stored timestamp
	// precision, then isolate the log to the patch operation itself.
	time.Sleep(time.Millisecond)
	sqlLog = nil
	var patch ent.PatchlessPatchRequest
	if err := patch.UnmarshalJSON([]byte(`{}`)); err != nil {
		t.Fatalf("unmarshal empty patch: %v", err)
	}
	valid, err := patch.Validate()
	if err != nil {
		t.Fatalf("validate empty patch: %v", err)
	}
	got, err := ent.PatchPatchless(ctx, c, created.ID, valid)
	if err != nil {
		t.Fatalf("PatchPatchless: %v", err)
	}

	updates := 0
	for _, line := range sqlLog {
		if strings.Contains(line, "UPDATE `patchlesses`") {
			updates++
		}
	}
	if updates != 1 {
		t.Fatalf("empty PatchPatchless issued %d UPDATE statements, want 1; log: %v", updates, sqlLog)
	}
	if !got.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("UpdateDefault result = %s, want after create value %s", got.UpdatedAt, created.UpdatedAt)
	}
	reread, err := ent.GetPatchless(ctx, c, created.ID)
	if err != nil {
		t.Fatalf("GetPatchless after empty patch: %v", err)
	}
	if !reread.UpdatedAt.Equal(got.UpdatedAt) {
		t.Fatalf("stored updated_at = %s, returned %s", reread.UpdatedAt, got.UpdatedAt)
	}
}

// TestConsumerReplacesOneOperation is the escape-hatch criterion: a consumer who
// needs different behaviour writes their own function and stops calling the
// generated one. Nothing has to be embedded, overridden or re-registered, and
// the operations they did not replace keep working.
func TestConsumerReplacesOneOperation(t *testing.T) {
	c, ctx := newClient(t)
	author := createAuthor(t, ctx, c, "ada")
	createArticle(t, ctx, c, "alpha", ptr(1), author.ID)
	createArticle(t, ctx, c, "beta", ptr(2), author.ID)

	// A consumer's own list operation: same generated predicates, converter and
	// eager-load plan, but a hard-coded ordering and a tenancy-style predicate
	// the generator has no way to know about.
	listMine := func(ctx context.Context, db *ent.Client, f *ent.ArticleFilter, r entapi.ListRequest) (*entapi.Page[ent.ArticleResponse], error) {
		q := ent.ArticleQueryWithResponseEdges(db.Article.Query())
		ps := append(f.Predicates(), article.AuthorID(author.ID))
		return entapi.ListPage(ctx, q, ps, []article.OrderOption{article.ByTitle()}, r, ent.NewArticleResponse)
	}

	p, err := listMine(ctx, c, &ent.ArticleFilter{}, entapi.ListRequest{})
	if err != nil {
		t.Fatalf("consumer list: %v", err)
	}
	if got, want := titles(p), []string{"alpha", "beta"}; !equal(got, want) {
		t.Errorf("consumer list = %v, want %v", got, want)
	}

	// The operations that were not replaced still work.
	if _, err := ent.GetAuthor(ctx, c, author.ID); err != nil {
		t.Fatalf("GetAuthor still has to work: %v", err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
