package softdeleteproof

import (
	"context"
	stdsql "database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/privacy"
	"github.com/google/uuid"
	"modernc.org/sqlite"

	// Aliased back to `ent`. The fixture package is named softdeleteent so that
	// no two packages in the generator's own module answer to the name
	// `ent` (#49);
	// a real consumer's generated package IS named `ent`, and these tests are
	// written from the consumer's side, so the alias keeps them reading the way
	// the code they stand in for does. Being spelled out, it is also nothing
	// goimports has to resolve.
	ent "github.com/githonllc/entapi/internal/fixtures/softdelete/softdeleteent"
	"github.com/githonllc/entapi/internal/fixtures/softdelete/softdeleteent/doc"
	"github.com/githonllc/entapi/internal/fixtures/softdelete/softdeleteent/draft"
	enttest "github.com/githonllc/entapi/internal/fixtures/softdelete/softdeleteent/enttest"
	"github.com/githonllc/entapi/internal/fixtures/softdelete/softdeleteent/ledger"
	"github.com/githonllc/entapi/internal/fixtures/softdelete/softdeleteent/note"
	_ "github.com/githonllc/entapi/internal/fixtures/softdelete/softdeleteent/runtime"
	entapi "github.com/githonllc/entapi/runtime"
)

func TestMain(m *testing.M) {
	// Ent's SQLite dialect name is "sqlite3"; modernc registers itself as
	// "sqlite". Register the same real driver under the name ent.Open requires.
	stdsql.Register(dialect.SQLite, &sqlite.Driver{})
	os.Exit(m.Run())
}

// newClient constructs a plain generated client. There is deliberately no
// soft-delete registration call: embedding the mixin must be the entire
// consumer-side opt-in.
func newClient(t *testing.T) (*ent.Client, *stdsql.DB, context.Context) {
	t.Helper()
	dsn := sqliteDSN(t, "new_client")
	db, err := stdsql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	c := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	t.Cleanup(func() { _ = c.Close() })
	ctx := privacy.DecisionContext(context.Background(), privacy.Allow)
	if err := c.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return c, db, ctx
}

