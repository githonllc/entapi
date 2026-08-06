// This file is HAND-WRITTEN and lives in package ent on purpose, like
// edges/ent/orerr_contract_test.go. It is the behavioural half of #27: the
// generated filter and sort artifacts are asserted through what they DO, not
// through substrings of the source that produced them.
//
// It has to be in package ent because that is where the generated artifacts
// land, and it has to avoid a database because this module deliberately carries
// no SQL driver. Neither is a limitation here: an ent predicate and an ent
// order option are both just functions over a *sql.Selector, so the SQL they
// produce can be read back directly — which is a stronger statement about what
// reaches the query than any assertion about the generated Go source.
//
// `go build` ignores test files, so the codegen harness is unaffected; `go test
// ./...` from the repository root compiles and runs this.
package queryent

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql"

	"github.com/githonllc/entdomain/internal/fixtures/query/queryent/predicate"
	"github.com/githonllc/entdomain/internal/fixtures/query/queryent/record"
	entdomain "github.com/githonllc/entdomain/runtime"
)

// selectorSQL applies fns to a fresh selector over the record table and returns
// the SQL it renders. This is the query — not a proxy for it.
func selectorSQL(fns ...func(*sql.Selector)) string {
	s := sql.Select("*").From(sql.Table(record.Table))
	for _, fn := range fns {
		fn(s)
	}
	query, _ := s.Query()
	return query
}

func orderFns(opts []record.OrderOption) []func(*sql.Selector) {
	out := make([]func(*sql.Selector), 0, len(opts))
	for _, o := range opts {
		out = append(out, o)
	}
	return out
}

func predicateFns(ps []predicate.Record) []func(*sql.Selector) {
	out := make([]func(*sql.Selector), 0, len(ps))
	for _, p := range ps {
		out = append(out, p)
	}
	return out
}

// ────────────────────────────────────────────────────────────────────────────
// The sort allow-list. This is the security half of #27: an unchecked sort
// field is an injection site, an unindexed-scan trigger and — combined with
// paging — an ordering oracle over columns the caller was never meant to read.
// ────────────────────────────────────────────────────────────────────────────

// TestSortAllowListIsExactlyTheAnnotatedFields pins both directions: only
// Sortable fields are in it, and every Sortable field is.
func TestSortAllowListIsExactlyTheAnnotatedFields(t *testing.T) {
	want := []string{"title", "created_at"}
	if !reflect.DeepEqual(RecordSortKeys, want) {
		t.Errorf("RecordSortKeys = %v, want %v", RecordSortKeys, want)
	}
}

// TestUnmarkedEntityHasAnEmptySortAllowList is the safe end of the default. An
// entity nobody marked is orderable by nothing at all, rather than by
// everything the preset builders happened to touch.
func TestUnmarkedEntityHasAnEmptySortAllowList(t *testing.T) {
	if len(PlainSortKeys) != 0 {
		t.Errorf("PlainSortKeys = %v, want empty: Plain marks no field sortable", PlainSortKeys)
	}
	if n := reflect.TypeOf(PlainFilter{}).NumField(); n != 0 {
		t.Errorf("PlainFilter has %d field(s), want 0: Plain marks no field filterable or searchable", n)
	}
}

// TestSortRejectsAnythingOutsideTheAllowList covers the ways a caller can be
// wrong: a column that exists but is not annotated, one annotated for input
// only, one that does not exist, and a payload that is not a column name.
func TestSortRejectsAnythingOutsideTheAllowList(t *testing.T) {
	for _, key := range []string{
		"secret",                       // exists, input-only, never queryable
		"note",                         // exists, query-scoped, but not marked sortable
		"body",                         // exists, searchable, but not marked sortable
		"status",                       // exists, filterable, but not marked sortable
		"id",                           // exists, and is not in the allow-list either
		"nonexistent",                  // does not exist
		"created_at; DROP TABLE users", // not a column name
		"created_at) OR 1=1--",         // not a column name
		"(SELECT secret FROM records)", // not a column name
	} {
		t.Run(key, func(t *testing.T) {
			opts, err := RecordOrder(entdomain.ListRequest{SortBy: key})

			if !errors.Is(err, entdomain.ErrValidation) {
				t.Fatalf("RecordOrder(SortBy=%q) error = %v, want one wrapping entdomain.ErrValidation", key, err)
			}
			if len(opts) != 0 {
				t.Fatalf("RecordOrder(SortBy=%q) returned %d order option(s) alongside the error", key, len(opts))
			}

			// The load-bearing assertion: nothing the caller wrote reaches the
			// query. Rendering the selector the rejected options would have
			// been applied to states that about the SQL, not about the source.
			query := selectorSQL(orderFns(opts)...)
			if strings.Contains(query, key) {
				t.Fatalf("caller-supplied sort key %q reached the query: %s", key, query)
			}
			if strings.Contains(strings.ToUpper(query), "ORDER BY") {
				t.Fatalf("a rejected sort still produced an ORDER BY: %s", query)
			}
		})
	}
}

