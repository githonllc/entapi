package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// Tag labels pets across catalog categories.
type Tag struct {
	ent.Schema
}

// Fields of the Tag.
func (Tag) Fields() []ent.Field {
	return []ent.Field{
		// Filterable exposes the per-field `name` query parameter. Unlike the
		// category name it deliberately adds neither `_q` search nor `_sort`;
		// Unique rejects duplicate labels.
		field.String("name").
			Unique().
			Annotations(api.Filterable()),
	}
}

// Edges of the Tag.
func (Tag) Edges() []ent.Edge {
	return []ent.Edge{
		// This inverse edge lets ent traverse a tag to its pets; without Expand
		// it does not add pets to tag response DTOs.
		edge.From("pets", Pet.Type).
			Ref("tags"),
	}
}

// Annotations of the Tag. api.Resource() opts this entity into EntAPI code and
// endpoint generation.
func (Tag) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}