// newOpenClient proves the generated Open path, which delegates to NewClient,
// receives exactly the same automatic configuration.
func newOpenClient(t *testing.T) (*ent.Client, *stdsql.DB, context.Context) {
	t.Helper()
	dsn := sqliteDSN(t, "open")
	db, err := stdsql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	c, err := ent.Open(dialect.SQLite, dsn)
	if err != nil {
		t.Fatalf("ent.Open: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := privacy.DecisionContext(context.Background(), privacy.Allow)
	if err := c.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return c, db, ctx
}

// newEnttestClient proves enttest.Open, the construction path consumers use
// most often in integration tests, receives the same automatic configuration.
func newEnttestClient(t *testing.T) (*ent.Client, *stdsql.DB, context.Context) {
	t.Helper()
	dsn := sqliteDSN(t, "enttest_open")
	db, err := stdsql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	c := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { _ = c.Close() })
	return c, db, privacy.DecisionContext(context.Background(), privacy.Allow)
}

func sqliteDSN(t *testing.T, suffix string) string {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return fmt.Sprintf("file:%s_%s?mode=memory&cache=shared&_fk=1", name, suffix)
}

// rowsOnDisk counts rows by raw SQL, below ent entirely. It is the only way to
// tell a soft delete from a hard one: through the client both look identical.
func rowsOnDisk(t *testing.T, db *stdsql.DB, table string, id uuid.UUID) int {
	t.Helper()
	var n int
	// #nosec G201 -- table is a generated ent constant, never user input.
	q := "SELECT COUNT(*) FROM " + table + " WHERE id = ?"
	if err := db.QueryRow(q, id).Scan(&n); err != nil {
		t.Fatalf("raw count on %s: %v", table, err)
	}
	return n
}

func newDoc(t *testing.T, c *ent.Client, ctx context.Context, title string) *ent.Doc {
	t.Helper()
	d, err := c.Doc.Create().SetTitle(title).Save(ctx)
	if err != nil {
		t.Fatalf("create doc %q: %v", title, err)
	}
	return d
}

// TestDirectClientQueryExcludesSoftDeleted is the load-bearing assertion of
// #18, and the reason none of this can live in a generated service.
//
// Nothing this project generates is in the call path below: DeleteOneID and
// Query are ent's own builders on ent's own client. If the row comes back, a
// consumer's `s.DB.Doc.Query()` returns tombstones and the feature is a lie —
// which is exactly what the pre-#18 service-layer implementation shipped.
func TestDirectClientQueryExcludesSoftDeleted(t *testing.T) {
	c, db, ctx := newClient(t)

	kept := newDoc(t, c, ctx, "kept")
	gone := newDoc(t, c, ctx, "gone")

	if err := c.Doc.DeleteOneID(gone.ID).Exec(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 1. It was a soft delete: the row is still there, below ent.
	if n := rowsOnDisk(t, db, doc.Table, gone.ID); n != 1 {
		t.Fatalf("row was hard-deleted: %d rows on disk for %s, want 1", n, gone.ID)
	}

	// 2. And it is invisible through every read path on the raw client.
	all, err := c.Doc.Query().All(ctx)
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(all) != 1 || all[0].ID != kept.ID {
		t.Fatalf("Doc.Query().All returned %d rows %v; want only the kept one (%s)", len(all), ids(all), kept.ID)
	}

	n, err := c.Doc.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("Doc.Query().Count = %d, want 1", n)
	}

	exists, err := c.Doc.Query().Where(doc.ID(gone.ID)).Exist(ctx)
	if err != nil {
		t.Fatalf("exist: %v", err)
	}
	if exists {
		t.Error("Doc.Query().Where(ID(deleted)).Exist = true, want false")
	}

	if _, err := c.Doc.Get(ctx, gone.ID); !ent.IsNotFound(err) {
		t.Errorf("Doc.Get(deleted) error = %v, want a NotFoundError", err)
	}
}

// TestPlainNewClientInstallsSoftDelete proves there is no unregistered-client
// concept left: plain NewClient installs the generated hook and interceptor.
func TestPlainNewClientInstallsSoftDelete(t *testing.T) {
	c, db, ctx := newClient(t)

	d := newDoc(t, c, ctx, "gone")
	if err := c.Doc.DeleteOneID(d.ID).Exec(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if got := rowsOnDisk(t, db, doc.Table, d.ID); got != 1 {
		t.Fatalf("plain NewClient hard-deleted the row: %d rows on disk, want 1", got)
	}
	all, err := c.Doc.Query().All(ctx)
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("plain NewClient returned %d soft-deleted rows, want 0", len(all))
	}
}

// TestEveryConstructionPathInstallsSoftDelete pins the partial's placement in
// newConfig: NewClient, Open and enttest.Open must all receive it without any
// ordering dependency or consumer registration call.
func TestEveryConstructionPathInstallsSoftDelete(t *testing.T) {
	constructors := map[string]func(*testing.T) (*ent.Client, *stdsql.DB, context.Context){
		"NewClient":    newClient,
		"Open":         newOpenClient,
		"enttest.Open": newEnttestClient,
	}
	for name, construct := range constructors {
		t.Run(name, func(t *testing.T) {
			c, db, ctx := construct(t)
			d := newDoc(t, c, ctx, name)
			if err := c.Doc.DeleteOneID(d.ID).Exec(ctx); err != nil {
				t.Fatalf("delete: %v", err)
			}
			if got := rowsOnDisk(t, db, doc.Table, d.ID); got != 1 {
				t.Fatalf("row was hard-deleted: %d rows on disk, want 1", got)
			}
			if exists, err := c.Doc.Query().Where(doc.ID(d.ID)).Exist(ctx); err != nil {
				t.Fatalf("query: %v", err)
			} else if exists {
				t.Fatal("soft-deleted row is visible")
			}
		})
	}
}

// TestMultipleClientsInstallSoftDeleteIndependently proves newConfig attaches
// fresh hook/interceptor slices to every instance rather than mutating a
// process-global client or only configuring the first one.
func TestMultipleClientsInstallSoftDeleteIndependently(t *testing.T) {
	clients := make([]*ent.Client, 0, 2)
	dbs := make([]*stdsql.DB, 0, 2)
	ctxs := make([]context.Context, 0, 2)
	for i := range 2 {
		dsn := sqliteDSN(t, fmt.Sprintf("client_%d", i))
		db, err := stdsql.Open("sqlite", dsn)
		if err != nil {
			t.Fatalf("open client %d database: %v", i, err)
		}
		t.Cleanup(func() { _ = db.Close() })
		c := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
		t.Cleanup(func() { _ = c.Close() })
		ctx := privacy.DecisionContext(context.Background(), privacy.Allow)
		if err := c.Schema.Create(ctx); err != nil {
			t.Fatalf("migrate client %d: %v", i, err)
		}
		clients = append(clients, c)
		dbs = append(dbs, db)
		ctxs = append(ctxs, ctx)
	}

	for i, c := range clients {
		d := newDoc(t, c, ctxs[i], fmt.Sprintf("client %d", i))
		if err := c.Doc.DeleteOneID(d.ID).Exec(ctxs[i]); err != nil {
			t.Fatalf("delete through client %d: %v", i, err)
		}
		if got := rowsOnDisk(t, dbs[i], doc.Table, d.ID); got != 1 {
			t.Errorf("client %d hard-deleted its row: %d rows on disk, want 1", i, got)
		}
		if exists, err := c.Doc.Query().Where(doc.ID(d.ID)).Exist(ctxs[i]); err != nil {
			t.Fatalf("query through client %d: %v", i, err)
		} else if exists {
			t.Errorf("client %d returned its soft-deleted row", i)
		}
	}
}

func TestTransactionSoftDeletePersistsTombstone(t *testing.T) {
	c, db, ctx := newClient(t)
	d := newDoc(t, c, ctx, "transactional")

	tx, err := c.Tx(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	if err := tx.Doc.DeleteOneID(d.ID).Exec(ctx); err != nil {
		_ = tx.Rollback()
		t.Fatalf("delete through transaction: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}

	var physicalRows, tombstones int
	// #nosec G201 -- table and column are generated ent constants, never user input.
	query := "SELECT COUNT(*), COUNT(" + doc.FieldDeletedAt + ") FROM " + doc.Table + " WHERE id = ?"
	if err := db.QueryRow(query, d.ID).Scan(&physicalRows, &tombstones); err != nil {
		t.Fatalf("inspect tombstone below ent: %v", err)
	}
	if physicalRows != 1 || tombstones != 1 {
		t.Fatalf("raw tombstone counts = (%d, %d), want (1, 1)", physicalRows, tombstones)
	}

	all, err := c.Doc.Query().All(ctx)
	if err != nil {
		t.Fatalf("query after transactional delete: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("bare query returned %d soft-deleted rows, want 0", len(all))
	}
}

// TestDenyByDefaultPrivacyCoexistsWithSoftDelete proves both halves of the
// interaction: the fixture really denies an undecided call, while an allowed
// delete can re-dispatch its tombstone update without privacy rejecting the
// hook's internal mutation.
func TestDenyByDefaultPrivacyCoexistsWithSoftDelete(t *testing.T) {
	c, db, allowed := newClient(t)
	if _, err := c.Doc.Create().SetTitle("denied").Save(context.Background()); !errors.Is(err, privacy.Deny) {
		t.Fatalf("create with no privacy decision returned %v, want privacy.Deny", err)
	}
	if _, err := c.Doc.Query().All(context.Background()); !errors.Is(err, privacy.Deny) {
		t.Fatalf("query with no privacy decision returned %v, want privacy.Deny", err)
	}

	d := newDoc(t, c, allowed, "allowed")
	if err := c.Doc.DeleteOneID(d.ID).Exec(allowed); err != nil {
		t.Fatalf("soft delete under an allowed privacy context: %v", err)
	}
	if got := rowsOnDisk(t, db, doc.Table, d.ID); got != 1 {
		t.Fatalf("privacy-compatible delete removed the row: %d rows on disk, want 1", got)
	}
	if exists, err := c.Doc.Query().Where(doc.ID(d.ID)).Exist(allowed); err != nil {
		t.Fatalf("query under an allowed privacy context: %v", err)
	} else if exists {
		t.Fatal("privacy-compatible query returned the soft-deleted row")
	}
}

// TestEscapeHatchIncludesSoftDeleted proves the documented way back in
// actually returns the tombstones, and that it is scoped to the context it is
// applied to rather than to the client.
func TestEscapeHatchIncludesSoftDeleted(t *testing.T) {
	c, _, ctx := newClient(t)

	gone := newDoc(t, c, ctx, "gone")
	if err := c.Doc.DeleteOneID(gone.ID).Exec(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}

	withDeleted := entapi.WithSoftDeleted(ctx)
	all, err := c.Doc.Query().All(withDeleted)
	if err != nil {
		t.Fatalf("query all with soft-deleted: %v", err)
	}
	if len(all) != 1 || all[0].ID != gone.ID {
		t.Fatalf("WithSoftDeleted returned %d rows %v; want the tombstone %s", len(all), ids(all), gone.ID)
	}
	if all[0].DeletedAt == nil {
		t.Error("the returned tombstone has a nil deleted_at; the hook did not stamp it")
	}

	// The plain context is unchanged — the hatch is per-call, not a mode.
	rest, err := c.Doc.Query().All(ctx)
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("the original context still filters: got %d rows, want 0", len(rest))
	}
}

// TestWithHardDeleteRemovesTheRow proves the other direction: without it a
// soft-deleted row could never be purged, and "soft delete" would mean
// "storage grows forever".
func TestWithHardDeleteRemovesTheRow(t *testing.T) {
	c, db, ctx := newClient(t)

	d := newDoc(t, c, ctx, "purge me")
	if err := c.Doc.DeleteOneID(d.ID).Exec(entapi.WithHardDelete(ctx)); err != nil {
		t.Fatalf("hard delete: %v", err)
	}
	if n := rowsOnDisk(t, db, doc.Table, d.ID); n != 0 {
		t.Errorf("WithHardDelete left %d rows on disk, want 0", n)
	}
}

// TestEagerLoadedEdgeExcludesSoftDeleted closes the hole a per-call filter
// would leave: a deleted Doc must not reappear through its parent Note.
func TestEagerLoadedEdgeExcludesSoftDeleted(t *testing.T) {
	c, _, ctx := newClient(t)

	n, err := c.Note.Create().SetBody("parent").Save(ctx)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	kept, err := c.Doc.Create().SetTitle("kept").SetNote(n).Save(ctx)
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	gone, err := c.Doc.Create().SetTitle("gone").SetNote(n).Save(ctx)
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	if err := c.Doc.DeleteOneID(gone.ID).Exec(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}

	loaded, err := c.Note.Query().Where(note.ID(n.ID)).WithDocs().Only(ctx)
	if err != nil {
		t.Fatalf("query note with docs: %v", err)
	}
	if len(loaded.Edges.Docs) != 1 || loaded.Edges.Docs[0].ID != kept.ID {
		t.Fatalf("eager-loaded docs = %d rows %v; want only %s", len(loaded.Edges.Docs), ids(loaded.Edges.Docs), kept.ID)
	}
}

func newDraft(t *testing.T, c *ent.Client, ctx context.Context, headline string, d *ent.Doc) *ent.Draft {
	t.Helper()
	dr, err := c.Draft.Create().SetHeadline(headline).SetDoc(d).Save(ctx)
	if err != nil {
		t.Fatalf("create draft %q: %v", headline, err)
	}
	return dr
}

func draftIDs(drafts []*ent.Draft) []uuid.UUID {
	out := make([]uuid.UUID, len(drafts))
	for i, d := range drafts {
		out[i] = d.ID
	}
	return out
}

// TestRequiredExpandedEdgeToSoftDeletedTarget is #100, reproduced against a real
// database rather than argued from the templates.
//
// Draft.doc is Unique, Required and api.Expand()ed, and Doc is soft-deletable.
// The schema says every Draft has a Doc and the foreign key still says so after
// the delete — the row is on disk. So this is the one place the two halves of
// soft delete are visible at once, and each half is a separate assertion:
//
//   - the traverser reaches the EAGER-LOAD sub-query, not only top-level
//     queries. templates/softdelete.tmpl claims this; before #100 nothing
//     proved it for a to-one edge, and a compile proof cannot: the difference
//     between "the predicate is generated" and "the predicate reaches the
//     sub-query SQL" is invisible to the type checker.
//   - soft delete does NOT cascade. The Draft is still there, and a consumer
//     who never asked about Docs still lists it.
//
// The two together are what the reporter saw as `"user": null` on a row that
// would not go away.
func TestRequiredExpandedEdgeToSoftDeletedTarget(t *testing.T) {
	c, _, ctx := newClient(t)

	docKept := newDoc(t, c, ctx, "kept")
	docGone := newDoc(t, c, ctx, "gone")
	draftKept := newDraft(t, c, ctx, "keeps its doc", docKept)
	draftGone := newDraft(t, c, ctx, "loses its doc", docGone)

	if err := c.Doc.DeleteOneID(docGone.ID).Exec(ctx); err != nil {
		t.Fatalf("soft delete doc: %v", err)
	}

	// (a) The eager load really is filtered. DocOrErr must report
	// loaded-and-absent — a NotFoundError — and NOT a NotLoadedError, which is
	// what a caller would get if WithDoc had simply been forgotten.
	loaded, err := c.Draft.Query().Where(draft.ID(draftGone.ID)).WithDoc().Only(ctx)
	if err != nil {
		t.Fatalf("query draft with its doc: %v", err)
	}
	if loaded.Edges.Doc != nil {
		t.Fatalf("eager-loaded doc = %v; the traverser did not reach the eager-load sub-query", loaded.Edges.Doc.ID)
	}
	if _, err := loaded.Edges.DocOrErr(); !ent.IsNotFound(err) {
		t.Fatalf("DocOrErr on a draft whose required doc is soft-deleted = %v (IsNotLoaded=%t); want a NotFoundError", err, ent.IsNotLoaded(err))
	}

	// The positive control for that discriminator: without WithDoc the very
	// same nil edge field must report NOT LOADED. Without this, (a) would pass
	// for a build in which every unloaded edge reports not-found.
	unloaded, err := c.Draft.Query().Where(draft.ID(draftGone.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("query draft without its doc: %v", err)
	}
	if _, err := unloaded.Edges.DocOrErr(); !ent.IsNotLoaded(err) {
		t.Fatalf("DocOrErr on a draft queried without WithDoc = %v; want a NotLoadedError, so that (a)'s NotFoundError means something", err)
	}

	// (b) The generated response constructor therefore succeeds and emits null,
	// which is exactly what openapi.yaml documents for an expanded edge
	// (oneOf [DocSummary, null]). The bug report is not a contract violation.
	resp, err := ent.NewDraftResponse(loaded)
	if err != nil {
		t.Fatalf("NewDraftResponse on a loaded-and-absent required edge: %v; want no error", err)
	}
	if resp.Doc != nil {
		t.Fatalf("NewDraftResponse doc = %+v; want nil, i.e. JSON null", resp.Doc)
	}

	// (c) Soft delete does not cascade: the owning row is untouched and still
	// listed. This is the complaint that no annotation and no generated code
	// answers today.
	all, err := c.Draft.Query().All(ctx)
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("Draft.Query().All returned %d rows %v; want both drafts — soft delete must not cascade", len(all), draftIDs(all))
	}

	// (d) And the recipe that excludes it is plain ent, because an edge
	// predicate is an ordinary SQL sub-query and carries no traverser of its
	// own. Asserting the KEPT draft survives matters as much as the gone one
	// disappearing: a predicate that excluded everything would pass otherwise.
	live, err := c.Draft.Query().Where(draft.HasDocWith(doc.DeletedAtIsNil())).All(ctx)
	if err != nil {
		t.Fatalf("list drafts with a live doc: %v", err)
	}
	if len(live) != 1 || live[0].ID != draftKept.ID {
		t.Fatalf("HasDocWith(DeletedAtIsNil()) returned %d rows %v; want only %s (draft %s must be excluded)", len(live), draftIDs(live), draftKept.ID, draftGone.ID)
	}
}

// TestHardDeleteEntitiesAreUnaffected covers both halves of "a schema without
// the mixin is unchanged":
//
//   - Note has no deleted_at at all.
//   - Ledger has one, named exactly what the retired convention keyed on,
//     Optional so ent generates DeletedAtIsNil for it — and no mixin. Under the
//     old rule one extra .Nillable() would have silently enrolled it. Under the
//     annotation rule it is an ordinary entity, and this is the assertion that
//     says so behaviourally rather than by reading the generated source.
func TestHardDeleteEntitiesAreUnaffected(t *testing.T) {
	c, db, ctx := newClient(t)

	n, err := c.Note.Create().SetBody("note").Save(ctx)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := c.Note.DeleteOneID(n.ID).Exec(ctx); err != nil {
		t.Fatalf("delete note: %v", err)
	}
	if got := rowsOnDisk(t, db, note.Table, n.ID); got != 0 {
		t.Errorf("Note was soft-deleted: %d rows on disk, want 0", got)
	}

	l, err := c.Ledger.Create().SetEntry("entry").Save(ctx)
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := c.Ledger.DeleteOneID(l.ID).Exec(ctx); err != nil {
		t.Fatalf("delete ledger: %v", err)
	}
	if got := rowsOnDisk(t, db, ledger.Table, l.ID); got != 0 {
		t.Errorf("Ledger has a deleted_at column but no mixin, and was soft-deleted anyway: %d rows on disk, want 0", got)
	}
	rest, err := c.Ledger.Query().All(ctx)
	if err != nil {
		t.Fatalf("query ledgers: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("Ledger query returned %d rows after a hard delete, want 0", len(rest))
	}
}

// TestGeneratedBatchDeleteRoutesThroughTheHook is acceptance criterion 5 for
// the batch delete, which is the half that used to live on the generated
// BaseDocService and now lives in the wiring (#29):
//
//	DeleteBatchDocs -> db.Doc.Delete().Where(doc.IDIn(ids...)).Exec(ctx)
//
// That is OpDelete where DeleteDoc is OpDeleteOne, and softDeleteHook fires on
// `OpDelete|OpDeleteOne`. Reading those two facts together says it must work.
// It is asserted anyway, because the hook does not merely observe the mutation
// — it calls SetOp(OpUpdate) and re-runs it, and the value that comes back out
// of Exec is produced by a different builder than the one the caller invoked.
//
// So the RETURNED COUNT is asserted separately from the rows on disk. A batch
// delete that tombstones three rows and reports 0 would satisfy every other
// assertion in this file while silently breaking the one thing the count is
// for, which is telling a caller how many of the ids existed.
func TestGeneratedBatchDeleteRoutesThroughTheHook(t *testing.T) {
	c, db, ctx := newClient(t)

	kept := newDoc(t, c, ctx, "kept")
	a := newDoc(t, c, ctx, "a")
	b := newDoc(t, c, ctx, "b")

	n, err := ent.DeleteBatchDocs(ctx, c, []uuid.UUID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("DeleteBatchDocs: %v", err)
	}
	if n != 2 {
		t.Errorf("DeleteBatchDocs reported %d rows, want 2 — the count survives the OpDelete -> OpUpdate rewrite or it does not", n)
	}

	for _, id := range []uuid.UUID{a.ID, b.ID} {
		if got := rowsOnDisk(t, db, doc.Table, id); got != 1 {
			t.Errorf("DeleteBatchDocs hard-deleted %s: %d rows on disk, want 1", id, got)
		}
	}

	left, err := c.Doc.Query().All(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(left) != 1 || left[0].ID != kept.ID {
		t.Errorf("after the batch delete the client returns %v, want only %s", ids(left), kept.ID)
	}

	// Batching the same ids again matches nothing: the rewritten update carries
	// the same DeletedAtIsNil predicate the read side uses, so an already
	// tombstoned row is not re-stamped and is not counted.
	n, err = ent.DeleteBatchDocs(ctx, c, []uuid.UUID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("second DeleteBatchDocs: %v", err)
	}
	if n != 0 {
		t.Errorf("re-deleting two already soft-deleted rows reported %d, want 0", n)
	}

	// An entity WITHOUT the mixin really loses its rows to the same call shape.
	// Without this, "the rows are still there" would be indistinguishable from
	// a batch delete that never deleted anything.
	l1, err := c.Ledger.Create().SetEntry("one").Save(ctx)
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	l2, err := c.Ledger.Create().SetEntry("two").Save(ctx)
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	n, err = ent.DeleteBatchLedgers(ctx, c, []uuid.UUID{l1.ID, l2.ID})
	if err != nil {
		t.Fatalf("DeleteBatchLedgers: %v", err)
	}
	if n != 2 {
		t.Errorf("DeleteBatchLedgers reported %d rows, want 2", n)
	}
	for _, id := range []uuid.UUID{l1.ID, l2.ID} {
		if got := rowsOnDisk(t, db, ledger.Table, id); got != 0 {
			t.Errorf("DeleteBatchLedgers left %s on disk: %d rows, want 0", id, got)
		}
	}
}

func ids(docs []*ent.Doc) []uuid.UUID {
	out := make([]uuid.UUID, len(docs))
	for i, d := range docs {
		out[i] = d.ID
	}
	return out
}

// TestGeneratedWiringRoutesThroughTheHook is criterion 5 again, for the other
// generated delete. #28's wiring emits
// `DeleteDoc → db.Doc.DeleteOneID(id).Exec(ctx)`, which is an ordinary ent
// builder and therefore runs the client's hooks — but "therefore" is reasoning,
// and the whole point of this module is that reasoning is not the standard here.
//
// The read half matters as much: GetDoc and ListDocs go through
// DocQueryWithResponseEdges(db.Doc.Query()), so they must not surface a
// tombstone either.
func TestGeneratedWiringRoutesThroughTheHook(t *testing.T) {
	c, db, ctx := newClient(t)

	kept := newDoc(t, c, ctx, "kept")
	gone := newDoc(t, c, ctx, "gone")

	if err := ent.DeleteDoc(ctx, c, gone.ID); err != nil {
		t.Fatalf("DeleteDoc: %v", err)
	}
	if n := rowsOnDisk(t, db, doc.Table, gone.ID); n != 1 {
		t.Errorf("DeleteDoc hard-deleted the row: %d on disk, want 1", n)
	}

	if _, err := ent.GetDoc(ctx, c, gone.ID); err == nil {
		t.Error("GetDoc returned a soft-deleted row")
	}

	// A second DeleteDoc of an already-tombstoned row is a not-found, not a
	// silent re-stamp: the rewritten update carries the same predicate the read
	// side uses.
	if err := ent.DeleteDoc(ctx, c, gone.ID); err == nil {
		t.Error("deleting an already soft-deleted row succeeded; want a not-found error")
	}

	page, err := ent.ListDocs(ctx, c, nil, entapi.ListRequest{})
	if err != nil {
		t.Fatalf("ListDocs: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != kept.ID {
		t.Fatalf("ListDocs returned %d items, want only %s", len(page.Data), kept.ID)
	}

	// And the wiring delete on an entity WITHOUT the mixin still really deletes.
	l, err := c.Ledger.Create().SetEntry("entry").Save(ctx)
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := ent.DeleteLedger(ctx, c, l.ID); err != nil {
		t.Fatalf("DeleteLedger: %v", err)
	}
	if n := rowsOnDisk(t, db, ledger.Table, l.ID); n != 0 {
		t.Errorf("DeleteLedger left %d rows on disk, want 0", n)
	}
}
