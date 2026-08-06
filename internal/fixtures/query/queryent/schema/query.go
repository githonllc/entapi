// Package schema holds the hand-written ent schemas for the "query" codegen
// fixture: the field shapes the filter, free-text search and sort allow-list
// generation is derived from (#27).
//
// The shapes here are chosen so that every branch of the operator derivation is
// exercised by a field whose expected operator set is known independently, from
// entc/gen/func.go's fieldOps plus the sql storage driver's extra ops:
//
//	string          stringOps + EqualFold + ContainsFold   13 parameters
//	enum            enumOps                                 4 parameters
//	int, Optional   numericOps + the collapsed null question 8 + 1 parameters
//	time.Time       numericOps                               8 parameters
//
// The class rule (ADR-0005) cuts across that table: those counts are what a
// Filterable AND Searchable field earns. "ref" is the same string type marked
// Filterable ONLY, so it earns 13 - 4 substring operators + the null question.
//
// Two fields exist to be ABSENT from the generated artifacts: "note" carries a
// query scope but no marker, and "secret" carries neither. Neither may be
// filterable, searchable or orderable, and the sort allow-list test names them.
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entdomain"
)

// Record is the entity with a full query surface.
type Record struct {
	ent.Schema
}

// Fields of the Record.
func (Record) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		// string: the full operator set, and all three dimensions at once.
		field.String("title").
			Annotations(entdomain.DefaultField().AsFilterable().AsSearchable().AsSortable()),

		// Searchable but NOT filterable: the free-text disjunction spans more
		// than one field, and search is not a synonym for filter.
		field.String("body").
			Annotations(entdomain.DefaultField().AsSearchable()),

		// Filterable WITHOUT Searchable: the cheap operator class only — no
		// substring parameter exists for this column (ADR-0005).
		field.String("ref").
			Optional().
			Annotations(entdomain.DefaultField().AsFilterable()),

		// enum: four operators, and no substring predicate to generate.
		field.Enum("status").
			Values("draft", "live").
			Default("draft").
			Annotations(entdomain.DefaultField().AsFilterable()),

		// Optional numeric: numericOps plus IsNil/NotNil, which collapse into
		// one boolean parameter rather than two contradictable ones.
		field.Int("score").
			Optional().
			Nillable().
			Annotations(entdomain.DefaultField().AsFilterable()),

		// time: numericOps, and orderable — the paging key.
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(entdomain.OutputOnlyField().AsFilterable().AsSortable()),

		// Query-scoped, but no marker: absent from every query artifact.
		field.String("note").
			Optional().
			Annotations(entdomain.DefaultField()),

		// Input only: no query scope and no marker. The sort rejection test
		// asks for this column by name.
		field.String("secret").
			Annotations(entdomain.InputOnlyField()),
	}
}

// Plain is the entity that marks nothing. It must still get a filter type and a
// sort allow-list, and both must be empty: "no field is orderable" has to be
// expressible, and it is the safe end of the allow-list.
type Plain struct {
	ent.Schema
}

// Fields of the Plain.
func (Plain) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entdomain.IdField()),

		field.String("label").
			Annotations(entdomain.DefaultField()),
	}
}
