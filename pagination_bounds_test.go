package entdomain

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// TestPageSizeCeilingHasExactlyOneHome is the whole of criterion 6 of #24.
//
// Before this change the ceiling was stated twice and the two disagreed:
//
//	types.go:10  MaxPageSize = 1000
//	types.go:17  validate:"omitempty,min=1,max=100"
//
// A third-party tag validator would reject size=500 while Validate() and
// Limit() both accept it. A struct tag cannot reference a constant, so the tag
// can never be made to *refer* to MaxPageSize — it can only restate it and
// drift. MaxPageSize is therefore the single home, and the tag must not carry a
// numeric ceiling at all. Limit() clamps to it; Validate() rejects above it.
func TestPageSizeCeilingHasExactlyOneHome(t *testing.T) {
	f, ok := reflect.TypeOf(ListRequest{}).FieldByName("Size")
	if !ok {
		t.Fatal("ListRequest has no Size field")
	}
	tag := f.Tag.Get("validate")
	for _, clause := range strings.Split(tag, ",") {
		if strings.HasPrefix(strings.TrimSpace(clause), "max=") {
			t.Fatalf("validate tag %q restates the page-size ceiling in %q; "+
				"MaxPageSize (=%d) is the single home and a tag cannot refer to it",
				tag, clause, MaxPageSize)
		}
	}
}

// TestCeilingDecidedInOnePlace checks the two code paths that enforce the
// bound both read MaxPageSize rather than a literal of their own.
func TestCeilingDecidedInOnePlace(t *testing.T) {
	t.Run("Limit clamps to MaxPageSize", func(t *testing.T) {
		r := ListRequest{Size: MaxPageSize + 1}
		if got := r.Limit(); got != MaxPageSize {
			t.Fatalf("Limit() = %d, want %d", got, MaxPageSize)
		}
	})

	t.Run("exactly MaxPageSize is allowed by both", func(t *testing.T) {
		r := ListRequest{Size: MaxPageSize}
		if got := r.Limit(); got != MaxPageSize {
			t.Fatalf("Limit() = %d, want %d", got, MaxPageSize)
		}
		if err := r.Validate(); err != nil {
			t.Fatalf("Validate() rejected the ceiling itself: %v", err)
		}
	})

	t.Run("Validate rejects above MaxPageSize", func(t *testing.T) {
		r := ListRequest{Size: MaxPageSize + 1}
		if err := r.Validate(); err == nil {
			t.Fatal("Validate() accepted a size above the ceiling")
		}
	})
}

func TestListRequestLimitAndOffset(t *testing.T) {
	cases := []struct {
		name             string
		req              ListRequest
		wantLim, wantOff int
	}{
		{"unset falls back to the default", ListRequest{}, DefaultPageSize, 0},
		{"zero size falls back to the default", ListRequest{Size: 0, Page: 3}, DefaultPageSize, 2 * DefaultPageSize},
		{"negative size falls back to the default", ListRequest{Size: -5}, DefaultPageSize, 0},
		{"oversized is clamped", ListRequest{Size: 10_000}, MaxPageSize, 0},
		{"page 0 and page 1 are the first page", ListRequest{Size: 10, Page: 0}, 10, 0},
		{"negative page is the first page", ListRequest{Size: 10, Page: -3}, 10, 0},
		{"page 2 skips one page", ListRequest{Size: 10, Page: 2}, 10, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.req.Limit(); got != c.wantLim {
				t.Errorf("Limit() = %d, want %d", got, c.wantLim)
			}
			if got := c.req.Offset(); got != c.wantOff {
				t.Errorf("Offset() = %d, want %d", got, c.wantOff)
			}
		})
	}
}

// --- a query builder shaped like ent's, with no ent in sight ---------------
//
// Query is an interface over the *shape* of an ent builder, so a fake that
// satisfies it exercises ListPage end to end without a database. It also
// doubles as proof that the constraint is satisfiable by something other than
// the one builder the fixture happens to use.

type fakeRow struct{ n int }

type fakeView struct{ n int }

type fakeQuery struct {
	rows     []*fakeRow
	limit    int
	limitSet bool
	offset   int
}

func (q fakeQuery) Where(...string) fakeQuery { return q }
func (q fakeQuery) Order(...string) fakeQuery { return q }

func (q fakeQuery) Limit(n int) fakeQuery {
	q.limit, q.limitSet = n, true
	return q
}

func (q fakeQuery) Offset(n int) fakeQuery {
	q.offset = n
	return q
}

// All mimics SQL LIMIT/OFFSET, including the part that matters here: a LIMIT of
// zero returns no rows, it does not return everything.
func (q fakeQuery) All(context.Context) ([]*fakeRow, error) {
	rows := q.rows
	if q.offset >= len(rows) {
		return nil, nil
	}
	rows = rows[q.offset:]
	if q.limitSet && q.limit >= 0 && q.limit < len(rows) {
		rows = rows[:q.limit]
	}
	return rows, nil
}

func (q fakeQuery) Count(context.Context) (int, error) { return len(q.rows), nil }

func newFakeQuery(n int) fakeQuery {
	rows := make([]*fakeRow, n)
	for i := range rows {
		rows[i] = &fakeRow{n: i}
	}
	return fakeQuery{rows: rows}
}

func toFakeView(r *fakeRow) (*fakeView, error) { return &fakeView{n: r.n}, nil }

// TestListPageSurvivesDegenerateLimits carries forward the v1 regression
// recorded on #6: the generated lister panicked on a non-empty table when the
// limit was zero — entities[:0] followed by entities[len(entities)-1]. The
// template that hosted it is going away, but ListPage is the new host of the
// same computation, so the case is pinned here against a non-empty table.
func TestListPageSurvivesDegenerateLimits(t *testing.T) {
	ctx := context.Background()
	q := newFakeQuery(7)

	cases := []struct {
		name    string
		req     ListRequest
		wantLen int
	}{
		{"zero limit", ListRequest{Size: 0}, 7},
		{"negative limit", ListRequest{Size: -1}, 7},
		{"negative page", ListRequest{Size: 2, Page: -4}, 2},
		{"page past the end", ListRequest{Size: 2, Page: 99}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page, err := ListPage(ctx, q, nil, nil, c.req, toFakeView)
			if err != nil {
				t.Fatalf("ListPage: %v", err)
			}
			if len(page.Data) != c.wantLen {
				t.Fatalf("len(Data) = %d, want %d", len(page.Data), c.wantLen)
			}
			if page.Total != 7 {
				t.Errorf("Total = %d, want 7", page.Total)
			}
			if page.Page < 1 {
				t.Errorf("Page = %d, want a 1-based page number", page.Page)
			}
			if page.Size != c.req.Limit() {
				t.Errorf("Size = %d, want the effective limit %d", page.Size, c.req.Limit())
			}
			if page.Data == nil {
				t.Error("Data must be an empty slice, not nil, so it marshals as []")
			}
		})
	}
}

func TestListPageAppliesTheEffectiveBound(t *testing.T) {
	ctx := context.Background()
	page, err := ListPage(ctx, newFakeQuery(5_000), nil, nil, ListRequest{Size: 10_000}, toFakeView)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != MaxPageSize {
		t.Fatalf("len(Data) = %d, want the clamped %d", len(page.Data), MaxPageSize)
	}
	if page.Size != MaxPageSize {
		t.Fatalf("Size = %d, want %d", page.Size, MaxPageSize)
	}
}
