package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

// Order records a pet purchase.
type Order struct {
	ent.Schema
}

// Fields of the Order.
func (Order) Fields() []ent.Field {
	return []ent.Field{
		// Positive turns a zero or negative quantity into an ent validation
		// error instead of storing an impossible purchase.
		field.Int("quantity").
			Positive(),

		// Filterable exposes the typed `status` query parameter; Sortable adds
		// status to the `_sort` allow-list. The default fills omitted creates.
		field.Enum("status").
			Values("placed", "approved", "delivered").
			Default("placed").
			Annotations(api.Filterable(), api.Sortable()),

		// Optional + Nillable lets a create omit the date and a patch clear it;
		// the database stores either a timestamp or NULL.
		field.Time("ship_date").
			Optional().
			Nillable(),

		// ReadOnly keeps it out of create and patch requests while it stays in
		// responses; Immutable + Default lets ent fill it at creation, and
		// Sortable adds it to the `_sort` allow-list.
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			Annotations(api.Sortable(), api.ReadOnly()),

		// A Required edge whose create family is reachable must name its backing
		// field with edge.Field, so the create request can carry pet_id.
		field.Int("pet_id"),
	}
}

// Edges of the Order.
func (Order) Edges() []ent.Edge {
	return []ent.Edge{
		// Expand eager-loads the pet and includes its nested summary in order
		// responses. Summaries have no edges, so expansion is one level deep.
		edge.To("pet", Pet.Type).
			Unique().
			Required().
			Field("pet_id").
			Annotations(api.Expand()),
	}
}

// Annotations of the Order. Resource opts the entity into EntAPI, but orders
// are records, not rows to remove, so Except(api.OpDelete) omits the DELETE
// route and its DeleteOrderEndpoint accessor; naming that removed method is a
// compile error.
func (Order) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource().Except(api.OpDelete)}
}
