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
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/githonllc/entapi/api"
)

// Bad carries one instance of each contradiction, because the refusal reports
// every problem in the graph at once.
type Bad struct {
	ent.Schema
}

// Fields of the Bad.
func (Bad) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),

		// Sortable, but a JSON column is not comparable, so ent's order-builder
		// template skips it and there is no ByTags to put in the allow-list.
		field.JSON("tags", []string{}).
			Optional().
			Annotations(api.Sortable()),

		// Searchable, but free-text search is a substring match and ent emits
		// no Contains predicate for an int.
		field.Int("count").Annotations(api.Searchable()),

		// Filterable, but a required JSON column has no predicates at all —
		// not even the null pair — so the filter group would be empty.
		field.JSON("meta", map[string]string{}).Annotations(api.Filterable()),

		field.String("token").
			Sensitive().
			Annotations(api.Filterable()),

		field.String("managed_secret").
			Default("server").
			Sensitive().
			Annotations(api.ReadOnly()),

		field.String("hidden").
			Optional().
			Annotations(api.Hidden(), api.Sortable()),

		field.String("private").
			Optional().
			StorageKey("_private").
			Annotations(api.Filterable()),
	}
}

func (Bad) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource().Except(api.OpList)}
}
