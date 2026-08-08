// Package schema holds the hand-written ent schemas for the "query" codegen
// fixture: the field shapes the filter, free-text search and sort allow-list
// generation is derived from (#27).
//
// The shapes here are chosen so that every branch of the operator derivation is
// exercised by a field whose expected operator set is known independently, from
// entc/gen/func.go's fieldOps plus the sql storage driver's extra ops:
//
//	string          stringOps + EqualFold + ContainsFold,
//	                minus EqualFold (no wire spelling)      12 parameters
//	enum            enumOps                                 4 parameters
//	int, Optional   numericOps + the collapsed null question 8 + 1 parameters
//	time.Time       numericOps                               8 parameters
//
// The class rule (ADR-0005) cuts across that table: those counts are what a
// Filterable AND Searchable field earns. EqualFold produces no parameter because
// it has no wire spelling. "ref" is the same string type marked Filterable ONLY,
// so it earns 12 - 3 Searchable-gated operators = 9, plus the null question.
//
// Two fields exist to be ABSENT from the generated artifacts: "note" carries
// no query word, and "secret" carries none either. Neither may be
// filterable, searchable or orderable, and the sort allow-list test names them.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Record is the entity with a full query surface.
type Record struct {
	ent.Schema
}

// Fields of the Record.
func (Record) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		// string: the full operator set, and all three dimensions at once.
		field.String("title").
			Annotations(api.Searchable(), api.Sortable(), api.Filterable()),

		// Searchable but NOT filterable: the free-text disjunction spans more
		// than one field, and search is not a synonym for filter.
		field.String("body").
			Annotations(api.Searchable()),

		// Filterable WITHOUT Searchable: the cheap operator class only — no
		// substring parameter exists for this column (ADR-0005).
		field.String("ref").
			Optional().
			StorageKey("reference").
			Annotations(api.Filterable()),

		// enum: four operators, and no substring predicate to generate.
		field.Enum("status").
			Values("draft", "live").
			Default("draft").
			Annotations(api.Filterable()),

		// Optional numeric: numericOps plus IsNil/NotNil, which collapse into
		// one boolean parameter rather than two contradictable ones.
		field.Int("score").
			Optional().
			Nillable().
			Annotations(api.Filterable()),

		// time: numericOps, and orderable — the paging key.
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(api.ReadOnly(), api.Filterable(), api.Sortable()),

		// Query-scoped, but no marker: absent from every query artifact.
		field.String("note").Optional(),

		// Input only: no query scope and no marker. The sort rejection test
		// asks for this column by name.
		field.String("secret").Sensitive(),
	}
}

func (Record) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

// Plain is the entity that marks nothing. It must still get a filter type and a
// sort allow-list, and both must be empty: "no field is orderable" has to be
// expressible, and it is the safe end of the allow-list.
type Plain struct {
	ent.Schema
}

// Fields of the Plain.
func (Plain) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		field.String("label"),
	}
}

func (Plain) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
