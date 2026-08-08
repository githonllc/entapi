package entapi

import (
	"reflect"
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// paramNames projects filterParamsNamed onto the struct field names it
// produces, so a failure names identifiers a reader can grep for.
func paramNames(ps []filterParam) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Field
	}
	return out
}

func paramTags(ps []filterParam) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Tag
	}
	return out
}

// TestFilterParamsFollowEntsOperatorTable is the unit-level statement of the
// rule the fixture proves end to end: existence is ent's, in ent's order, and
// the substring class is filtered out of it unless the field is Searchable
// (ADR-0005, #64). The two string rows are the same ent operator table read
// through the two marker combinations.
//
// The expectations here are the ones reachable without a graph — gen.Field.Ops
// consults the storage driver only for a field bound to a config, so EqualFold
// and ContainsFold appear in the generated fixture rather than here. That gap
// is why internal/fixtures/query exists; this test pins the shape, that one
// pins the count.
func TestFilterParamsFollowEntsOperatorTable(t *testing.T) {
	tests := []struct {
		name  string
		field *gen.Field
		want  []string
	}{
		{
			// Filterable only: the cheap class. HasPrefix, HasSuffix and Contains
			// are operators ent derived and this field did not earn.
			name:  "string/filterable",
			field: newStringField("title", fieldPtr(api.Filterable())),
			want: []string{
				"Title", "TitleNEQ", "TitleIn", "TitleNotIn",
				"TitleGT", "TitleGTE", "TitleLT", "TitleLTE",
			},
		},
		{
			// The same type, plus api.Searchable(): the substring class returns.
			name:  "string/filterable+searchable",
			field: newStringField("title", fieldPtr(api.FieldAnnotation{Filterable: true, Searchable: true})),
			want: []string{
				"Title", "TitleNEQ", "TitleIn", "TitleNotIn",
				"TitleGT", "TitleGTE", "TitleLT", "TitleLTE",
				"TitleContains", "TitleHasPrefix", "TitleHasSuffix",
			},
		},
		{
			name:  "time",
			field: newTimeField("created_at", nil),
			want: []string{
				"CreatedAt", "CreatedAtNEQ", "CreatedAtIn", "CreatedAtNotIn",
				"CreatedAtGT", "CreatedAtGTE", "CreatedAtLT", "CreatedAtLTE",
			},
		},
		{
			name:  "enum",
			field: newField("status", &field.TypeInfo{Type: field.TypeEnum, Ident: "Status"}, nil),
			want:  []string{"Status", "StatusNEQ", "StatusIn", "StatusNotIn"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paramNames(filterParamsNamed(tt.field, tt.field.StructField()))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterParamsNamed(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestFilterParamsCollapseTheNullPair pins the one deliberate departure from
// one-parameter-per-operator. Two parameters would admit "is null AND is not
// null", which has no answer.
func TestFilterParamsCollapseTheNullPair(t *testing.T) {
	f := newIntField("score", nil)
	f.Optional = true

	ps := filterParamsNamed(f, f.StructField())
	names := paramNames(ps)

	for _, absent := range []string{"ScoreIsNil", "ScoreNotNil"} {
		for _, got := range names {
			if got == absent {
				t.Errorf("filterParams emitted %s; IsNil and NotNil must collapse into one parameter", absent)
			}
		}
	}

	last := ps[len(ps)-1]
	if last.Field != "ScoreIsNull" || last.Tag != "score_is_null" || last.Type != "[]bool" || last.Kind != "null" {
		t.Errorf("collapsed null parameter = %+v, want {ScoreIsNull score_is_null []bool null}", last)
	}
	if last.Pred != "" {
		t.Errorf("collapsed null parameter names a single predicate %q; it needs both IsNil and NotNil", last.Pred)
	}
}

// TestFilterParamShapes pins the three emitted statement shapes, because the
// template branches on Kind and a wrong Kind is a compile error in a
// consumer's package rather than here.
func TestFilterParamShapes(t *testing.T) {
	f := newStringField("title", fieldPtr(api.FieldAnnotation{Filterable: true, Searchable: true}))
	ps := filterParamsNamed(f, f.StructField())
	byName := map[string]filterParam{}
	for _, p := range ps {
		byName[p.Field] = p
	}

	if got := byName["Title"]; got.Kind != "one" || got.Type != "[]string" || got.Pred != "TitleEQ" || got.Tag != "title" {
		t.Errorf("equality parameter = %+v, want {Title title []string TitleEQ one}", got)
	}
	if got := byName["TitleIn"]; got.Kind != "many" || got.Type != "[][]string" || got.Pred != "TitleIn" || got.Tag != "title_in" {
		t.Errorf("In parameter = %+v, want a variadic slice parameter", got)
	}
	if got := byName["TitleHasPrefix"]; got.Tag != "title_prefix" {
		t.Errorf("HasPrefix tag = %q, want %q", got.Tag, "title_prefix")
	}
}

// TestFilterParamTagsUseTheStorageKey guards the wire names against being
// derived from the Go field name, which would rename a query parameter the
// moment a schema author set a custom storage key.
func TestFilterParamTagsUseTheStorageKey(t *testing.T) {
	f := newStringField("created_by", nil)
	tags := paramTags(filterParamsNamed(f, f.StructField()))
	if len(tags) == 0 {
		t.Fatal("no parameters for a string field")
	}
	if tags[0] != "created_by" {
		t.Errorf("equality tag = %q, want %q", tags[0], "created_by")
	}
}

func TestFilterParamsNamedOfNilField(t *testing.T) {
	if got := filterParamsNamed(nil, ""); got != nil {
		t.Errorf("filterParamsNamed(nil, %q) = %v, want nil", "", got)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Selection: any query-dimension word reaches the query surface.
// ────────────────────────────────────────────────────────────────────────────

func TestQueryFieldsSelectDimensionWords(t *testing.T) {
	filterable := fieldPtr(api.Filterable())
	searchable := fieldPtr(api.Searchable())
	sortable := fieldPtr(api.Sortable())
	unmarked := fieldPtr(api.FieldAnnotation{})

	node := newTestType("Doc",
		newStringField("title", filterable),
		newStringField("body", searchable),
		newStringField("slug", sortable),
		newStringField("note", unmarked),
		newStringField("plain", nil), // no annotation at all
	)

	var got []string
	for _, f := range queryFields(node) {
		got = append(got, f.Name)
	}
	want := []string{"title", "body", "slug"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("queryFields = %v, want %v", got, want)
	}

	if !isFilterable(node.Fields[0]) || isSearchable(node.Fields[0]) || isSortable(node.Fields[0]) {
		t.Error("title: expected filterable only")
	}
	if !isSearchable(node.Fields[1]) || isFilterable(node.Fields[1]) {
		t.Error("body: expected searchable only")
	}
	if !isSortable(node.Fields[2]) || isFilterable(node.Fields[2]) {
		t.Error("slug: expected sortable only")
	}
	for _, f := range node.Fields[3:] {
		if isFilterable(f) || isSearchable(f) || isSortable(f) {
			t.Errorf("%s: expected no marker", f.Name)
		}
	}

	var search []string
	for _, f := range searchFields(node) {
		search = append(search, f.Name)
	}
	if !reflect.DeepEqual(search, []string{"body"}) {
		t.Errorf("searchFields = %v, want [body]", search)
	}
}

func TestParseFieldsIncludesPrimaryKeyAndFilterableFields(t *testing.T) {
	id := newUUIDField("id", nil)
	node := newTestType("Doc",
		newStringField("title", fieldPtr(api.FieldAnnotation{Filterable: true, Searchable: true})),
		newStringField("ref", fieldPtr(api.Filterable())),
		newStringField("body", fieldPtr(api.Searchable())),
	)
	node.ID = id

	got := parseFields(node)
	if len(got) != 3 {
		t.Fatalf("parseFields returned %d fields, want id, title, ref", len(got))
	}
	if got[0].Field != id || got[0].ParseKind != "text" {
		t.Errorf("primary key parse field = %+v, want UUID id through TextUnmarshaler", got[0])
	}
	if got[1].Field.Name != "title" || got[2].Field.Name != "ref" {
		t.Fatalf("parseFields order = %s, %s, %s", got[0].Field.Name, got[1].Field.Name, got[2].Field.Name)
	}

	if got, want := parseOperatorPrefixes(got[0].Operators), []string{"eq", "ne", "in", "not_in", "gt", "ge", "lt", "le", "from", "to", "between"}; !reflect.DeepEqual(got, want) {
		t.Errorf("uuid id operators = %v, want %v", got, want)
	}
	if got, want := parseOperatorPrefixes(got[1].Operators), []string{"eq", "ne", "in", "not_in", "gt", "ge", "lt", "le", "like", "prefix", "suffix", "from", "to", "between"}; !reflect.DeepEqual(got, want) {
		t.Errorf("searchable title operators = %v, want %v", got, want)
	}
	if got, want := parseOperatorPrefixes(got[2].Operators), []string{"eq", "ne", "in", "not_in", "gt", "ge", "lt", "le", "from", "to", "between"}; !reflect.DeepEqual(got, want) {
		t.Errorf("filterable-only ref operators = %v, want %v", got, want)
	}
}

func parseOperatorPrefixes(ops []parseOperator) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = op.Prefix
	}
	return out
}

// ────────────────────────────────────────────────────────────────────────────
// Imports.
// ────────────────────────────────────────────────────────────────────────────

// TestFilterImportsAreUnconditionalPlusFieldTypes pins that the imports
// the artifacts always use are always declared, including for an entity that
// marks nothing — that entity still gets an (empty) filter type, an (empty)
// user-marked allow-list and an Order function, and each names one of them.
func TestFilterImportsAreUnconditionalPlusFieldTypes(t *testing.T) {
	node := newTestType("Doc", newStringField("title", nil))
	node.Config = &gen.Config{Package: "example.com/app/ent"}

	got := filterImports(node)
	want := []string{
		`"entgo.io/ent/dialect/sql"`,
		`"example.com/app/ent/doc"`,
		`"example.com/app/ent/predicate"`,
		`"fmt"`,
		`"net/url"`,
		`"sort"`,
		`"strconv"`,
		`"strings"`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterImports (no filterable field) = %v, want %v", got, want)
	}

	// PkgPath spelled out because the shared helper omits it; ent's loader sets
	// it, and it is what fieldImportSpec reads.
	createdAt := newField("created_at",
		&field.TypeInfo{Type: field.TypeTime, Ident: "time.Time", PkgPath: "time"},
		fieldPtr(api.Filterable()))
	node.Fields = append(node.Fields, createdAt)
	got = filterImports(node)
	want = append(want, `"time"`)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filterImports (filterable time field) = %v, want %v", got, want)
	}
}

func TestFilterImportsOfNilType(t *testing.T) {
	if got := filterImports(nil); got != nil {
		t.Errorf("filterImports(nil) = %v, want nil", got)
	}
}
