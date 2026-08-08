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
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/google/uuid"

	"github.com/githonllc/entapi/internal/fixtures/query/queryent/predicate"
	"github.com/githonllc/entapi/internal/fixtures/query/queryent/record"
	entapi "github.com/githonllc/entapi/runtime"
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
	want := []string{"id", "title", "created_at"}
	if !reflect.DeepEqual(RecordSortKeys, want) {
		t.Errorf("RecordSortKeys = %v, want %v", RecordSortKeys, want)
	}
}

// TestUnmarkedEntityGetsOnlyThePrimaryKeyQuerySurface pins the annotation-free
// ID behaviour without exposing any unmarked non-ID field.
func TestUnmarkedEntityGetsOnlyThePrimaryKeyQuerySurface(t *testing.T) {
	if want := []string{"id"}; !reflect.DeepEqual(PlainSortKeys, want) {
		t.Errorf("PlainSortKeys = %v, want %v: the primary key is naturally sortable", PlainSortKeys, want)
	}
	if n := reflect.TypeOf(PlainFilter{}).NumField(); n != 8 {
		t.Errorf("PlainFilter has %d field(s), want 8 primary-key operator slots", n)
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
		"nonexistent",                  // does not exist
		"created_at; DROP TABLE users", // not a column name
		"created_at) OR 1=1--",         // not a column name
		"(SELECT secret FROM records)", // not a column name
	} {
		t.Run(key, func(t *testing.T) {
			opts, err := RecordOrder(entapi.ListRequest{Sort: []entapi.SortSpec{{Key: key}}})

			if !errors.Is(err, entapi.ErrValidation) {
				t.Fatalf("RecordOrder(Sort=%q) error = %v, want one wrapping entapi.ErrValidation", key, err)
			}
			if len(opts) != 0 {
				t.Fatalf("RecordOrder(Sort=%q) returned %d order option(s) alongside the error", key, len(opts))
			}
			for _, legal := range RecordSortKeys {
				if !strings.Contains(err.Error(), legal) {
					t.Errorf("error %q does not list legal sort key %q", err, legal)
				}
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
		name    string
		wantDsc bool
	}{
		{name: "asc"},
		{name: "desc", wantDsc: true},
	} {
		t.Run("created_at/"+tc.name, func(t *testing.T) {
			opts, err := RecordOrder(entapi.ListRequest{Sort: []entapi.SortSpec{{Key: "created_at", Desc: tc.wantDsc}}})
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
				t.Errorf("order %q produced %d DESC term(s), want %d: %s", tc.name, got, wantDesc, query)
			}
		})
	}
}

func TestMultiKeySortHonoursEveryTermAndTiebreakAnywhere(t *testing.T) {
	opts, err := RecordOrder(entapi.ListRequest{Sort: []entapi.SortSpec{
		{Key: "created_at", Desc: true},
		{Key: "id"},
		{Key: "title", Desc: true},
	}})
	if err != nil {
		t.Fatalf("RecordOrder: %v", err)
	}
	if len(opts) != 3 {
		t.Fatalf("got %d terms, want the three requested terms and no duplicate id tiebreak", len(opts))
	}
	query := selectorSQL(orderFns(opts)...)
	createdAt := strings.Index(query, "`"+record.FieldCreatedAt+"` DESC")
	id := strings.Index(query, "`"+record.FieldID+"`")
	title := strings.Index(query, "`"+record.FieldTitle+"` DESC")
	if createdAt < 0 || id < createdAt || title < id {
		t.Fatalf("multi-key order is not created_at desc, id asc, title desc: %s", query)
	}
	if strings.Count(query, "`"+record.FieldID+"`") != 1 {
		t.Fatalf("id was appended twice even though it appeared in the middle: %s", query)
	}
}

func TestPrimaryKeyIsNaturallySortable(t *testing.T) {
	opts, err := RecordOrder(entapi.ListRequest{Sort: []entapi.SortSpec{{Key: "id", Desc: true}}})
	if err != nil {
		t.Fatalf("RecordOrder(id): %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("RecordOrder(id) returned %d terms, want exactly one", len(opts))
	}
	query := selectorSQL(orderFns(opts)...)
	if !strings.Contains(query, "`"+record.FieldID+"` DESC") {
		t.Fatalf("id descending did not reach SQL: %s", query)
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
	opts, err := RecordOrder(entapi.ListRequest{})
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

// TestOperatorCoverageFollowsTheClassRule pins ADR-0005. Existence is still
// ent's answer alone — the class rule is a filter over which of the operators
// ent derived becomes a URL parameter, never a second operator table.
//
// title is Filterable AND Searchable, so it carries the full set. ref is
// Filterable ONLY, so it carries the cheap class and the five expensive-class
// operators are absent by name: `LIKE '%x%'` defeats the index exactly like
// the unchecked sort the allow-list above exists to prevent, and no annotation
// on ref ever asked for it.
func TestOperatorCoverageFollowsTheClassRule(t *testing.T) {
	want := map[string]int{
		"id":         8,  // numericOps; the primary key is naturally Filterable
		"title":      12, // stringOps plus ContainsFold, minus EqualFold (no wire spelling)
		"reference":  9,  // Go field Ref; every wire surface follows StorageKey
		"status":     4,  // enumOps
		"score":      9,  // numericOps (8) + IsNil/NotNil collapsed into one
		"created_at": 8,  // numericOps
		"_q":         1,  // the free-text input: one for the whole entity
	}

	got := map[string]int{}
	rt := reflect.TypeOf(RecordFilter{})
	for i := 0; i < rt.NumField(); i++ {
		if form := rt.Field(i).Tag.Get("form"); form != "" {
			t.Errorf("RecordFilter.%s retains retired form tag %q", rt.Field(i).Name, form)
		}
		tag := rt.Field(i).Tag.Get("json")
		if tag == "" {
			t.Fatalf("RecordFilter.%s carries no json tag", rt.Field(i).Name)
		}
		tag = strings.TrimSuffix(tag, ",omitempty")
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

	// The counts say how many; these say which. A count alone would pass if the
	// split had merely renamed a parameter rather than withheld a class.
	for _, present := range []string{"TitleContains", "TitleContainsFold", "TitleHasSuffix"} {
		if _, ok := rt.FieldByName(present); !ok {
			t.Errorf("RecordFilter.%s is missing; title is Filterable AND Searchable, which is what earns the substring class", present)
		}
	}
	if _, ok := rt.FieldByName("TitleEqualFold"); ok {
		t.Error("RecordFilter.TitleEqualFold exists; EqualFold has no wire spelling and must fall out of the vocabulary intersection")
	}
	for _, absent := range []string{"RefContains", "RefContainsFold", "RefEqualFold", "RefHasSuffix"} {
		if _, ok := rt.FieldByName(absent); ok {
			t.Errorf("RecordFilter.%s exists; ref is Filterable only, and the substring class requires api.Searchable() (ADR-0005)", absent)
		}
	}

	// And the cheap class is exactly what remains, in ent's own operator order.
	wantRef := []string{"Ref", "RefNEQ", "RefIn", "RefNotIn", "RefGT", "RefGTE", "RefLT", "RefLTE", "RefIsNull"}
	var gotRef []string
	for i := 0; i < rt.NumField(); i++ {
		if name := rt.Field(i).Name; strings.HasPrefix(name, "Ref") {
			gotRef = append(gotRef, name)
		}
	}
	if !reflect.DeepEqual(gotRef, wantRef) {
		t.Errorf("ref parameters = %v, want exactly the cheap class %v", gotRef, wantRef)
	}
}

// TestUnmarkedFieldsAreAbsentFromTheFilter is the converse: a field that is not
// marked contributes nothing, whatever its type or scope.
func TestUnmarkedFieldsAreAbsentFromTheFilter(t *testing.T) {
	rt := reflect.TypeOf(RecordFilter{})
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		for _, forbidden := range []string{"Note", "Secret", "Body"} {
			if strings.HasPrefix(name, forbidden) {
				t.Errorf("RecordFilter.%s exists, but %q is not marked filterable", name, forbidden)
			}
		}
	}
	if _, ok := rt.FieldByName("ID"); !ok {
		t.Error("RecordFilter.ID is absent; the primary key must be naturally Filterable")
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

	if q := selectorSQL(predicateFns((&RecordFilter{ScoreIsNull: []bool{true}}).Predicates())...); !strings.Contains(q, "IS NULL") {
		t.Errorf("score_is_null=true did not produce IS NULL: %s", q)
	}
	if q := selectorSQL(predicateFns((&RecordFilter{ScoreIsNull: []bool{false}}).Predicates())...); !strings.Contains(q, "NOT NULL") {
		t.Errorf("score_is_null=false did not produce IS NOT NULL: %s", q)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Composition: filters AND, free-text ORs within itself and ANDs with the rest.
// ────────────────────────────────────────────────────────────────────────────

func TestFiltersCombineConjunctively(t *testing.T) {
	f := &RecordFilter{TitleContains: []string{"release"}, Status: []record.Status{record.StatusLive}}

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
	f := &RecordFilter{Q: &term, Status: []record.Status{record.StatusLive}}

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

func parseRecord(t *testing.T, values url.Values) (*RecordFilter, entapi.ListRequest) {
	t.Helper()
	f, r, err := ParseRecordQuery(values)
	if err != nil {
		t.Fatalf("ParseRecordQuery(%v): %v", values, err)
	}
	return f, r
}

func requireQueryValidation(t *testing.T, values url.Values, wants ...string) {
	t.Helper()
	_, _, err := ParseRecordQuery(values)
	if !errors.Is(err, entapi.ErrValidation) {
		t.Fatalf("ParseRecordQuery(%v) error = %v, want ErrValidation", values, err)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestParseRule1BareEmptyIsIgnoredButExplicitEqIsReal(t *testing.T) {
	f, _ := parseRecord(t, url.Values{"title": {"", "eq:"}})
	if !reflect.DeepEqual(f.Title, []string{""}) {
		t.Fatalf("Title = %#v, want one explicit empty equality", f.Title)
	}
}

func TestParseRule2BareValueMeansEquality(t *testing.T) {
	f, _ := parseRecord(t, url.Values{"title": {"release"}})
	if !reflect.DeepEqual(f.Title, []string{"release"}) {
		t.Fatalf("Title = %#v, want bare equality", f.Title)
	}
	query := selectorSQL(predicateFns(f.Predicates())...)
	if strings.Contains(query, "LIKE") || !strings.Contains(query, "=") {
		t.Fatalf("bare equality rendered as a pattern operator: %s", query)
	}
}

func TestParseRule3AllowedPrefixAppliesItsOperator(t *testing.T) {
	f, _ := parseRecord(t, url.Values{"score": {"gt:30"}})
	if !reflect.DeepEqual(f.ScoreGT, []int{30}) {
		t.Fatalf("ScoreGT = %#v, want [30]", f.ScoreGT)
	}
}

func TestParseRule4KnownButDisallowedPrefixIsValidationError(t *testing.T) {
	requireQueryValidation(t, url.Values{"reference": {"like:scan"}}, "reference", "like:scan", "legal operators", "between")
}

func TestParseFilterableOnlyPrefixIsValidationError(t *testing.T) {
	requireQueryValidation(t, url.Values{"reference": {"prefix:left"}}, "reference", "prefix:left", "legal operators")
}

func TestParseRule5UnknownPrefixFallsBackToWholeEqualityLiteral(t *testing.T) {
	f, _ := parseRecord(t, url.Values{"title": {"12:30"}})
	if !reflect.DeepEqual(f.Title, []string{"12:30"}) {
		t.Fatalf("Title = %#v, want the whole value as equality literal", f.Title)
	}
	requireQueryValidation(t, url.Values{"score": {"12:30"}}, "score", "12:30")
}

func TestParseRule6ExplicitEqEscapesOperatorLookingLiterals(t *testing.T) {
	f, _ := parseRecord(t, url.Values{"title": {"eq:like:scan"}})
	if !reflect.DeepEqual(f.Title, []string{"like:scan"}) {
		t.Fatalf("Title = %#v, want [like:scan]", f.Title)
	}
}

func TestParseRecordQueryCoversEveryOperatorClass(t *testing.T) {
	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	f, r := parseRecord(t, url.Values{
		"id":         {"in:" + id1.String() + "," + id2.String()},
		"title":      {"ne:x", "gt:a", "ge:b", "lt:y", "le:z", "like:mid", "ilike:FOLD", "prefix:pre", "suffix:suf"},
		"status":     {"in:draft,live", "not_in:draft"},
		"score":      {"is_null:", "not_null:", "from:10", "to:90", "between:20,80"},
		"created_at": {"eq:2026-08-08T12:34:56Z"},
		"_q":         {"needle"},
		"_sort":      {"created_at:desc,id"},
		"_page":      {"2"},
		"_size":      {"1001"},
	})
	if len(f.IDIn) != 1 || !reflect.DeepEqual(f.IDIn[0], []uuid.UUID{id1, id2}) {
		t.Errorf("IDIn = %#v", f.IDIn)
	}
	if len(f.TitleNEQ) != 1 || len(f.TitleGT) != 1 || len(f.TitleGTE) != 1 || len(f.TitleLT) != 1 || len(f.TitleLTE) != 1 || len(f.TitleContains) != 1 || len(f.TitleContainsFold) != 1 || len(f.TitleHasPrefix) != 1 || len(f.TitleHasSuffix) != 1 {
		t.Errorf("string operator slots were not all populated: %+v", f)
	}
	if !reflect.DeepEqual(f.ScoreIsNull, []bool{true, false}) || !reflect.DeepEqual(f.ScoreGTE, []int{10, 20}) || !reflect.DeepEqual(f.ScoreLTE, []int{90, 80}) {
		t.Errorf("null/range slots = null:%v gte:%v lte:%v", f.ScoreIsNull, f.ScoreGTE, f.ScoreLTE)
	}
	wantTime := time.Date(2026, 8, 8, 12, 34, 56, 0, time.UTC)
	if !reflect.DeepEqual(f.CreatedAt, []time.Time{wantTime}) {
		t.Errorf("CreatedAt = %v, want %v", f.CreatedAt, wantTime)
	}
	if f.Q == nil || *f.Q != "needle" || r.Page != 2 || r.Size != 1001 || len(r.Sort) != 2 {
		t.Errorf("reserved params = Q:%v request:%+v", f.Q, r)
	}
	if len(f.Predicates()) != 20 {
		t.Errorf("Predicates() = %d, want 20 independent predicates", len(f.Predicates()))
	}
}

func TestRepeatedFilterParamsAndMerge(t *testing.T) {
	f, _ := parseRecord(t, url.Values{
		"score":  {"gt:30", "le:50"},
		"status": {"eq:draft", "eq:live"},
	})
	if len(f.Predicates()) != 4 {
		t.Fatalf("Predicates() = %d, want four ANDed occurrences", len(f.Predicates()))
	}
	query := selectorSQL(predicateFns(f.Predicates())...)
	if strings.Count(query, " AND ") < 3 || strings.Count(query, record.FieldStatus) != 2 {
		t.Fatalf("repeated filters did not remain independent AND predicates: %s", query)
	}
}

func TestBareWildcardsRemainEqualityLiterals(t *testing.T) {
	f, _ := parseRecord(t, url.Values{"title": {"*literal?"}})
	query := selectorSQL(predicateFns(f.Predicates())...)
	if strings.Contains(query, "LIKE") {
		t.Fatalf("bare wildcards were translated to LIKE: %s", query)
	}
}

func TestBetweenRequiresTwoValuesAndDoesNotReorder(t *testing.T) {
	requireQueryValidation(t, url.Values{"score": {"between:1"}}, "score", "between:1", "exactly two")
	requireQueryValidation(t, url.Values{"score": {"between:1,2,3"}}, "score", "between:1,2,3", "exactly two")
	f, _ := parseRecord(t, url.Values{"score": {"between:50,30"}})
	if !reflect.DeepEqual(f.ScoreGTE, []int{50}) || !reflect.DeepEqual(f.ScoreLTE, []int{30}) {
		t.Fatalf("between reordered endpoints: gte=%v lte=%v", f.ScoreGTE, f.ScoreLTE)
	}
}

func TestParseRecordQueryValidationPaths(t *testing.T) {
	cases := []struct {
		name   string
		values url.Values
		wants  []string
	}{
		{name: "numeric", values: url.Values{"score": {"gt:abc"}}, wants: []string{"score", "gt:abc"}},
		{name: "enum", values: url.Values{"status": {"eq:missing"}}, wants: []string{"status", "missing", "draft", "live"}},
		{name: "uuid", values: url.Values{"id": {"eq:not-a-uuid"}}, wants: []string{"id", "not-a-uuid"}},
		{name: "size non-numeric", values: url.Values{"_size": {"many"}}, wants: []string{"_size", "many", "integer"}},
		{name: "size zero", values: url.Values{"_size": {"0"}}, wants: []string{"_size", "0", "count-only"}},
		{name: "size negative", values: url.Values{"_size": {"-1"}}, wants: []string{"_size", "-1"}},
		{name: "page non-numeric", values: url.Values{"_page": {"next"}}, wants: []string{"_page", "next", "integer"}},
		{name: "page zero", values: url.Values{"_page": {"0"}}, wants: []string{"_page", "0"}},
		{name: "page negative", values: url.Values{"_page": {"-1"}}, wants: []string{"_page", "-1"}},
		{name: "sort direction", values: url.Values{"_sort": {"created_at:sideways"}}, wants: []string{"_sort", "sideways", "asc", "desc"}},
		{name: "null operator value", values: url.Values{"score": {"is_null:true"}}, wants: []string{"score", "is_null:true", "with a value"}},
		{name: "unknown field", values: url.Values{"unknown": {"x"}}, wants: []string{"unknown"}},
		{name: "Go name is not a wire alias", values: url.Values{"ref": {"prefix:x"}}, wants: []string{"ref", "unknown"}},
		{name: "non-filterable field", values: url.Values{"body": {"x"}}, wants: []string{"body", "not Filterable"}},
		{name: "unknown reserved", values: url.Values{"_limit": {"10"}}, wants: []string{"_limit"}},
		{name: "duplicate reserved", values: url.Values{"_sort": {"id", "title"}}, wants: []string{"_sort", "exactly once"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { requireQueryValidation(t, tc.values, tc.wants...) })
	}
}

func TestQOnResourceWithoutSearchableFieldsIsValidationError(t *testing.T) {
	_, _, err := ParsePlainQuery(url.Values{"_q": {"scan"}})
	if !errors.Is(err, entapi.ErrValidation) || !strings.Contains(err.Error(), "_q") || !strings.Contains(err.Error(), "Searchable") {
		t.Fatalf("ParsePlainQuery(_q) error = %v", err)
	}
}

func TestUnknownSortFieldIsRejectedOnlyByRecordOrder(t *testing.T) {
	_, request := parseRecord(t, url.Values{"_sort": {"missing"}})
	if _, err := RecordOrder(request); !errors.Is(err, entapi.ErrValidation) || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("RecordOrder error = %v", err)
	}
}

func TestParsedIDSortDoesNotAppendATiebreak(t *testing.T) {
	_, request := parseRecord(t, url.Values{"_sort": {"created_at,id"}})
	opts, err := RecordOrder(request)
	if err != nil {
		t.Fatalf("RecordOrder: %v", err)
	}
	query := selectorSQL(orderFns(opts)...)
	if strings.Count(query, "`"+record.FieldID+"`") != 1 {
		t.Fatalf("id was appended despite already appearing in _sort: %s", query)
	}
}

func TestQueryKeysAreVisitedInSortedOrder(t *testing.T) {
	requireQueryValidation(t, url.Values{"z_unknown": {"x"}, "a_unknown": {"y"}}, "a_unknown")
}
