// Package schema holds the hand-written ent schema for the "queryconflict"
// fixture: query annotations that contradict what ent generates for the field
// they are attached to.
//
// Every field here is legal ent and legal entapi in isolation. The
// combination has no correct output at all, so generation is refused rather
// than emitting a call to a function ent never wrote — which the consumer would
// meet as an undefined symbol inside their own ent package, with nothing
// pointing back at the annotation that asked for it.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi"
)

// Bad carries one instance of each contradiction, because the refusal reports
// every problem in the graph at once.
type Bad struct {
	ent.Schema
}

// Fields of the Bad.
func (Bad) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Annotations(entapi.IdField()),

		// Sortable, but a JSON column is not comparable, so ent's order-builder
		// template skips it and there is no ByTags to put in the allow-list.
		field.JSON("tags", []string{}).
			Optional().
			Annotations(entapi.DefaultField().AsSortable()),

		// Searchable, but free-text search is a substring match and ent emits
		// no Contains predicate for an int.
		field.Int("count").
			Annotations(entapi.DefaultField().AsSearchable()),

		// Filterable, but a required JSON column has no predicates at all —
		// not even the null pair — so the filter group would be empty.
		field.JSON("meta", map[string]string{}).
			Annotations(entapi.DefaultField().AsFilterable()),

		// Filterable, but the annotation withholds the query scope, so the
		// field is not exposed to the query API in the first place.
		field.String("token").
			Annotations(entapi.InputOnlyField().AsFilterable()),
	}
}
