package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/githonllc/entapi/api"
)

type Record struct{ ent.Schema }

func (Record) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("misplaced_expand").Optional().Annotations(api.Expand()),
	}
}

func (Record) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("owners", Owner.Type).Annotations(api.Filterable()),
	}
}

func (Record) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }

type Owner struct{ ent.Schema }

func (Owner) Fields() []ent.Field { return []ent.Field{field.String("name")} }

func (Owner) Annotations() []schema.Annotation { return []schema.Annotation{api.Resource()} }
