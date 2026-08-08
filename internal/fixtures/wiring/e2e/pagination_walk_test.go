// Offset pagination is only correct over a TOTAL order (#59, ADR-0002). A
// prefix order that leaves ties — or no order at all — lets the engine return
// rows in a different physical order per query, so walking the pages can hand
// the same row out twice and never hand out another, with zero concurrent
// writes.
//
// This file walks EVERY page of a table larger than one page and diffs the
// union against what was inserted. It does that twice: with no sort requested
// at all, and with a sort on a column whose value is identical in every row —
// the two shapes ADR-0002 identifies. Each walk asserts two things:
//
//   - coverage: the union is exactly the inserted set, with no repeats;
//   - determinism: the rows come back in ascending primary-key order, which is
//     the tiebreak the generated {Entity}Order now appends.
//
// The determinism assertion is the one that fails deterministically before the
// fix. Coverage is the property that actually matters, but an engine is free to
// satisfy it by accident on any given day — a test that only asserts coverage
// would go green on a broken generator whenever SQLite happened to scan in
// rowid order, which is most of the time. Asserting the order states the
// contract instead of hoping the violation shows up.
package e2e

import (
	"context"
	"testing"

	"github.com/google/uuid"

	ent "github.com/githonllc/entapi/internal/fixtures/wiring/wiringent"
	entapi "github.com/githonllc/entapi/runtime"
)

// walkPages drives the generated list wiring page by page until it runs out of
// rows, returning the ids in the order they were served. It is deliberately the
// generated entry point rather than a hand-built query: the defect lived in
// what {Entity}Order handed ListPage, so anything that bypasses it proves
// nothing.
func walkPages(t *testing.T, ctx context.Context, c *ent.Client, r entapi.ListRequest) []uuid.UUID {
	t.Helper()

	var served []uuid.UUID
	for page := 1; ; page++ {
		r.Page = page
		p, err := ent.ListArticles(ctx, c, nil, r)
		if err != nil {
			t.Fatalf("ListArticles(page=%d): %v", page, err)
		}
		if len(p.Data) == 0 {
			return served
		}
		for _, a := range p.Data {
			served = append(served, a.ID)
		}
		if len(served) > p.Total {
			// Not a guard against an infinite loop so much as the failure
			// itself: more rows served than the table holds means at least one
			// was served twice.
			t.Fatalf("served %d rows after page %d, table holds %d", len(served), page, p.Total)
		}
	}
}

// assertWalkIsTotal is the whole contract in one place: every inserted row
// appears exactly once, and the sequence is primary-key ascending.
func assertWalkIsTotal(t *testing.T, what string, served []uuid.UUID, inserted map[uuid.UUID]bool) {
	t.Helper()

	seen := make(map[uuid.UUID]bool, len(served))
	for _, id := range served {
		if seen[id] {
			t.Errorf("%s: row %s was served twice", what, id)
		}
		seen[id] = true
		if !inserted[id] {
			t.Errorf("%s: row %s was served but never inserted", what, id)
		}
	}
	for id := range inserted {
		if !seen[id] {
			t.Errorf("%s: row %s was never served — the walk lost it", what, id)
		}
	}
	if len(served) != len(inserted) {
		t.Errorf("%s: walk served %d rows, %d were inserted", what, len(served), len(inserted))
	}

	for i := 1; i < len(served); i++ {
		if served[i-1].String() >= served[i].String() {
			t.Fatalf("%s: ids are not primary-key ascending at position %d: %s then %s — "+
				"the order the pages were cut on is not total",
				what, i, served[i-1], served[i])
		}
	}
}

// TestPaginationWalkServesEveryRowExactlyOnce is #59's behavioural proof.
//
// 2*DefaultPageSize+1 rows is the smallest size that needs a third page, so a
// boundary that shifts between queries has two places to lose a row rather than
// one.
func TestPaginationWalkServesEveryRowExactlyOnce(t *testing.T) {
	c, ctx := newClient(t)
	author := createAuthor(t, ctx, c, "ada")

	const rows = 2*entapi.DefaultPageSize + 1
	inserted := make(map[uuid.UUID]bool, rows)
	for i := 0; i < rows; i++ {
		// One title for every row: the sort key below is then a pure prefix
		// order with rows ties and nothing to break them but the primary key.
		got := createArticle(t, ctx, c, "same", ptr(i), author.ID)
		inserted[got.ID] = true
	}
	if len(inserted) != rows {
		t.Fatalf("inserted %d distinct ids, want %d", len(inserted), rows)
	}

	t.Run("no sort requested", func(t *testing.T) {
		assertWalkIsTotal(t, "unsorted walk", walkPages(t, ctx, c, entapi.ListRequest{}), inserted)
	})

	t.Run("sorted by a non-unique column", func(t *testing.T) {
		for _, desc := range []bool{false, true} {
			name := "asc"
			if desc {
				name = "desc"
			}
			t.Run(name, func(t *testing.T) {
				served := walkPages(t, ctx, c, entapi.ListRequest{Sort: []entapi.SortSpec{{Key: "title", Desc: desc}}})
				if desc {
					// The tiebreak follows the requested direction, so reverse
					// before asserting the same ascending invariant.
					for i, j := 0, len(served)-1; i < j; i, j = i+1, j-1 {
						served[i], served[j] = served[j], served[i]
					}
				}
				assertWalkIsTotal(t, "walk sorted by title "+name, served, inserted)
			})
		}
	})
}