// TestSortAcceptsAnAllowedKeyInBothDirections shows the allow-list is a
// checkpoint rather than a wall, and that the column reaching SQL is ent's own
// constant rather than anything the caller spelled.
//
// It also pins ADR-0002's tiebreak: the requested column comes first and the
// primary key follows it, in the SAME direction, so a descending walk stays
// descending all the way down. created_at is not unique, so without that second
// term the page boundaries cut through ties nondeterministically.
func TestSortAcceptsAnAllowedKeyInBothDirections(t *testing.T) {
	for _, tc := range []struct {
		order   string
		wantDsc bool
	}{
		{order: ""},
		{order: "asc"},
		{order: "desc", wantDsc: true},
		{order: "DESC", wantDsc: true},
	} {
		t.Run("created_at/"+tc.order, func(t *testing.T) {
			opts, err := RecordOrder(entdomain.ListRequest{SortBy: "created_at", Order: tc.order})
			if err != nil {
				t.Fatalf("RecordOrder: unexpected error %v", err)
			}
			if len(opts) != 2 {
				t.Fatalf("got %d order options, want 2 (the requested column plus the primary-key tiebreak)", len(opts))
			}
			query := selectorSQL(orderFns(opts)...)
			at := strings.Index(query, record.FieldCreatedAt)
			if at < 0 {
				t.Fatalf("ORDER BY does not name %q: %s", record.FieldCreatedAt, query)
			}
			if id := strings.Index(query[at:], "`"+record.FieldID+"`"); id < 0 {
				t.Errorf("ORDER BY does not append %q after %q: %s", record.FieldID, record.FieldCreatedAt, query)
			}
			// Both terms carry the direction, or neither does — a tiebreak that
			// disagreed with its primary term would reverse inside every group
			// of equal keys.
			wantDesc := 0
			if tc.wantDsc {
				wantDesc = 2
			}
			if got := strings.Count(query, "DESC"); got != wantDesc {
				t.Errorf("order %q produced %d DESC term(s), want %d: %s", tc.order, got, wantDesc, query)
			}
		})
	}
}

