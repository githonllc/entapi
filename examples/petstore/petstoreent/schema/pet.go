package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi"
	"github.com/githonllc/entapi/api"
)

// Pet is an animal offered by the store.
type Pet struct {
	ent.Schema
}

// Fields of the Pet.
func (Pet) Fields() []ent.Field {
	return []ent.Field{
		// Searchable feeds the free-text `_q` parameter, Filterable the
		// per-field `name` parameter, Sortable the `_sort` allow-list.
		field.String("name").
			Annotations(api.Searchable(), api.Filterable(), api.Sortable()),

		// Filterable exposes `status` as a typed query parameter; Sortable adds
		// it to the `_sort` allow-list. The default keeps it optional on create.
		field.Enum("status").
			Values("available", "pending", "sold").
			Default("available").
			Annotations(api.Filterable(), api.Sortable()),

		// Filterable exposes `price` predicates and Sortable permits it in
		// `_sort`; Optional + Default makes the create field a pointer.
		field.Float("price").
			Optional().
			Default(0).
			Annotations(api.Filterable(), api.Sortable()),

		// This is the JSON-slice field shape: Optional makes it a pointer in the
		// create request, and without Filterable it gets no query operators.
		field.JSON("photo_urls", []string{}).
			Optional(),

		// ReadOnly keeps it out of create and patch requests while it stays in
		// responses; Immutable + Default lets ent fill it at creation, and
		// Sortable adds it to the `_sort` allow-list.
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			Annotations(api.Sortable(), api.ReadOnly()),

		// A Required edge whose create family is reachable must name its backing
		// field with edge.Field, so the create request can carry category_id.
		field.Int("category_id"),
	}
}

// Edges of the Pet.
func (Pet) Edges() []ent.Edge {
	return []ent.Edge{
		// Expand eager-loads the category and includes its nested summary in pet
		// responses instead of forcing a second request per pet.
		edge.To("category", Category.Type).
			Unique().
			Required().
			Field("category_id").
			Annotations(api.Expand()),

		// Expand does the same for the to-many side: responses carry a list of
		// tag summaries instead of forcing a second request per pet.
		edge.To("tags", Tag.Type).
			Annotations(api.Expand()),
	}
}

// Mixin of the Pet. SoftDeleteMixin turns DELETE into a tombstone write and
// makes every generated query filter deleted pets. It does not cascade: an
// order keeps its pet_id foreign key, and expanding that pet yields JSON null.
func (Pet) Mixin() []ent.Mixin {
	return []ent.Mixin{entapi.SoftDeleteMixin{}}
}

// Annotations of the Pet. api.Resource() opts this entity into EntAPI code and
// endpoint generation.
func (Pet) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}
}