// TestNoSortRequestedOrdersByID pins ADR-0002's determinism floor. The policy
// this replaced — "no sort requested means no ORDER BY" — was not a neutral
// default: LIMIT/OFFSET over an unordered result set lets rows repeat or vanish
// between pages with zero concurrent writes, so offset pagination was simply
// incorrect without it.
//
// There is still no default sort COLUMN, which is the policy the schema does
// not contain. The primary key is not a guess: it is unique, so ordering by it
// makes the result total, and it is the only term returned here — the response
// claims no ordering the caller did not request, it merely stops being random.
func TestNoSortRequestedOrdersByID(t *testing.T) {
	opts, err := RecordOrder(entdomain.ListRequest{})
	if err != nil {
		t.Fatalf("RecordOrder: unexpected error %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("got %d order options for an empty request, want exactly 1 (the primary key)", len(opts))
	}

	query := selectorSQL(orderFns(opts)...)
	want := selectorSQL(orderFns([]record.OrderOption{record.ByID(sql.OrderAsc())})...)
	if query != want {
		t.Errorf("unsorted request rendered\n  %s\nwant the primary-key ascending term alone\n  %s", query, want)
	}
	if strings.Contains(query, "DESC") {
		t.Errorf("the determinism floor must be ascending: %s", query)
	}
	for _, column := range []string{record.FieldTitle, record.FieldCreatedAt} {
		if strings.Contains(query, column) {
			t.Errorf("unsorted request ordered by %q, which the caller never asked for: %s", column, query)
		}
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Operator coverage. The counts are ent's own, from entc/gen/func.go fieldOps
// plus the sql driver's extra Ops; they are restated here so a change to the
// derivation shows up as a number rather than as a diff nobody reads.
// ────────────────────────────────────────────────────────────────────────────

func TestOperatorCoverageIsTheFullSetEntDerives(t *testing.T) {
	want := map[string]int{
		"title":      13, // stringOps (11) + EqualFold + ContainsFold
		"status":     4,  // enumOps
		"score":      9,  // numericOps (8) + IsNil/NotNil collapsed into one
		"created_at": 8,  // numericOps
		"q":          1,  // the free-text input: one for the whole entity
	}

	got := map[string]int{}
	rt := reflect.TypeOf(RecordFilter{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("form")
		if tag == "" {
			t.Fatalf("RecordFilter.%s carries no form tag", rt.Field(i).Name)
		}
		matched := false
		for key := range want {
			if tag == key || strings.HasPrefix(tag, key+"_") {
				got[key]++
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("RecordFilter.%s (form:%q) belongs to no field expected to be filterable", rt.Field(i).Name, tag)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter parameters per field = %v, want %v", got, want)
	}
}

// TestUnmarkedFieldsAreAbsentFromTheFilter is the converse: a field that is not
// marked contributes nothing, whatever its type or scope.
func TestUnmarkedFieldsAreAbsentFromTheFilter(t *testing.T) {
	rt := reflect.TypeOf(RecordFilter{})
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		for _, forbidden := range []string{"Note", "Secret", "Body", "ID"} {
			if strings.HasPrefix(name, forbidden) {
				t.Errorf("RecordFilter.%s exists, but %q is not marked filterable", name, forbidden)
			}
		}
	}
}

// TestNullQuestionIsOneParameter pins the one deliberate departure from "one
// parameter per operator": IsNil and NotNil are one boolean question, and two
// parameters would admit a request that contradicts itself.
func TestNullQuestionIsOneParameter(t *testing.T) {
	rt := reflect.TypeOf(RecordFilter{})
	if _, ok := rt.FieldByName("ScoreIsNull"); !ok {
		t.Fatal("RecordFilter has no ScoreIsNull field")
	}
	for _, absent := range []string{"ScoreIsNil", "ScoreNotNil", "ScoreNotNull"} {
		if _, ok := rt.FieldByName(absent); ok {
			t.Errorf("RecordFilter.%s exists; the null pair must collapse to one parameter", absent)
		}
	}

	isNull, notNull := true, false
	if q := selectorSQL(predicateFns((&RecordFilter{ScoreIsNull: &isNull}).Predicates())...); !strings.Contains(q, "IS NULL") {
		t.Errorf("score_is_null=true did not produce IS NULL: %s", q)
	}
	if q := selectorSQL(predicateFns((&RecordFilter{ScoreIsNull: &notNull}).Predicates())...); !strings.Contains(q, "NOT NULL") {
		t.Errorf("score_is_null=false did not produce IS NOT NULL: %s", q)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Composition: filters AND, free-text ORs within itself and ANDs with the rest.
// ────────────────────────────────────────────────────────────────────────────

func TestFiltersCombineConjunctively(t *testing.T) {
	title := "release"
	status := record.StatusLive
	f := &RecordFilter{TitleContains: &title, Status: &status}

	query := selectorSQL(predicateFns(f.Predicates())...)
	for _, want := range []string{record.FieldTitle, record.FieldStatus, "AND"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %s does not contain %q", query, want)
		}
	}
	if strings.Contains(query, " OR ") {
		t.Errorf("two independent filters were combined disjunctively: %s", query)
	}
}

func TestFreeTextSearchOrsWithinItselfAndAndsWithTheRest(t *testing.T) {
	term := "release"
	status := record.StatusLive
	f := &RecordFilter{Q: &term, Status: &status}

	query := selectorSQL(predicateFns(f.Predicates())...)
	if !strings.Contains(query, " OR ") {
		t.Errorf("free-text search did not produce a disjunction: %s", query)
	}
	for _, want := range []string{record.FieldTitle, record.FieldBody} {
		if !strings.Contains(query, want) {
			t.Errorf("free-text search does not cover searchable field %q: %s", want, query)
		}
	}
	if !strings.Contains(query, " AND ") {
		t.Errorf("the free-text disjunction was not ANDed with the remaining filters: %s", query)
	}
	if !strings.Contains(query, record.FieldStatus) {
		t.Errorf("the non-search filter was dropped: %s", query)
	}
	// body is searchable but NOT filterable, so it may only appear inside the
	// disjunction — never as a filter parameter of its own.
	if _, ok := reflect.TypeOf(RecordFilter{}).FieldByName("Body"); ok {
		t.Error("RecordFilter.Body exists; body is searchable, not filterable")
	}
}

func TestEmptyFilterProducesNoPredicates(t *testing.T) {
	if ps := (&RecordFilter{}).Predicates(); len(ps) != 0 {
		t.Errorf("an empty filter produced %d predicate(s)", len(ps))
	}
	var nilFilter *RecordFilter
	if ps := nilFilter.Predicates(); len(ps) != 0 {
		t.Errorf("a nil filter produced %d predicate(s)", len(ps))
	}
	blank := ""
	if ps := (&RecordFilter{Q: &blank}).Predicates(); len(ps) != 0 {
		t.Errorf("an empty free-text term produced %d predicate(s)", len(ps))
	}
}
